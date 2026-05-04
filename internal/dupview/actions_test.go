package dupview

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
)

type testFileSpec struct {
	name   string
	marked bool
}

func initDupviewTestLogger() {
	dsklog.InitializeDlogger("/dev/null")
}

func mustWriteDupFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustHashDigest(t *testing.T, path string) dmap.Digest {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	df, err := dfs.NewDfile(path, info.Size(), dfs.HashSHA256)
	if err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return dmap.Digest(df.Hash())
}

func newTestGroup(t *testing.T, dir string, content string, specs []testFileSpec) (*Group, []string) {
	t.Helper()

	paths := make([]string, 0, len(specs))
	group := &Group{Title: "test-group"}
	for _, spec := range specs {
		path := filepath.Join(dir, spec.name)
		mustWriteDupFile(t, path, content)
		group.Files = append(group.Files, &FileEntry{
			Path:   path,
			Marked: spec.marked,
			Status: FileStatusPending,
		})
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		t.Fatalf("newTestGroup requires at least one file")
	}
	group.Hash = mustHashDigest(t, paths[0])
	return group, paths
}

func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs %s: %v", path, err)
	}
	return abs
}

func restorePaths(entries []manifest.Entry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.RestorePath)
	}
	sort.Strings(paths)
	return paths
}

func TestApplyMarkedDeleteBackupEntriesOnlyChangedPaths(t *testing.T) {
	initDupviewTestLogger()

	dir := t.TempDir()
	group, paths := newTestGroup(t, dir, "same-content", []testFileSpec{
		{name: "a.bin", marked: false},
		{name: "b.bin", marked: true},
		{name: "c.bin", marked: false},
	})
	manifestPath := filepath.Join(dir, "restore.jsonl")

	result, err := ApplyMarked([]*Group{group}, ActionDelete, ApplyOptions{
		BackupPath:    manifestPath,
		HashAlgorithm: dfs.HashSHA256,
	})
	if err != nil {
		t.Fatalf("ApplyMarked: %v", err)
	}
	if result != "Deleted 1 file(s)." {
		t.Fatalf("unexpected result: %q", result)
	}

	entries, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Read manifest: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected manifest entry count: got %d want 1", len(entries))
	}
	if entries[0].Canonical != absPath(t, paths[0]) {
		t.Fatalf("unexpected canonical path: got %s want %s", entries[0].Canonical, absPath(t, paths[0]))
	}
	if entries[0].RestorePath != absPath(t, paths[1]) {
		t.Fatalf("unexpected restore path: got %s want %s", entries[0].RestorePath, absPath(t, paths[1]))
	}
	if _, err := os.Stat(paths[2]); err != nil {
		t.Fatalf("untouched file should remain: %v", err)
	}
}

func TestApplyMarkedLinkBackupUsesCurrentUnmarkedTarget(t *testing.T) {
	initDupviewTestLogger()

	dir := t.TempDir()
	group, paths := newTestGroup(t, dir, "same-content", []testFileSpec{
		{name: "a.bin", marked: true},
		{name: "b.bin", marked: false},
		{name: "c.bin", marked: true},
	})
	manifestPath := filepath.Join(dir, "restore.jsonl")

	result, err := ApplyMarked([]*Group{group}, ActionLink, ApplyOptions{
		BackupPath:    manifestPath,
		HashAlgorithm: dfs.HashSHA256,
	})
	if err != nil {
		t.Fatalf("ApplyMarked: %v", err)
	}
	if result != "Converted 2 file(s) to symlinks." {
		t.Fatalf("unexpected result: %q", result)
	}

	entries, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Read manifest: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected manifest entry count: got %d want 2", len(entries))
	}
	wantCanonical := absPath(t, paths[1])
	for _, entry := range entries {
		if entry.Canonical != wantCanonical {
			t.Fatalf("unexpected canonical path: got %s want %s", entry.Canonical, wantCanonical)
		}
	}
	gotRestorePaths := restorePaths(entries)
	wantRestorePaths := []string{absPath(t, paths[0]), absPath(t, paths[2])}
	sort.Strings(wantRestorePaths)
	if len(gotRestorePaths) != len(wantRestorePaths) || gotRestorePaths[0] != wantRestorePaths[0] || gotRestorePaths[1] != wantRestorePaths[1] {
		t.Fatalf("unexpected restore paths: got %v want %v", gotRestorePaths, wantRestorePaths)
	}

	for _, path := range []string{paths[0], paths[2]} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %s: %v", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s should be a symlink after linking", path)
		}
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatalf("readlink %s: %v", path, err)
		}
		if target != paths[1] {
			t.Fatalf("unexpected symlink target for %s: got %s want %s", path, target, paths[1])
		}
	}
}

