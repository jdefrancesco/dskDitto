package fuzzy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHammingDistanceAndSimilarity(t *testing.T) {
	if d := HammingDistance(0b1010, 0b1010); d != 0 {
		t.Fatalf("expected distance 0, got %d", d)
	}
	if d := HammingDistance(0b1010, 0b1000); d != 1 {
		t.Fatalf("expected distance 1, got %d", d)
	}

	if s := SimilarityFromDistance(0); s != 100 {
		t.Fatalf("expected 100 similarity, got %d", s)
	}
	if s := SimilarityFromDistance(64); s != 0 {
		t.Fatalf("expected 0 similarity, got %d", s)
	}
}

func TestMaxDistanceForSimilarity(t *testing.T) {
	if got := MaxDistanceForSimilarity(100); got != 0 {
		t.Fatalf("expected max distance 0 for 100%%, got %d", got)
	}
	if got := MaxDistanceForSimilarity(85); got != 9 {
		t.Fatalf("expected max distance 9 for 85%%, got %d", got)
	}
	if got := MaxDistanceForSimilarity(0); got != 64 {
		t.Fatalf("expected max distance 64 for 0%%, got %d", got)
	}
}

func TestBKTreeSearchNearby(t *testing.T) {
	tree := &bkTree{}
	tree.Insert(0b0000, 0)
	tree.Insert(0b0001, 1)
	tree.Insert(0b1111, 2)

	ids := tree.Search(0b0000, 1)
	if len(ids) != 2 {
		t.Fatalf("expected 2 nearby ids, got %d", len(ids))
	}
}

func TestFindSimilarGroupsContentOnly(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "a.bin")
	near := filepath.Join(tmp, "b.bin")
	far := filepath.Join(tmp, "c.bin")

	baseData := []byte("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	nearData := []byte("alpha beta gamma delta epsilon zeta eta theta iota kappa lambdA")
	farData := []byte("completely different payload with unrelated tokens and bytes")

	if err := os.WriteFile(base, baseData, 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(near, nearData, 0o644); err != nil {
		t.Fatalf("write near: %v", err)
	}
	if err := os.WriteFile(far, farData, 0o644); err != nil {
		t.Fatalf("write far: %v", err)
	}

	candidates := []Candidate{
		{Path: base, Size: int64(len(baseData))},
		{Path: near, Size: int64(len(nearData))},
		{Path: far, Size: int64(len(farData))},
	}

	res, err := FindSimilarGroups(candidates, Options{MinSimilarity: 70, MinGroupSize: 2})
	if err != nil {
		t.Fatalf("FindSimilarGroups failed: %v", err)
	}
	if len(res.Groups) == 0 {
		t.Fatalf("expected at least one fuzzy group")
	}

	group := res.Groups[0]
	if len(group.Matches) < 2 {
		t.Fatalf("expected group with at least 2 files, got %d", len(group.Matches))
	}
}

func TestFindSimilarGroupsSameExtFilter(t *testing.T) {
	tmp := t.TempDir()
	textA := filepath.Join(tmp, "a.txt")
	textB := filepath.Join(tmp, "b.md")

	data := []byte("same content data for ext filtering test")
	if err := os.WriteFile(textA, data, 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(textB, data, 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	res, err := FindSimilarGroups([]Candidate{
		{Path: textA, Size: int64(len(data))},
		{Path: textB, Size: int64(len(data))},
	}, Options{MinSimilarity: 90, MinGroupSize: 2, SameExt: true})
	if err != nil {
		t.Fatalf("FindSimilarGroups failed: %v", err)
	}
	if len(res.Groups) != 0 {
		t.Fatalf("expected no groups with same-ext filter, got %d", len(res.Groups))
	}
}

func TestFindSimilarGroupsSkipsMissingFiles(t *testing.T) {
	res, err := FindSimilarGroups([]Candidate{{Path: "/definitely/missing/file.bin", Size: 12}}, Options{})
	if err != nil {
		t.Fatalf("expected skip, got error: %v", err)
	}
	if res.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", res.Skipped)
	}
}
