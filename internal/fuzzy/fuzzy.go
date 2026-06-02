package fuzzy

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultMinSimilarity = 75
	DefaultMaxReadBytes  = int64(256 * 1024)
	DefaultMaxSizeRatio  = 2.0
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
	Groups    []Group
	Processed uint
	Skipped   uint
}

type Options struct {
	MinSimilarity int
	MinGroupSize  int
	SameExt       bool
	MaxReadBytes  int64
	MaxSizeRatio  float64
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

	sigs := make([]fileSig, 0, len(candidates))
	for _, c := range candidates {
		if c.Path == "" || c.Size < 0 {
			result.Skipped++
			continue
		}
		hash, err := SignatureFromFile(c.Path, opts.MaxReadBytes)
		if err != nil {
			result.Skipped++
			continue
		}
		sigs = append(sigs, fileSig{Path: c.Path, Size: c.Size, Hash: hash})
	}

	result.Processed = uint(len(sigs))
	if len(sigs) == 0 {
		return result, nil
	}

	tree := &bkTree{}
	for i, sig := range sigs {
		tree.Insert(sig.Hash, i)
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
