package manifest

// The two most important test functions begin around line 97. Helper functions
// keep toward the beginning.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
)

func initTestLogger() {
	dsklog.InitializeDlogger("/dev/null")
}

// Write a file
func mustWriteFile(t *testing.T, path string, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fileHash(t *testing.T, path string, algo dfs.HashAlgorithm) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	df, err := dfs.NewDfile(path, info.Size(), algo)
	if err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return df.HashString()
}

func TestManifestWriteReadRoundTrip(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "a", "file1.bin")
	dup := filepath.Join(dir, "b", "file1-copy.bin")
	mustWriteFile(t, canonical, "same-data", 0o644)
	mustWriteFile(t, dup, "same-data", 0o640)

	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatalf("NewDmap: %v", err)
	}
	df, err := dfs.NewDfile(canonical, int64(len("same-data")), dfs.HashSHA256)
	if err != nil {
		t.Fatalf("hash canonical: %v", err)
	}
	digest := dmap.Digest(df.Hash())
	dm.AddPath(digest, dup)
	dm.AddPath(digest, canonical)

	entries, err := EntriesFromDmap(dm, dfs.HashSHA256)
	if err != nil {
		t.Fatalf("EntriesFromDmap: %v", err)
	}
	if got, want := len(entries), 1; got != want {
		t.Fatalf("entry count mismatch: got %d want %d", got, want)
	}

	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, entries); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}

	readBack, err := Read(manifestPath)
	if err != nil {
		t.Fatalf("Read manifest: %v", err)
	}
	if got, want := len(readBack), 1; got != want {
		t.Fatalf("read entry count mismatch: got %d want %d", got, want)
	}
	if readBack[0].Canonical != entries[0].Canonical {
		t.Fatalf("canonical mismatch: got %s want %s", readBack[0].Canonical, entries[0].Canonical)
	}
	if readBack[0].RestorePath != entries[0].RestorePath {
		t.Fatalf("restore path mismatch: got %s want %s", readBack[0].RestorePath, entries[0].RestorePath)
	}
	if readBack[0].HashAlgo != string(dfs.HashSHA256) {
		t.Fatalf("hash algorithm mismatch: got %s", readBack[0].HashAlgo)
	}
}

func TestCanonicalSelectionDeterministic(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	pathA := filepath.Join(dir, "a", "f.txt")
	pathB := filepath.Join(dir, "b", "f.txt")
	pathC := filepath.Join(dir, "c", "f.txt")
	mustWriteFile(t, pathA, "dup", 0o644)
	mustWriteFile(t, pathB, "dup", 0o644)
	mustWriteFile(t, pathC, "dup", 0o644)

	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatalf("NewDmap: %v", err)
	}
	var digest dmap.Digest
	digest[0] = 0x1
	dm.AddPath(digest, pathC)
	dm.AddPath(digest, pathB)
	dm.AddPath(digest, pathA)

	entries, err := EntriesFromDmap(dm, dfs.HashSHA256)
	if err != nil {
		t.Fatalf("EntriesFromDmap: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 restore entries, got %d", len(entries))
	}

	absA, _ := filepath.Abs(pathA)
	absB, _ := filepath.Abs(pathB)
	absC, _ := filepath.Abs(pathC)

	for _, entry := range entries {
		if entry.Canonical != absA {
			t.Fatalf("expected canonical %s, got %s", absA, entry.Canonical)
		}
	}

	got := []string{entries[0].RestorePath, entries[1].RestorePath}
	wantFirst := absB
	wantSecond := absC
	if got[0] != wantFirst || got[1] != wantSecond {
		t.Fatalf("unexpected restore path order: got %v want [%s %s]", got, wantFirst, wantSecond)
	}
}

