//go:build unix

package manifest

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openCreateTruncate(absPath string, perm fs.FileMode) (*os.File, error) {
	return openInParent(absPath, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC, perm)
}

func openCreateExclusive(absPath string, perm fs.FileMode) (*os.File, error) {
	return openInParent(absPath, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, perm)
}

func syncDir(path string) error {
	dirfd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", path, err)
	}
	defer func() {
		_ = unix.Close(dirfd)
	}()
	if err := unix.Fsync(dirfd); err != nil {
		return fmt.Errorf("fsync directory %s: %w", path, err)
	}
	return nil
}

func openInParent(absPath string, flags int, perm fs.FileMode) (*os.File, error) {
	cleanPath := filepath.Clean(absPath)
	base := filepath.Base(cleanPath)
	if base == "" || base == "." || base == ".." || base != filepath.Clean(base) {
		return nil, fmt.Errorf("invalid filename %q", base)
	}
	parent := filepath.Dir(cleanPath)

	dirfd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open directory %s: %w", parent, err)
	}
	defer func() {
		_ = unix.Close(dirfd)
	}()

	mode := uint32(perm.Perm())
	fd, err := unix.Openat(dirfd, base, flags, mode)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", cleanPath, err)
	}

	if fd < 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("invalid file descriptor: %d", fd)
	}
	if uintptr(fd) > uintptr(math.MaxInt) {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("file descriptor overflow: %d", fd)
	}

	file := os.NewFile(uintptr(fd), cleanPath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("failed to create file handle for %s", cleanPath)
	}
	return file, nil
}
