package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/pkg/utils"
)

// Worker-pool tuning constants.
// These multipliers scale against GOMAXPROCS; the resulting count is always
// capped by utils.MaxWorkerCount (128) to prevent excessive goroutines and
// I/O contention on high-core or spinning-disk systems.
const (
	// hashWorkerMultiplier controls full-content hashing workers.
	// I/O-bound work benefits from more parallelism than pure CPU work.
	hashWorkerMultiplier = 4
	// sampleWorkerMultiplier controls sample/partial hashing workers.
	// Sample reads are smaller and faster, so the same multiplier as full
	// hashing provides adequate throughput without over-committing I/O.
	sampleWorkerMultiplier = 4
)

// hashWorkerCount returns the number of goroutines to use for full-file hashing.
func hashWorkerCount(total int) int {
	return utils.BoundedWorkerCount(total, hashWorkerMultiplier)
}

// sampleWorkerCount returns the number of goroutines to use for sample hashing.
func sampleWorkerCount(total int) int {
	return utils.BoundedWorkerCount(total, sampleWorkerMultiplier)
}

func eligibleHashCandidates(sizeGroups map[int64][]dwalk.FileCandidate, minDups uint, singleFileMode bool) ([]dwalk.FileCandidate, uint) {
	if minDups < 2 {
		minDups = 2
	}

	total := 0
	for _, files := range sizeGroups {
		if singleFileMode || uint(len(files)) >= minDups {
			total += len(files)
		}
	}

	candidates := make([]dwalk.FileCandidate, 0, total)
	var skipped uint
	for _, files := range sizeGroups {
		if singleFileMode || uint(len(files)) >= minDups {
			candidates = append(candidates, files...)
			continue
		}
		skipped += uint(len(files))
	}

	return candidates, skipped
}

// singleFileTarget holds precomputed data for --file mode.
type singleFileTarget struct {
	filePath     string
	fileSize     int64
	digest       dmap.Digest
	sampleDigest dmap.Digest
}

// prepareSingleFileTarget stats, hashes, and sample-hashes the --file target.
func prepareSingleFileTarget(path string, hashAlgo dfs.HashAlgorithm, opts dfs.HashOptions) (*singleFileTarget, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat --file path %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--file path must be a regular file: %s", path)
	}
	dfile, err := dfs.NewDfileWithOptions(path, info.Size(), hashAlgo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to hash --file target %s: %w", path, err)
	}
	sample, err := dfs.HashFileSampleWithOptions(dfile.FileName(), info.Size(), hashAlgo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to sample --file target %s: %w", path, err)
	}
	return &singleFileTarget{
		filePath:     dfile.FileName(),
		fileSize:     info.Size(),
		digest:       dmap.Digest(dfile.Hash()),
		sampleDigest: dmap.Digest(sample.Digest),
	}, nil
}

type sampleKey struct {
	size   int64
	digest dmap.Digest
}

type sampledFile struct {
	candidate       dwalk.FileCandidate
	digest          dmap.Digest
	coversWholeFile bool
}

func eligibleSampleCandidates(sampleGroups map[sampleKey][]sampledFile, minDups uint, singleFileMode bool) ([]sampledFile, []dwalk.FileCandidate, uint) {
	if minDups < 2 {
		minDups = 2
	}

	var directFiles []sampledFile
	var fullHashList []dwalk.FileCandidate
	var skipped uint

	for _, files := range sampleGroups {
		if len(files) == 0 {
			continue
		}
		if !singleFileMode && uint(len(files)) < minDups {
			skipped += uint(len(files))
			continue
		}
		if files[0].coversWholeFile {
			directFiles = append(directFiles, files...)
			continue
		}
		for _, file := range files {
			fullHashList = append(fullHashList, file.candidate)
		}
	}

	return directFiles, fullHashList, skipped
}

