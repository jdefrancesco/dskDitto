// Utility functions and types foir querying information about
// File System we wish to run unditto on.
package dfs

import (
	"fmt"
	"os"
	"path/filepath"
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

// GetDFileState will return Stat information of file.
// This includes UID/Filesize/etc.
func GetDFileStat(info os.FileInfo) (*DFileStat, error) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		statDev := stat.Dev
		if stat.Dev < 0 || stat.Rdev < 0 {
			return nil, fmt.Errorf("device values are negative: dev=%d rdev=%d", stat.Dev, stat.Rdev)
		}
		return &DFileStat{
			Dev:     uint64(statDev),
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

// GetFileUidGid calls Stat on a function and grabs the Uid and Gid
// of a file.
func GetFileUidGid(filename string) (Uid, Gid int) {

	info, err := os.Stat(filename)
	if err != nil {
		err := fmt.Errorf("error os.Stat %v", err)
		fmt.Println(err)
		return -1, -1
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		fmt.Println("error calling Stat")
		return -1, -1
	}

	Uid = int(stat.Uid)
	Gid = int(stat.Gid)
	return Uid, Gid
}

// Check if we have proper permissions for investigating
// a file. Performs various safety checks to prevent any
// file path vulnerabilities.
// TODO: Add more safety checks later on..
func CheckFilePerms(path string) bool {
	cleanPath := filepath.Clean(path)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	// #nosec G304
	file, err := os.Open(absPath)
	if err != nil {
		if os.IsPermission(err) {
			return false
		}
		// fmt.Printf("Error opening file: %s, error: %v\n", path, err)
		return false
	}
	defer file.Close()
	return true
}