func TestRestoreMissingPathCreatesFile(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "keep.bin")
	restorePath := filepath.Join(dir, "restore", "copy.bin")
	mustWriteFile(t, canonical, "payload-123", 0o640)

	entry := Entry{
		Version:     ManifestVersion,
		GroupID:     1,
		HashAlgo:    string(dfs.HashSHA256),
		Hash:        fileHash(t, canonical, dfs.HashSHA256),
		Size:        int64(len("payload-123")),
		Canonical:   canonical,
		RestorePath: restorePath,
		Mode:        0o640,
		ModTimeUnix: time.Now().Unix(),
		ModTimeNsec: 0,
	}

	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, []Entry{entry}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	opts := RestoreOptions{
		VerifyHash:   true,
		Overwrite:    false,
		RestoreMode:  true,
		RestoreMTime: true,
	}
	if err := RestoreManifest(manifestPath, opts); err != nil {
		t.Fatalf("RestoreManifest: %v", err)
	}

	data, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatalf("read restore path: %v", err)
	}
	if string(data) != "payload-123" {
		t.Fatalf("unexpected restored content: %q", data)
	}
}

func TestRestoreExistingIdenticalSkips(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "keep.bin")
	restorePath := filepath.Join(dir, "dup.bin")
	mustWriteFile(t, canonical, "same-content", 0o644)
	mustWriteFile(t, restorePath, "same-content", 0o644)

	before, err := os.Stat(restorePath)
	if err != nil {
		t.Fatalf("stat restore before: %v", err)
	}

	entry := Entry{
		Version:     ManifestVersion,
		GroupID:     1,
		HashAlgo:    string(dfs.HashSHA256),
		Hash:        fileHash(t, canonical, dfs.HashSHA256),
		Size:        int64(len("same-content")),
		Canonical:   canonical,
		RestorePath: restorePath,
	}
	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, []Entry{entry}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := RestoreManifest(manifestPath, RestoreOptions{VerifyHash: true}); err != nil {
		t.Fatalf("RestoreManifest: %v", err)
	}

	after, err := os.Stat(restorePath)
	if err != nil {
		t.Fatalf("stat restore after: %v", err)
	}
	if before.ModTime() != after.ModTime() {
		t.Fatalf("expected restore path to be untouched when identical")
	}
}

func TestRestoreExistingDifferentErrorsWithoutOverwrite(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "keep.bin")
	restorePath := filepath.Join(dir, "dup.bin")
	mustWriteFile(t, canonical, "canonical-content", 0o644)
	mustWriteFile(t, restorePath, "different-content", 0o644)

	entry := Entry{
		Version:     ManifestVersion,
		GroupID:     1,
		HashAlgo:    string(dfs.HashSHA256),
		Hash:        fileHash(t, canonical, dfs.HashSHA256),
		Size:        int64(len("canonical-content")),
		Canonical:   canonical,
		RestorePath: restorePath,
	}
	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, []Entry{entry}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	err := RestoreManifest(manifestPath, RestoreOptions{VerifyHash: true, Overwrite: false})
	if err == nil {
		t.Fatalf("expected restore error for existing conflicting file")
	}
	if !strings.Contains(err.Error(), "overwrite is disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestoreWithOverwriteReplacesFile(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "keep.bin")
	restorePath := filepath.Join(dir, "dup.bin")
	mustWriteFile(t, canonical, "canonical", 0o644)
	mustWriteFile(t, restorePath, "old-value", 0o600)

	entry := Entry{
		Version:     ManifestVersion,
		GroupID:     1,
		HashAlgo:    string(dfs.HashSHA256),
		Hash:        fileHash(t, canonical, dfs.HashSHA256),
		Size:        int64(len("canonical")),
		Canonical:   canonical,
		RestorePath: restorePath,
		Mode:        0o644,
	}
	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, []Entry{entry}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := RestoreManifest(manifestPath, RestoreOptions{
		VerifyHash:   true,
		Overwrite:    true,
		RestoreMode:  true,
		RestoreMTime: false,
	}); err != nil {
		t.Fatalf("RestoreManifest: %v", err)
	}

	data, err := os.ReadFile(restorePath)
	if err != nil {
		t.Fatalf("read restore path: %v", err)
	}
	if string(data) != "canonical" {
		t.Fatalf("overwrite did not replace content: %q", data)
	}
}

