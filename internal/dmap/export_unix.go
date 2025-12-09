//go:build unix

package dmap

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

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

	fd, err := unix.Openat(int(dirHandle.Fd()), cleaned, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open output file %s: %w", absPath, err)
	}

	return os.NewFile(uintptr(fd), absPath), nil
}
