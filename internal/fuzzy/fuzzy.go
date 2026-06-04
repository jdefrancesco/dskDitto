package fuzzy

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const (
	DefaultMinSimilarity      = 75
	DefaultMaxReadBytes       = int64(256 * 1024)
	DefaultMaxSizeRatio       = 2.0
	DefaultMaxFuzzyCandidates = 10_000
	// DefaultFuzzyMinFileSize is the minimum file size for fuzzy candidates when
	// no explicit --min-size is provided. Tiny files are rarely meaningful for
	// near-duplicate detection and filtering them dramatically reduces candidate count.
	DefaultFuzzyMinFileSize = int64(4 * 1024) // 4 KiB
	DefaultFuzzyMinSizeStr  = "4K"

	// sigWorkerCap is the maximum number of goroutines used for signature
	// computation. Signature work is CPU-bound (hashing + comparison), so
	// capping at a small multiple of GOMAXPROCS avoids over-scheduling and
	// cache thrashing on high-core-count machines.
	sigWorkerCap = 8
)

// Candidate represents a file eligible for fuzzy content matching.
type Candidate struct {
	Path string
	Size int64
}

// Match describes a file and its similarity score relative to a group representative.
type Match struct {
	Path       string
	Similarity int
}

// Group holds a set of near-duplicate files discovered via fuzzy content matching.
type Group struct {
	Key            string
	Representative string
	Matches        []Match
}

// Result summarizes a fuzzy scan operation.
type Result struct {
	Groups              []Group
	Processed           uint
	Skipped             uint
	CandidatesTruncated int // number of candidates dropped due to MaxCandidates limit
}

type Options struct {
	MinSimilarity int
	MinGroupSize  int
	SameExt       bool
	MaxReadBytes  int64
	MaxSizeRatio  float64
	// MaxCandidates caps how many file signatures enter the grouping phase.
	// If 0, DefaultMaxFuzzyCandidates is used. Set to -1 to disable the cap.
	MaxCandidates   int
	OnProgress      func(done uint, processed uint, skipped uint, total uint)
	OnGroupProgress func(done, total int)
}

func normalizeOptions(opts Options) Options {
	if opts.MinSimilarity < 0 {
		opts.MinSimilarity = 0
	}
	if opts.MinSimilarity > 100 {
		opts.MinSimilarity = 100
	}
	if opts.MinSimilarity == 0 {
		opts.MinSimilarity = DefaultMinSimilarity
	}
	if opts.MinGroupSize < 2 {
		opts.MinGroupSize = 2
	}
	if opts.MaxReadBytes <= 0 {
		opts.MaxReadBytes = DefaultMaxReadBytes
	}
	if opts.MaxSizeRatio < 1 {
		opts.MaxSizeRatio = DefaultMaxSizeRatio
	}
	if opts.MaxCandidates == 0 {
		opts.MaxCandidates = DefaultMaxFuzzyCandidates
	}
	return opts
}

type fileSig struct {
	Path string
	Size int64
	Hash uint64
}

// FindSimilarGroups computes fuzzy signatures for candidate files and returns
// groups of likely near-duplicate files. Similarity is based on 64-bit simhash
// Hamming distance and is independent of file names.
func FindSimilarGroups(candidates []Candidate, opts Options) (Result, error) {
	result := Result{}
	if len(candidates) == 0 {
		return result, nil
	}

	opts = normalizeOptions(opts)
	maxDistance := MaxDistanceForSimilarity(opts.MinSimilarity)

	sigs, processed, skipped := collectSignatures(candidates, opts)
	result.Processed = processed
	result.Skipped = skipped
	if len(sigs) == 0 {
		return result, nil
	}

	// Enforce MaxCandidates to prevent O(n²) BK-tree degeneration on huge inputs.
	if opts.MaxCandidates > 0 && len(sigs) > opts.MaxCandidates {
		result.CandidatesTruncated = len(sigs) - opts.MaxCandidates
		sigs = sigs[:opts.MaxCandidates]
	}

	total := len(sigs)
	reportGroupProgress := func(done int) {
		if opts.OnGroupProgress != nil {
			opts.OnGroupProgress(done, total*2) // insert phase + search phase
		}
	}

	tree := &bkTree{}
	for i, sig := range sigs {
		tree.Insert(sig.Hash, i)
		if (i+1)%512 == 0 || i+1 == total {
			reportGroupProgress(i + 1)
		}
	}

	uf := newUnionFind(len(sigs))
	for i, sig := range sigs {
		neighbors := tree.Search(sig.Hash, maxDistance)
		for _, j := range neighbors {
			if j <= i {
				continue
			}
			other := sigs[j]
			if opts.SameExt && !sameExt(sig.Path, other.Path) {
				continue
			}
			if !sizeRatioWithin(sig.Size, other.Size, opts.MaxSizeRatio) {
				continue
			}
			distance := HammingDistance(sig.Hash, other.Hash)
			similarity := SimilarityFromDistance(distance)
			if similarity < opts.MinSimilarity {
				continue
			}
			uf.Union(i, j)
		}
		if (i+1)%512 == 0 || i+1 == total {
			reportGroupProgress(total + i + 1)
		}
	}

	components := make(map[int][]int)
	for i := range sigs {
		root := uf.Find(i)
		components[root] = append(components[root], i)
	}

	groups := make([]Group, 0, len(components))
	for _, idxs := range components {
		if len(idxs) < opts.MinGroupSize {
			continue
		}
		sort.Slice(idxs, func(i, j int) bool {
			return sigs[idxs[i]].Path < sigs[idxs[j]].Path
		})

		rep := sigs[idxs[0]]
		matches := make([]Match, 0, len(idxs))
		for _, idx := range idxs {
			s := sigs[idx]
			d := HammingDistance(rep.Hash, s.Hash)
			matches = append(matches, Match{Path: s.Path, Similarity: SimilarityFromDistance(d)})
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].Similarity == matches[j].Similarity {
				return matches[i].Path < matches[j].Path
			}
			return matches[i].Similarity > matches[j].Similarity
		})

		key := fmt.Sprintf("near-content >=%d%% (sig %016x)", opts.MinSimilarity, rep.Hash)
		groups = append(groups, Group{
			Key:            key,
			Representative: rep.Path,
			Matches:        matches,
		})
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Matches) == len(groups[j].Matches) {
			return groups[i].Key < groups[j].Key
		}
		return len(groups[i].Matches) > len(groups[j].Matches)
	})

	result.Groups = groups
	return result, nil
}

