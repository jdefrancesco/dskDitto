package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"

	"github.com/pterm/pterm"
)

// scanSettings contains mode-specific settings resolved before walking files.
type scanSettings struct {
	options           commandOptions
	config            config.Config
	roots             []string
	singleTarget      *singleFileTarget
	shallowTargetName string
	fuzzyMinFileSize  int64
}

type scanStats struct {
	scannedFiles    uint
	sampledFiles    uint
	fullHashedFiles uint
	fuzzyProcessed  uint
	fuzzySkipped    uint
	duration        time.Duration
}

// scanFiles collects metadata, then groups candidates using the selected mode.
func scanFiles(ctx context.Context, dMap *dmap.Dmap, settings scanSettings) (scanStats, error) {
	opts := settings.options
	appCfg := settings.config
	rootDirs := settings.roots
	singleTarget := settings.singleTarget
	shallowTargetName := settings.shallowTargetName
	fuzzyMinFileSize := settings.fuzzyMinFileSize
	fuzzyMode := opts.fuzzy
	shallowMode := opts.nameOnly || opts.fileShallow != ""
	singleFileMode := singleTarget != nil
	minDups := appCfg.MinDuplicates
	hashAlgo := appCfg.HashAlgorithm
	hashOptions := dfs.HashOptions{NoCache: appCfg.NoCache}
	start := time.Now()

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	tickC := tick.C
	infoSpinner, _ := pterm.DefaultSpinner.Start()
	defer infoSpinner.Stop()
	updateProgress := func(msg string) { infoSpinner.UpdateText(msg) }

	// First collect cheap file metadata. Hashing waits until after this pass so
	// unique file sizes never touch the expensive content path.
	candidateFiles := make(chan dwalk.FileCandidate, 4096)
	walker := dwalk.NewCandidateWalker(rootDirs, candidateFiles, appCfg)
	walker.Run(ctx)

	sizeGroups := make(map[int64][]dwalk.FileCandidate, 4096)
	nameGroups := make(map[string][]dwalk.FileCandidate, 4096)
	fuzzyCandidates := make([]dwalk.FileCandidate, 0, 4096)
	var scannedFiles uint

CollectLoop:
	for {
		select {
		case <-ctx.Done():
			for range candidateFiles {
			}
			break CollectLoop

		case candidate, ok := <-candidateFiles:
			if !ok {
				break CollectLoop
			}
			scannedFiles++
			if fuzzyMode {
				if fuzzyMinFileSize > 0 && candidate.Size < fuzzyMinFileSize {
					continue
				}
				fuzzyCandidates = append(fuzzyCandidates, candidate)
				continue
			}
			if shallowMode {
				name := filepath.Base(candidate.Path)
				if shallowTargetName != "" && name != shallowTargetName {
					continue
				}
				nameGroups[name] = append(nameGroups[name], candidate)
				continue
			}
			if singleFileMode && candidate.Size != singleTarget.fileSize {
				continue
			}
			sizeGroups[candidate.Size] = append(sizeGroups[candidate.Size], candidate)

		case <-tickC:
			progressMsg := fmt.Sprintf("Scanned %d files...", scannedFiles)
			updateProgress(progressMsg)
		}
	}

	var sampledFiles uint
	var fullHashedFiles uint
	var fuzzyProcessed uint
	var fuzzySkipped uint

	if fuzzyMode {
		addedGroups, processed, skippedBySignature, fuzzyErr := addFuzzyContentGroups(dMap, fuzzyCandidates, minDups, opts.fuzzyThreshold, opts.fuzzySameExt, opts.fuzzyMaxCandidates,
			func(done uint, processed uint, skipped uint, total uint) {
				progressMsg := fmt.Sprintf("Scanned %d files, fuzzy-processed %d/%d (kept %d, skipped %d)...", scannedFiles, done, total, processed, skipped)
				updateProgress(progressMsg)
			},
			func(done, total int) {
				progressMsg := fmt.Sprintf("Scanned %d files, fuzzy-grouping %d/%d...", scannedFiles, done, total)
				updateProgress(progressMsg)
			},
		)
		if fuzzyErr != nil {
			return scanStats{}, fmt.Errorf("fuzzy scan failed: %w", fuzzyErr)
		}
		fuzzyProcessed = processed
		fuzzySkipped = skippedBySignature
		dsklog.Dlogger.Debugf("Added %d fuzzy content groups; skipped %d files during signature stage", addedGroups, skippedBySignature)
	} else if shallowMode {
		addedGroups, skippedByName := addNameOnlyGroups(dMap, nameGroups, minDups)
		nameGroups = nil
		dsklog.Dlogger.Debugf("Added %d shallow filename groups; skipped %d files with unique names", addedGroups, skippedByName)
	} else {
		sampleList, skippedBySize := eligibleHashCandidates(sizeGroups, minDups, singleFileMode)
		sizeGroups = nil
		dsklog.Dlogger.Debugf("Skipped %d files with unique sizes before sample hashing", skippedBySize)
		sampledFiles, fullHashedFiles = runContentPipeline(ctx, dMap, sampleList, minDups, singleTarget, hashAlgo, hashOptions, tickC, updateProgress)
	}

	duration := time.Since(start)

	return scanStats{
		scannedFiles:    scannedFiles,
		sampledFiles:    sampledFiles,
		fullHashedFiles: fullHashedFiles,
		fuzzyProcessed:  fuzzyProcessed,
		fuzzySkipped:    fuzzySkipped,
		duration:        duration,
	}, nil
}
