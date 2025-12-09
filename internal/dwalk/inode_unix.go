//go:build unix

package dwalk

import (
	"os"
	"syscall"
)

// fileIdentity uniquely identifies a file by device and inode on Unix systems.
type fileIdentity struct {
	dev uint64
	ino uint64
}

// getFileIdentity extracts a stable identity for a regular file based on its
// underlying Stat_t. If the platform or info does not expose this, the
// second return value is false.
func getFileIdentity(info os.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return fileIdentity{}, false
	}
	return fileIdentity{
		dev: uint64(stat.Dev), // #nosec G115 -- platform-defined but safely representable in uint64
		ino: uint64(stat.Ino), // #nosec G115 -- platform-defined but safely representable in uint64
	}, true
}
