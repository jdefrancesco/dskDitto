package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/internal/fuzzy"
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
		t.Fatalf("expected no workers for no work, got %d", got)
	}
	if got := hashWorkerCount(1); got != 1 {
		t.Fatalf("expected worker count to cap at total work, got %d", got)
	}
}

func TestValidateRestoreModeAcceptsValidInvocation(t *testing.T) {
	err := validateRestoreMode("restore.jsonl", "", nil, false, false, false, "", "", "", "", false, 0, false)
	if err != nil {
		t.Fatalf("expected valid restore invocation, got: %v", err)
	}
}

func TestValidateRestoreModeRejectsPathArgs(t *testing.T) {
	err := validateRestoreMode("restore.jsonl", "", []string{"."}, false, false, false, "", "", "", "", false, 0, false)
	if err == nil {
		t.Fatalf("expected error when path args are provided")
	}
	if !strings.Contains(err.Error(), "path arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRestoreModeRejectsScanFlags(t *testing.T) {
	err := validateRestoreMode("restore.jsonl", "", nil, true, false, false, "", "", "", "", false, 0, false)
	if err == nil {
		t.Fatalf("expected error when scan flags are provided")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddNameOnlyGroups(t *testing.T) {
	dsklog.InitializeDlogger(filepath.Join(t.TempDir(), "test.log"))

	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatalf("NewDmap failed: %v", err)
	}

	groups := map[string][]dwalk.FileCandidate{
		"same.txt": {
			{Path: "/tmp/b/same.txt"},
			{Path: "/tmp/a/same.txt"},
		},
		"unique.txt": {
			{Path: "/tmp/a/unique.txt"},
		},
	}

	added, skipped := addNameOnlyGroups(dm, groups, 2)
	if added != 1 {
		t.Fatalf("expected one added group, got %d", added)
	}
	if skipped != 1 {
		t.Fatalf("expected one skipped file, got %d", skipped)
	}

	files, _ := dm.Get(dmap.NameDigest("same.txt"))
	if len(files) != 2 {
		t.Fatalf("expected two same-name files, got %d", len(files))
	}
	if files[0] != "/tmp/a/same.txt" {
		t.Fatalf("expected stable path ordering, got %#v", files)
	}
	if _, ok := dm.GetMap()[dmap.NameDigest("unique.txt")]; ok {
		t.Fatalf("did not expect unique filename group")
	}
}

func TestValidateShallowModeRejectsBackup(t *testing.T) {
	_, err := validateShallowMode(true, "", "", "restore.jsonl")
	if err == nil {
		t.Fatalf("expected backup rejection for shallow mode")
	}
	if !strings.Contains(err.Error(), "restore backups are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateShallowModeUsesFileWithNameOnly(t *testing.T) {
	name, err := validateShallowMode(true, "", "/tmp/dir/target.txt", "")
	if err != nil {
		t.Fatalf("validateShallowMode failed: %v", err)
	}
	if name != "target.txt" {
		t.Fatalf("expected target basename, got %q", name)
	}
}

func TestValidateShallowModeRejectsFileAndFileShallow(t *testing.T) {
	_, err := validateShallowMode(true, "target.txt", "content-target.txt", "")
	if err == nil {
		t.Fatalf("expected --file and --file-shallow rejection")
	}
}

func TestValidateShallowModeReturnsFileShallowBasename(t *testing.T) {
	name, err := validateShallowMode(false, "/tmp/dir/text1", "", "")
	if err != nil {
		t.Fatalf("validateShallowMode failed: %v", err)
	}
	if name != "text1" {
		t.Fatalf("expected basename text1, got %q", name)
	}
}

func TestResolveSkipHiddenIncludesHiddenShallowTarget(t *testing.T) {
	if resolveSkipHidden(false, ".dskditto.log") {
		t.Fatalf("expected hidden shallow target to include hidden entries")
	}
}

func TestResolveSkipHiddenKeepsDefaultForNonHiddenShallowTarget(t *testing.T) {
	if !resolveSkipHidden(false, "dskditto.log") {
		t.Fatalf("expected non-hidden shallow target to preserve hidden skipping")
	}
}

func TestResolveSkipHiddenHonorsHiddenFlag(t *testing.T) {
	if resolveSkipHidden(true, "") {
		t.Fatalf("expected --hidden to include hidden entries")
	}
}

func TestValidateFuzzyModeRejectsShallowModes(t *testing.T) {
	err := validateFuzzyMode(true, true, "", "", "", 0, false, 85)
	if err == nil {
		t.Fatalf("expected shallow/fuzzy incompatibility")
	}
}

func TestValidateFuzzyModeRejectsMutations(t *testing.T) {
	err := validateFuzzyMode(true, false, "", "", "", 1, false, 85)
	if err == nil {
		t.Fatalf("expected remove/fuzzy incompatibility")
	}

	err = validateFuzzyMode(true, false, "", "", "", 0, true, 85)
	if err == nil {
		t.Fatalf("expected link/fuzzy incompatibility")
	}
}

func TestValidateFuzzyModeRejectsInvalidThreshold(t *testing.T) {
	err := validateFuzzyMode(true, false, "", "", "", 0, false, 120)
	if err == nil {
		t.Fatalf("expected invalid threshold rejection")
	}
}

func TestAddFuzzyContentGroups(t *testing.T) {
	dsklog.InitializeDlogger(filepath.Join(t.TempDir(), "test.log"))

	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatalf("NewDmap failed: %v", err)
	}

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.bin")
	b := filepath.Join(tmp, "b.bin")
	c := filepath.Join(tmp, "c.bin")

	aData := []byte("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	bData := []byte("alpha beta gamma delta epsilon zeta eta theta iota kappa lambdA")
	cData := []byte("totally different content payload that should not cluster")

	if err := os.WriteFile(a, aData, 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, bData, 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(c, cData, 0o644); err != nil {
		t.Fatalf("write c: %v", err)
	}

	added, processed, skipped, err := addFuzzyContentGroups(dm, []dwalk.FileCandidate{
		{Path: a, Size: int64(len(aData))},
		{Path: b, Size: int64(len(bData))},
		{Path: c, Size: int64(len(cData))},
	}, 2, 70, false, fuzzy.DefaultMaxFuzzyCandidates, nil, nil)
	if err != nil {
		t.Fatalf("addFuzzyContentGroups failed: %v", err)
	}
	if processed == 0 {
		t.Fatalf("expected processed files > 0")
	}
	if skipped > 0 {
		t.Fatalf("expected no skipped files, got %d", skipped)
	}
	if added == 0 {
		t.Fatalf("expected at least one fuzzy group")
	}

	foundFuzzy := false
	for hash := range dm.GetMap() {
		if dm.MatchInfo(hash).Type == dmap.MatchFuzzy {
			foundFuzzy = true
			break
		}
	}
	if !foundFuzzy {
		t.Fatalf("expected fuzzy match type in dmap")
	}
}
