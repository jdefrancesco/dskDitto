//go:build !unix

package manifest

import (
	"io/fs"
	"os"
)

func openCreateTruncate(absPath string, perm fs.FileMode) (*os.File, error) {
	// #nosec G304 -- non-unix fallback retains scoped caller behavior from normalized absolute paths.
	return os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
}

func openCreateExclusive(absPath string, perm fs.FileMode) (*os.File, error) {
	// #nosec G304 -- non-unix fallback retains scoped caller behavior from normalized absolute paths.
	return os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
}

func syncDir(path string) error {
	// #nosec G304 -- non-unix fallback syncing parent directory metadata after atomic rename.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