func TestVerifyHashFailureOnChangedCanonical(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "keep.bin")
	restorePath := filepath.Join(dir, "dup.bin")
	mustWriteFile(t, canonical, "original", 0o644)

	hash := fileHash(t, canonical, dfs.HashSHA256)
	entry := Entry{
		Version:     ManifestVersion,
		GroupID:     1,
		HashAlgo:    string(dfs.HashSHA256),
		Hash:        hash,
		Size:        int64(len("original")),
		Canonical:   canonical,
		RestorePath: restorePath,
	}
	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, []Entry{entry}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mustWriteFile(t, canonical, "changed", 0o644)

	err := RestoreManifest(manifestPath, RestoreOptions{VerifyHash: true})
	if err == nil {
		t.Fatalf("expected canonical verify failure")
	}
	if !errors.Is(err, ErrHashMismatch) && !errors.Is(err, ErrSizeMismatch) {
		if !strings.Contains(err.Error(), "canonical verify failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestManifestRemoveRestoreRoundTrip(t *testing.T) {
	initTestLogger()

	dir := t.TempDir()
	canonical := filepath.Join(dir, "a", "file1.bin")
	dup := filepath.Join(dir, "b", "file1-copy.bin")
	unique := filepath.Join(dir, "c", "unique.bin")
	mustWriteFile(t, canonical, "same-data", 0o644)
	mustWriteFile(t, dup, "same-data", 0o644)
	mustWriteFile(t, unique, "different", 0o644)

	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatalf("NewDmap: %v", err)
	}

	infoA, _ := os.Stat(canonical)
	infoB, _ := os.Stat(dup)
	infoU, _ := os.Stat(unique)
	dfA, err := dfs.NewDfile(canonical, infoA.Size(), dfs.HashSHA256)
	if err != nil {
		t.Fatalf("hash canonical: %v", err)
	}
	dfB, err := dfs.NewDfile(dup, infoB.Size(), dfs.HashSHA256)
	if err != nil {
		t.Fatalf("hash duplicate: %v", err)
	}
	dfU, err := dfs.NewDfile(unique, infoU.Size(), dfs.HashSHA256)
	if err != nil {
		t.Fatalf("hash unique: %v", err)
	}

	dm.AddPath(dmap.Digest(dfA.Hash()), canonical)
	dm.AddPath(dmap.Digest(dfB.Hash()), dup)
	dm.AddPath(dmap.Digest(dfU.Hash()), unique)

	if err := CanonicalizeDmapGroups(dm); err != nil {
		t.Fatalf("CanonicalizeDmapGroups: %v", err)
	}
	entries, err := EntriesFromDmap(dm, dfs.HashSHA256)
	if err != nil {
		t.Fatalf("EntriesFromDmap: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one restore entry for duplicate pair, got %d", len(entries))
	}

	manifestPath := filepath.Join(dir, "restore.jsonl")
	if err := Write(manifestPath, entries); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}

	removed, err := dm.RemoveDuplicates(1)
	if err != nil {
		t.Fatalf("RemoveDuplicates: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected one removed duplicate, got %d", len(removed))
	}
	if _, statErr := os.Stat(dup); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected duplicate to be removed before restore, stat err: %v", statErr)
	}

	if err := RestoreManifest(manifestPath, RestoreOptions{VerifyHash: true}); err != nil {
		t.Fatalf("RestoreManifest: %v", err)
	}

	restoredHash := fileHash(t, dup, dfs.HashSHA256)
	canonicalHash := fileHash(t, canonical, dfs.HashSHA256)
	if restoredHash != canonicalHash {
		t.Fatalf("restored hash mismatch: got %s want %s", restoredHash, canonicalHash)
	}
}
