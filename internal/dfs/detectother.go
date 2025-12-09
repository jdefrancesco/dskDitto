//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package dfs

import "fmt"

func detectFilesystem(path string) (string, error) {
	return "", fmt.Errorf("filesystem detection not supported on this OS")
}
