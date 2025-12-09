//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package dfs

import (
	"syscall"
)

// extract name from fixed-size C array
func bsdNameToString(arr []int8) string {
	buf := make([]byte, 0, len(arr))
	for _, c := range arr {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}

func detectFilesystem(path string) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", err
	}

	return bsdNameToString(stat.Fstypename[:]), nil
}
