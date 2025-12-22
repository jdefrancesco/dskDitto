//go:build linux

package dfs

import (
	"fmt"
	"syscall"
)

func detectFilesystem(path string) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", err
	}

	magic := stat.Type

	known := map[int64]string{
		0xEF53:     "ext2/ext3/ext4",
		0x9123683E: "btrfs",
		0x58465342: "xfs",
		0x5346544e: "ntfs",
		0x01021994: "tmpfs",
		0x73717368: "squashfs",
		0x2fc12fc1: "zfs",
		0x62656572: "f2fs",
		0x6462671f: "debugfs",
		0x858458f6: "ramfs",
	}

	if fs, ok := known[magic]; ok {
		return fs, nil
	}

	return fmt.Sprintf("unknown (magic=0x%x)", magic), nil
}
