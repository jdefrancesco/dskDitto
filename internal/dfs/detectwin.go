//go:build windows

package dfs

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func detectFilesystem(path string) (string, error) {
	full, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	p, err := windows.UTF16PtrFromString(full)
	if err != nil {
		return "", err
	}

	volName := make([]uint16, windows.MAX_PATH)
	fsName := make([]uint16, windows.MAX_PATH)
	var serial, maxCompLen, flags uint32

	err = windows.GetVolumeInformation(
		p,
		&volName[0],
		uint32(len(volName)),
		&serial,
		&maxCompLen,
		&flags,
		&fsName[0],
		uint32(len(fsName)),
	)
	if err != nil {
		return "", err
	}

	return windows.UTF16ToString(fsName), nil
}
