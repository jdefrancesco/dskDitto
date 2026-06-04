package main

import (
	"fmt"
	"sort"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/internal/fuzzy"

	"github.com/pterm/pterm"
)

// addFuzzyContentGroups converts walk candidates into fuzzy groups and records them in dMap.
// Returns (addedGroups, processed, skippedBySignature, error).
func addFuzzyContentGroups(
	dMap *dmap.Dmap,
	candidates []dwalk.FileCandidate,
	minDups uint,
	threshold int,
	sameExt bool,
	maxCandidates int,
	onProgress func(done, processed, skipped, total uint),
	onGroupProgress func(done, total int),
) (uint, uint, uint, error) {
	if dMap == nil {
		return 0, 0, 0, nil
	}
	if minDups < 2 {
		minDups = 2
	}

	fuzzyCandidates := make([]fuzzy.Candidate, 0, len(candidates))
	for _, c := range candidates {
		fuzzyCandidates = append(fuzzyCandidates, fuzzy.Candidate{Path: c.Path, Size: c.Size})
	}

	res, err := fuzzy.FindSimilarGroups(fuzzyCandidates, fuzzy.Options{
		MinSimilarity:   threshold,
		MinGroupSize:    int(minDups),
		SameExt:         sameExt,
		MaxReadBytes:    fuzzy.DefaultMaxReadBytes,
		MaxSizeRatio:    fuzzy.DefaultMaxSizeRatio,
		MaxCandidates:   maxCandidates,
		OnProgress:      onProgress,
		OnGroupProgress: onGroupProgress,
	})
	if err != nil {
		return 0, res.Processed, res.Skipped, err
	}
	if res.CandidatesTruncated > 0 {
		pterm.Warning.Printf(
			"Fuzzy grouping limited to %d files (%d dropped). Use --fuzzy-max-candidates to adjust or add filters (--min-size, --fuzzy-same-ext) to reduce the candidate set.\n",
			maxCandidates, res.CandidatesTruncated,
		)
	}

	var addedGroups uint
	for _, group := range res.Groups {
		if len(group.Matches) < int(minDups) {
			continue
		}
		sort.Slice(group.Matches, func(i, j int) bool {
			return group.Matches[i].Path < group.Matches[j].Path
		})
		for _, match := range group.Matches {
			dMap.AddFuzzyPath(group.Key, match.Path)
		}
		addedGroups++
	}

	return addedGroups, res.Processed, res.Skipped, nil
}

// validateFuzzyMode returns an error if the --fuzzy flag is combined with incompatible flags.
func validateFuzzyMode(fuzzyMode, shallowMode bool, singleFile, backupFile, restoreFile string, keep uint, linkMode bool, threshold int) error {
	if !fuzzyMode {
		return nil
	}
	if shallowMode {
		return fmt.Errorf("--fuzzy cannot be combined with --name-only or --file-shallow")
	}
	if singleFile != "" {
		return fmt.Errorf("--fuzzy cannot be combined with --file")
	}
	if backupFile != "" {
		return fmt.Errorf("restore backups are not supported for fuzzy matches; rerun without --backup")
	}
	if restoreFile != "" {
		return fmt.Errorf("--fuzzy cannot be combined with --restore")
	}
	if keep > 0 || linkMode {
		return fmt.Errorf("--remove/--link are disabled in --fuzzy mode; near matches are review-only")
	}
	if threshold < 0 || threshold > 100 {
		return fmt.Errorf("--fuzzy-threshold must be between 0 and 100")
	}
	return nil
}
