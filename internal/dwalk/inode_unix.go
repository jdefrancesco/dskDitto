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

type fileMeta struct {
	size        int64
	mode        os.FileMode
	identity    fileIdentity
	hasIdentity bool
}

func statFile(path string) (fileMeta, error) {
	var stat syscall.Stat_t
	if err := syscall.Lstat(path, &stat); err != nil {
		return fileMeta{}, err
	}
	return fileMeta{
		size: stat.Size,
		mode: modeFromStat(uint32(stat.Mode)),
		identity: fileIdentity{
			dev: uint64(stat.Dev), // #nosec G115 -- platform-defined but safely representable in uint64
			ino: uint64(stat.Ino), // #nosec G115 -- platform-defined but safely representable in uint64
		},
		hasIdentity: true,
	}, nil
}

func modeFromStat(mode uint32) os.FileMode {
	fileMode := os.FileMode(mode & 0o777)
	switch mode & syscall.S_IFMT {
	case syscall.S_IFDIR:
		fileMode |= os.ModeDir
	case syscall.S_IFLNK:
		fileMode |= os.ModeSymlink
	case syscall.S_IFBLK:
		fileMode |= os.ModeDevice
	case syscall.S_IFCHR:
		fileMode |= os.ModeDevice | os.ModeCharDevice
	case syscall.S_IFIFO:
		fileMode |= os.ModeNamedPipe
	case syscall.S_IFSOCK:
		fileMode |= os.ModeSocket
	}
	return fileMode
}
