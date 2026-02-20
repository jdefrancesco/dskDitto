//go:build unix

package dmap

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func fdUintptrToInt(fd uintptr) (int, error) {
	if fd > uintptr(math.MaxInt) {
		return 0, fmt.Errorf("file descriptor overflow: %d", fd)
	}
	return int(fd), nil
}

func newFileFromFD(fd int, name string) (*os.File, error) {
	if fd < 0 {
		return nil, fmt.Errorf("invalid file descriptor: %d", fd)
	}
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		return nil, fmt.Errorf("failed to create file from descriptor: %d", fd)
	}
	return f, nil
}

func openFileSecure(absPath, dirPath, fileName string) (*os.File, error) {
	cleaned := filepath.Clean(fileName)
	if cleaned == "" || cleaned == "." || cleaned == ".." || cleaned != fileName {
		return nil, fmt.Errorf("invalid output filename %q", fileName)
	}

	// #nosec G304 -- dirPath and fileName are validated and originate from user-supplied path after cleaning
	dirHandle, err := os.Open(dirPath)
	if err != nil {
		return nil, fmt.Errorf("open directory %s: %w", dirPath, err)
	}
	defer dirHandle.Close()

	dirfd, err := fdUintptrToInt(dirHandle.Fd())
	if err != nil {
		return nil, fmt.Errorf("directory file descriptor: %w", err)
	}

	fd, err := unix.Openat(dirfd, cleaned, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open output file %s: %w", absPath, err)
	}

	f, err := newFileFromFD(fd, absPath)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return f, nil
}