// runContentPipeline runs the two-phase sample-then-full-hash pipeline and populates dMap.
// It returns the count of files sampled and the count fully hashed.
func runContentPipeline(
	ctx context.Context,
	dMap *dmap.Dmap,
	sampleList []dwalk.FileCandidate,
	minDups uint,
	singleTarget *singleFileTarget,
	hashAlgo dfs.HashAlgorithm,
	hashOptions dfs.HashOptions,
	tickC <-chan time.Time,
	updateProgress func(string),
) (sampledFiles, fullHashedFiles uint) {
	singleFileMode := singleTarget != nil
	sampleGroups := make(map[sampleKey][]sampledFile, 4096)

	if len(sampleList) > 0 {
		sampleJobs := make(chan dwalk.FileCandidate, min(len(sampleList), 4096))
		sampledFileCh := make(chan sampledFile, min(len(sampleList), 4096))
		workerCount := sampleWorkerCount(len(sampleList))

		var sampleWG sync.WaitGroup
		sampleWG.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer sampleWG.Done()
				for candidate := range sampleJobs {
					sample, err := dfs.HashFileSampleWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
					if err != nil {
						dsklog.Dlogger.Debugf("Skipping file after sample failure %s: %v", candidate.Path, err)
						continue
					}
					select {
					case <-ctx.Done():
						return
					case sampledFileCh <- sampledFile{
						candidate:       candidate,
						digest:          dmap.Digest(sample.Digest),
						coversWholeFile: sample.CoversWholeFile,
					}:
					}
				}
			}()
		}
		go func() {
			defer close(sampleJobs)
			for _, candidate := range sampleList {
				select {
				case <-ctx.Done():
					return
				case sampleJobs <- candidate:
				}
			}
		}()
		go func() {
			sampleWG.Wait()
			close(sampledFileCh)
		}()

	SampleLoop:
		for {
			select {
			case sample, ok := <-sampledFileCh:
				if !ok {
					break SampleLoop
				}
				sampledFiles++
				if singleFileMode && sample.digest != singleTarget.sampleDigest {
					continue
				}
				key := sampleKey{size: sample.candidate.Size, digest: sample.digest}
				sampleGroups[key] = append(sampleGroups[key], sample)
			case <-tickC:
				updateProgress(fmt.Sprintf("Sampled %d/%d candidate files...", sampledFiles, len(sampleList)))
			}
		}
	}

	directFiles, fullHashList, skippedBySample := eligibleSampleCandidates(sampleGroups, minDups, singleFileMode)
	sampleGroups = nil
	dsklog.Dlogger.Debugf("Skipped %d files with unique samples before full hashing", skippedBySample)

	for _, file := range directFiles {
		dMap.AddPathSized(file.digest, file.candidate.Path, file.candidate.Size)
	}

	if len(fullHashList) == 0 {
		return
	}

	hashJobs := make(chan dwalk.FileCandidate, min(len(fullHashList), 4096))
	hashedFiles := make(chan *dfs.Dfile, min(len(fullHashList), 4096))
	workerCount := hashWorkerCount(len(fullHashList))

	var hashWG sync.WaitGroup
	hashWG.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer hashWG.Done()
			for candidate := range hashJobs {
				dFile, err := dfs.NewDfileWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
				if err != nil {
					dsklog.Dlogger.Debugf("Skipping file after hash failure %s: %v", candidate.Path, err)
					continue
				}
				select {
				case <-ctx.Done():
					return
				case hashedFiles <- dFile:
				}
			}
		}()
	}
	go func() {
		defer close(hashJobs)
		for _, candidate := range fullHashList {
			select {
			case <-ctx.Done():
				return
			case hashJobs <- candidate:
			}
		}
	}()
	go func() {
		hashWG.Wait()
		close(hashedFiles)
	}()

HashLoop:
	for {
		select {
		case dFile, ok := <-hashedFiles:
			if !ok {
				break HashLoop
			}
			if dFile == nil {
				dsklog.Dlogger.Warn("Received nil dFile, skipping...")
				continue
			}
			dMap.Add(dFile)
			fullHashedFiles++
		case <-tickC:
			updateProgress(fmt.Sprintf("Hashed %d/%d full candidate files...", fullHashedFiles, len(fullHashList)))
		}
	}
	return
}