type signatureResult struct {
	index int
	sig   fileSig
	ok    bool
}

func collectSignatures(candidates []Candidate, opts Options) ([]fileSig, uint, uint) {
	if len(candidates) == 0 {
		return nil, 0, 0
	}

	workers := signatureWorkerCount(len(candidates))
	if workers == 1 {
		return collectSignaturesSequential(candidates, opts)
	}

	jobs := make(chan int, workers*2)
	results := make(chan signatureResult, workers*2)

	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				candidate := candidates[index]
				if candidate.Path == "" || candidate.Size < 0 {
					results <- signatureResult{index: index}
					continue
				}
				hash, err := SignatureFromFile(candidate.Path, opts.MaxReadBytes)
				if err != nil {
					results <- signatureResult{index: index}
					continue
				}
				results <- signatureResult{
					index: index,
					sig: fileSig{
						Path: candidate.Path,
						Size: candidate.Size,
						Hash: hash,
					},
					ok: true,
				}
			}
		}()
	}

	go func() {
		for index := range candidates {
			jobs <- index
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	total := uint(len(candidates))
	ordered := make([]fileSig, len(candidates))
	valid := make([]bool, len(candidates))
	var processed uint
	var skipped uint
	var done uint
	for result := range results {
		done++
		if result.ok {
			ordered[result.index] = result.sig
			valid[result.index] = true
			processed++
		} else {
			skipped++
		}
		if opts.OnProgress != nil && (done == total || done%128 == 0) {
			opts.OnProgress(done, processed, skipped, total)
		}
	}

	sigs := make([]fileSig, 0, processed)
	for index := range ordered {
		if !valid[index] {
			continue
		}
		sigs = append(sigs, ordered[index])
	}
	return sigs, processed, skipped
}

func collectSignaturesSequential(candidates []Candidate, opts Options) ([]fileSig, uint, uint) {
	total := uint(len(candidates))
	sigs := make([]fileSig, 0, len(candidates))
	var processed uint
	var skipped uint
	for i, candidate := range candidates {
		if candidate.Path == "" || candidate.Size < 0 {
			skipped++
		} else if hash, err := SignatureFromFile(candidate.Path, opts.MaxReadBytes); err != nil {
			skipped++
		} else {
			sigs = append(sigs, fileSig{Path: candidate.Path, Size: candidate.Size, Hash: hash})
			processed++
		}
		done := uint(i + 1)
		if opts.OnProgress != nil && (done == total || done%128 == 0) {
			opts.OnProgress(done, processed, skipped, total)
		}
	}
	return sigs, processed, skipped
}

func signatureWorkerCount(total int) int {
	if total <= 0 {
		return 1
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	workers = min(workers, sigWorkerCap)
	workers = min(workers, total)
	return workers
}

func sameExt(a, b string) bool {
	extA := strings.ToLower(filepath.Ext(a))
	extB := strings.ToLower(filepath.Ext(b))
	if extA == "" && extB == "" {
		return true
	}
	return extA == extB
}

func sizeRatioWithin(a, b int64, maxRatio float64) bool {
	if a <= 0 || b <= 0 {
		return a == b
	}
	small := float64(min(a, b))
	large := float64(max(a, b))
	if small == 0 {
		return false
	}
	return (large / small) <= maxRatio
}

func MaxDistanceForSimilarity(similarity int) int {
	if similarity >= 100 {
		return 0
	}
	if similarity <= 0 {
		return 64
	}
	remaining := 100 - similarity
	return int(math.Floor(float64(remaining) * 64.0 / 100.0))
}
