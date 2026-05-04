package manifest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
)

var (
	ErrHashMismatch = errors.New("hash mismatch")
	ErrSizeMismatch = errors.New("size mismatch")
)

func RestoreManifest(path string, opts RestoreOptions) error {
	entries, err := Read(path)
	if err != nil {
		return err
	}

	var allErrs []error
	for i, entry := range entries {
		if err := restoreEntry(i, entry, opts); err != nil {
			allErrs = append(allErrs, err)
		}
	}
	if len(allErrs) > 0 {
		return errors.Join(allErrs...)
	}
	return nil
}

func restoreEntry(index int, entry Entry, opts RestoreOptions) error {
	if entry.Version != 0 && entry.Version != ManifestVersion {
		return fmt.Errorf("entry %d has unsupported version %d", index+1, entry.Version)
	}
	if entry.Canonical == "" {
		return fmt.Errorf("entry %d has empty canonical path", index+1)
	}
	if entry.RestorePath == "" {
		return fmt.Errorf("entry %d has empty restore path", index+1)
	}

	algo, err := dfs.ParseHashAlgorithm(entry.HashAlgo)
	if err != nil {
		return fmt.Errorf("entry %d has invalid hash algorithm %q: %w", index+1, entry.HashAlgo, err)
	}

	canonicalPath, err := filepath.Abs(filepath.Clean(entry.Canonical))
	if err != nil {
		return fmt.Errorf("entry %d canonical path resolution failed: %w", index+1, err)
	}
	restorePath, err := filepath.Abs(filepath.Clean(entry.RestorePath))
	if err != nil {
		return fmt.Errorf("entry %d restore path resolution failed: %w", index+1, err)
	}

	canonicalInfo, err := os.Stat(canonicalPath)
	if err != nil {
		return fmt.Errorf("entry %d canonical stat failed %s: %w", index+1, canonicalPath, err)
	}
	if !canonicalInfo.Mode().IsRegular() {
		return fmt.Errorf("entry %d canonical path is not a regular file: %s", index+1, canonicalPath)
	}

	if opts.VerifyHash {
		if err := VerifyFileHash(canonicalPath, entry.Hash, entry.Size, algo); err != nil {
			return fmt.Errorf("entry %d canonical verify failed: %w", index+1, err)
		}
	}

	restoreInfo, restoreStatErr := os.Lstat(restorePath)
	if restoreStatErr == nil {
		if restoreInfo.Mode()&os.ModeSymlink != 0 {
			if opts.DryRun {
				return nil
			}
			if err := os.Remove(restorePath); err != nil {
				return fmt.Errorf("entry %d remove existing restore symlink %s: %w", index+1, restorePath, err)
			}
		} else if !restoreInfo.Mode().IsRegular() {
			return fmt.Errorf("entry %d restore path exists but is not a regular file: %s", index+1, restorePath)
		} else {
			verifyErr := VerifyFileHash(restorePath, entry.Hash, entry.Size, algo)
			if verifyErr == nil {
				return nil
			}
			if !errors.Is(verifyErr, ErrHashMismatch) && !errors.Is(verifyErr, ErrSizeMismatch) {
				return fmt.Errorf("entry %d restore-path verify failed: %w", index+1, verifyErr)
			}
			if !opts.Overwrite {
				return fmt.Errorf("entry %d restore path exists with different content and overwrite is disabled: %s", index+1, restorePath)
			}
			if opts.DryRun {
				return nil
			}
			if err := os.Remove(restorePath); err != nil {
				return fmt.Errorf("entry %d remove existing restore path %s: %w", index+1, restorePath, err)
			}
		}
	} else if !errors.Is(restoreStatErr, os.ErrNotExist) {
		return fmt.Errorf("entry %d restore path stat failed %s: %w", index+1, restorePath, restoreStatErr)
	}

	if opts.DryRun {
		return nil
	}

	mode := canonicalInfo.Mode().Perm()
	if opts.RestoreMode && entry.Mode != 0 {
		mode = fs.FileMode(entry.Mode & 0o777)
	}

	if err := CopyFile(canonicalPath, restorePath, mode); err != nil {
		return fmt.Errorf("entry %d restore copy failed: %w", index+1, err)
	}

	if opts.RestoreMTime && entry.ModTimeUnix != 0 {
		mt := time.Unix(entry.ModTimeUnix, entry.ModTimeNsec)
		if err := os.Chtimes(restorePath, mt, mt); err != nil {
			return fmt.Errorf("entry %d restore mtime failed: %w", index+1, err)
		}
	}

	return nil
}

func VerifyFileHash(path, expectedHash string, expectedSize int64, algo dfs.HashAlgorithm) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if expectedSize >= 0 && info.Size() != expectedSize {
		return fmt.Errorf("%w for %s: expected %d got %d", ErrSizeMismatch, path, expectedSize, info.Size())
	}

	dfile, err := dfs.NewDfile(path, info.Size(), algo)
	if err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	got := dfile.HashString()
	if !strings.EqualFold(got, expectedHash) {
		return fmt.Errorf("%w for %s: expected %s got %s", ErrHashMismatch, path, expectedHash, got)
	}
	return nil
}

func CopyFile(src, dst string, mode fs.FileMode) error {
	srcFile, err := os.Open(src) // #nosec G304 -- src is manifest-controlled restore input
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return fmt.Errorf("create destination directory %s: %w", dstDir, err)
	}

	dstFile, err := openCreateExclusive(dst, mode)
	if err != nil {
		return fmt.Errorf("open destination %s: %w", dst, err)
	}

	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(dstFile, srcFile, buf); err != nil {
		_ = dstFile.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("sync destination %s: %w", dst, err)
	}
	if err := dstFile.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("close destination %s: %w", dst, err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod destination %s: %w", dst, err)
	}
	return nil
}
