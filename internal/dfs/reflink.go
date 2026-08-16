package dfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrReflinkUnsupported indicates the current platform, or the filesystem
// backing the source/destination paths, does not support copy-on-write
// reflink clones (e.g. APFS, Btrfs, or XFS with reflink=1).
var ErrReflinkUnsupported = errors.New("reflink not supported on this filesystem/platform")

// ReflinkSupported reports whether this platform has a reflink clone
// syscall available at all. It does not guarantee the filesystem backing
// any particular path supports cloning; callers should still handle
// ErrReflinkUnsupported from Reflink/ReflinkReplace.
func ReflinkSupported() bool {
	return reflinkSupported
}

// Reflink creates dst as a copy-on-write clone of src. dst must not already
// exist. Returns ErrReflinkUnsupported (wrapped) if the platform or the
// backing filesystem cannot perform the clone.
func Reflink(dst, src string) error {
	if src == "" || dst == "" {
		return errors.New("reflink source and destination paths must not be empty")
	}
	return reflink(dst, src)
}

// ReflinkReplace atomically replaces the file at path with a copy-on-write
// clone of target. It clones into a temporary file in the same directory as
// path, then renames over path so a failed clone never leaves path missing.
// Returns ErrReflinkUnsupported (wrapped) if cloning isn't possible here.
func ReflinkReplace(path, target string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dskditto-reflink-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file %s: %w", tmpPath, err)
	}
	// Reflink requires the destination not to exist; the temp file only
	// reserved a unique name for us.
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("prepare temp file %s: %w", tmpPath, err)
	}

	if err := Reflink(tmpPath, target); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
