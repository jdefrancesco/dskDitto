package main

import (
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
)

func TestNewDupView(t *testing.T) {

}

func TestEligibleHashCandidatesSkipsUniqueSizes(t *testing.T) {
	groups := map[int64][]dwalk.FileCandidate{
		10: []dwalk.FileCandidate{{Path: "one", Size: 10}},
		20: []dwalk.FileCandidate{
			{Path: "two-a", Size: 20},
			{Path: "two-b", Size: 20},
		},
	}

	got, skipped := eligibleHashCandidates(groups, 2, false)
	if skipped != 1 {
		t.Fatalf("expected one skipped unique-size file, got %d", skipped)
	}
	if len(got) != 2 {
		t.Fatalf("expected two hash candidates, got %d", len(got))
	}
}

func TestEligibleHashCandidatesKeepsSingleFileCandidates(t *testing.T) {
	groups := map[int64][]dwalk.FileCandidate{
		10: []dwalk.FileCandidate{{Path: "target-sized", Size: 10}},
	}

	got, skipped := eligibleHashCandidates(groups, 2, true)
	if skipped != 0 {
		t.Fatalf("expected no skipped target-size candidates, got %d", skipped)
	}
	if len(got) != 1 {
		t.Fatalf("expected one hash candidate, got %d", len(got))
	}
}

func TestEligibleSampleCandidatesSplitsFullSamplesAndLargeFiles(t *testing.T) {
	var digest dmap.Digest
	digest[0] = 1
	groups := map[sampleKey][]sampledFile{
		{size: 10, digest: digest}: []sampledFile{
			{candidate: dwalk.FileCandidate{Path: "small-a", Size: 10}, digest: digest, coversWholeFile: true},
			{candidate: dwalk.FileCandidate{Path: "small-b", Size: 10}, digest: digest, coversWholeFile: true},
		},
		{size: 100, digest: digest}: []sampledFile{
			{candidate: dwalk.FileCandidate{Path: "large-a", Size: 100}, digest: digest},
			{candidate: dwalk.FileCandidate{Path: "large-b", Size: 100}, digest: digest},
		},
		{size: 200, digest: digest}: []sampledFile{
			{candidate: dwalk.FileCandidate{Path: "unique-sample", Size: 200}, digest: digest},
		},
	}

	direct, full, skipped := eligibleSampleCandidates(groups, 2, false)
	if skipped != 1 {
		t.Fatalf("expected one skipped unique-sample file, got %d", skipped)
	}
	if len(direct) != 2 {
		t.Fatalf("expected two direct full-sample files, got %d", len(direct))
	}
	if len(full) != 2 {
		t.Fatalf("expected two full hash candidates, got %d", len(full))
	}
}

func TestHashWorkerCount(t *testing.T) {
	if got := hashWorkerCount(0); got != 0 {
		t.Fatalf("expected no workers for no work, got %d", got)
	}
	if got := hashWorkerCount(1); got != 1 {
		t.Fatalf("expected worker count to cap at total work, got %d", got)
	}
}
