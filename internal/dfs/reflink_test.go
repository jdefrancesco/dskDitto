package dfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReflinkReplaceClonesContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.bin")
	dup := filepath.Join(dir, "dup.bin")
	content := []byte("same-content-for-reflink-test")

	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(dup, content, 0o644); err != nil {
		t.Fatalf("write dup: %v", err)
	}

	err := ReflinkReplace(dup, target)
	if err != nil {
		if errors.Is(err, ErrReflinkUnsupported) {
			t.Skipf("reflink not supported on this filesystem/platform: %v", err)
		}
		t.Fatalf("ReflinkReplace: %v", err)
	}

	got, err := os.ReadFile(dup)
	if err != nil {
		t.Fatalf("read dup after reflink: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("unexpected content after reflink: got %q want %q", got, content)
	}

	// The cloned path should remain a regular file (not a symlink) and
	// independently addressable, unlike the symlink strategy.
	info, err := os.Lstat(dup)
	if err != nil {
		t.Fatalf("lstat dup: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("reflinked path should not be a symlink")
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("reflinked path should be a regular file")
	}
}

func TestReflinkReplaceMissingTargetFails(t *testing.T) {
	dir := t.TempDir()
	dup := filepath.Join(dir, "dup.bin")
	if err := os.WriteFile(dup, []byte("x"), 0o644); err != nil {
		t.Fatalf("write dup: %v", err)
	}

	err := ReflinkReplace(dup, filepath.Join(dir, "does-not-exist.bin"))
	if err == nil {
		t.Fatalf("expected error when target does not exist")
	}
	// The original duplicate must survive an aborted clone attempt.
	if _, statErr := os.Stat(dup); statErr != nil {
		t.Fatalf("dup should remain untouched on failure: %v", statErr)
	}
}

func TestReflinkRejectsEmptyPaths(t *testing.T) {
	if err := Reflink("", "src"); err == nil {
		t.Fatalf("expected error for empty dst")
	}
	if err := Reflink("dst", ""); err == nil {
		t.Fatalf("expected error for empty src")
	}
}