func TestApplyMarkedDeleteBackupUsesSurvivingUnmarkedTarget(t *testing.T) {
	initDupviewTestLogger()

	dir := t.TempDir()
	group, paths := newTestGroup(t, dir, "same-content", []testFileSpec{
		{name: "a.bin", marked: true},
		{name: "b.bin", marked: false},
		{name: "c.bin", marked: true},
	})
	manifestPath := filepath.Join(dir, "restore.jsonl")

	result, err := ApplyMarked([]*Group{group}, ActionDelete, ApplyOptions{
		BackupPath:    manifestPath,
		HashAlgorithm: dfs.HashSHA256,
	})
	if err != nil {
		t.Fatalf("ApplyMarked: %v", err)
	}
	if result != "Deleted 2 file(s)." {
		t.Fatalf("unexpected result: %q", result)
	}

	entries, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Read manifest: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected manifest entry count: got %d want 2", len(entries))
	}
	wantCanonical := absPath(t, paths[1])
	for _, entry := range entries {
		if entry.Canonical != wantCanonical {
			t.Fatalf("unexpected canonical path: got %s want %s", entry.Canonical, wantCanonical)
		}
	}
}

func TestApplyMarkedDeleteBackupAbortsWhenAllFilesMarked(t *testing.T) {
	initDupviewTestLogger()

	dir := t.TempDir()
	group, paths := newTestGroup(t, dir, "same-content", []testFileSpec{
		{name: "a.bin", marked: true},
		{name: "b.bin", marked: true},
	})
	manifestPath := filepath.Join(dir, "restore.jsonl")

	result, err := ApplyMarked([]*Group{group}, ActionDelete, ApplyOptions{
		BackupPath:    manifestPath,
		HashAlgorithm: dfs.HashSHA256,
	})
	if err == nil {
		t.Fatalf("expected error when every file is marked")
	}
	if result != "" {
		t.Fatalf("unexpected result on failure: %q", result)
	}
	for _, path := range paths {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("path should remain untouched on failure: %s: %v", path, statErr)
		}
	}
	if _, statErr := os.Stat(manifestPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("manifest should not be written on failure, got stat err: %v", statErr)
	}
}

func TestApplyMarkedBackupWriteFailureAbortsBeforeMutation(t *testing.T) {
	initDupviewTestLogger()

	dir := t.TempDir()
	group, paths := newTestGroup(t, dir, "same-content", []testFileSpec{
		{name: "a.bin", marked: false},
		{name: "b.bin", marked: true},
	})
	manifestPath := filepath.Join(dir, "manifest-dir")
	if err := os.Mkdir(manifestPath, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}

	result, err := ApplyMarked([]*Group{group}, ActionDelete, ApplyOptions{
		BackupPath:    manifestPath,
		HashAlgorithm: dfs.HashSHA256,
	})
	if err == nil {
		t.Fatalf("expected manifest write failure")
	}
	if result != "" {
		t.Fatalf("unexpected result on failure: %q", result)
	}
	if _, statErr := os.Stat(paths[1]); statErr != nil {
		t.Fatalf("marked file should remain after backup write failure: %v", statErr)
	}
}
