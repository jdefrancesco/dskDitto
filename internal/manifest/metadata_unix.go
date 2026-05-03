//go:build unix

package manifest

import (
	"os"
	"syscall"
)

func fileIdentity(info os.FileInfo) (uint64, uint64) {
	if info == nil {
		return 0, 0
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	if stat.Dev < 0 {
		return 0, stat.Ino
	}
	return uint64(stat.Dev), stat.Ino
}
