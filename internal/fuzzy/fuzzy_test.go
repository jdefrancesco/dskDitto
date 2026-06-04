package fuzzy

import (
	"bytes"
	"fmt"
	"hash/fnv"
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

func TestHashTokenMatchesFNV64a(t *testing.T) {
	tokens := [][]byte{
		[]byte("abcd"),
		[]byte("wxyz"),
		[]byte{0x00, 0xff, 0x12, 0x34},
		[]byte("a"),
	}
	for _, token := range tokens {
		got := hashToken(token)
		want := fnv64a(token)
		if got != want {
			t.Fatalf("hash mismatch for %q: got %d want %d", token, got, want)
		}
	}
}

func fnv64a(token []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(token)
	return h.Sum64()
}

func BenchmarkSignatureFromFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "blob.bin")
	payload := bytes.Repeat([]byte("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda\n"), 4096)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		b.Fatalf("write payload: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := SignatureFromFile(path, DefaultMaxReadBytes); err != nil {
			b.Fatalf("SignatureFromFile: %v", err)
		}
	}
}

func BenchmarkFindSimilarGroups(b *testing.B) {
	dir := b.TempDir()
	candidates := make([]Candidate, 0, 512)
	for i := 0; i < 512; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file-%03d.bin", i))
		payload := bytes.Repeat([]byte(fmt.Sprintf("group-%d near duplicate content block\n", i%16)), 1536)
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			b.Fatalf("write file %d: %v", i, err)
		}
		candidates = append(candidates, Candidate{Path: path, Size: int64(len(payload))})
	}

	opts := Options{
		MinSimilarity: 75,
		MinGroupSize:  2,
		MaxReadBytes:  DefaultMaxReadBytes,
		MaxCandidates: -1, // no cap in benchmark
	}
	b.ResetTimer()
	for b.Loop() {
		res, err := FindSimilarGroups(candidates, opts)
		if err != nil {
			b.Fatalf("FindSimilarGroups: %v", err)
		}
		if len(res.Groups) == 0 {
			b.Fatalf("expected at least one fuzzy group")
		}
	}
}

func TestFindSimilarGroupsMaxCandidatesTruncation(t *testing.T) {
	tmp := t.TempDir()
	candidates := make([]Candidate, 5)
	for i := range candidates {
		path := filepath.Join(tmp, fmt.Sprintf("file%d.bin", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("content %d", i)), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		candidates[i] = Candidate{Path: path, Size: 10}
	}

	res, err := FindSimilarGroups(candidates, Options{
		MinSimilarity: 75,
		MinGroupSize:  2,
		MaxCandidates: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CandidatesTruncated != 2 {
		t.Fatalf("expected CandidatesTruncated=2, got %d", res.CandidatesTruncated)
	}
}
