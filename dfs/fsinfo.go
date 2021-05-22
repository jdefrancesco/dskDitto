// Utility functions and types foir querying information about
// File System we wish to run unditto on.
package dfs

import (
	"fmt"
	"os"
	"syscall"

	sigar "github.com/cloudfoundry/gosigar"
)

const OutputFormat = "%-15s %4s %4s %5s %4s %-15s\n"

// DFileStat holds our filesystem specific Stat
// information. We are currently only concerned
// with Darwin.
type DFileStat struct {
	Dev     uint64
	Inode   uint64
	Nlink   uint64
	Mode    uint32
	Uid     uint32
	Gid     uint32
	Rdev    uint64
	Size    int64
	Blksize int64
	Blocks  int64
}

// GetDFileState will return Stat information of file
func GetDFileStat(info os.FileInfo) (*DFileStat, error) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return &DFileStat{
			Dev:     uint64(stat.Dev),
			Inode:   stat.Ino,
			Nlink:   uint64(stat.Nlink),
			Mode:    uint32(stat.Mode),
			Uid:     stat.Uid,
			Gid:     stat.Gid,
			Rdev:    uint64(stat.Rdev),
			Size:    stat.Size,
			Blksize: int64(stat.Blksize),
			Blocks:  stat.Blocks,
		}, nil
	}

	return nil, fmt.Errorf("unable to get file stat for %#v", info)

}

// Store F
type DittoFsInfo struct {
}

// DittoFsInfo constructor.
// func NewDittoFsInfo() (*DittoFsInfo, error) {

// 	return
// }

// FsInfo will print out filesystem stats.
func (fs *DittoFsInfo) PrintFsInfo(path string) {

}

// List filesystems on machine
func ListFileSystems() bool {

	fsList := sigar.FileSystemList{}
	fsList.Get()

	fmt.Fprintf(os.Stdout, OutputFormat,
		"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted On")

	for _, fs := range fsList.List {
		dirName := fs.DirName
		usage := sigar.FileSystemUsage{}
		usage.Get(dirName)

		fmt.Fprintf(os.Stdout, OutputFormat,
			fs.DevName,
			formatSize(usage.Total),
			formatSize(usage.Used),
			formatSize(usage.Avail),
			sigar.FormatPercent(usage.UsePercent()),
			dirName)

	}

	// For our Example test harness
	return true
}

// formatSize will make out sizes more human friendly
func formatSize(size uint64) string {
	return sigar.FormatSize(size * 1024)
}
