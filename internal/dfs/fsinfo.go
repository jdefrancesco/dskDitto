// Utility functions and types foir querying information about
// File System we wish to run unditto on.
package dfs

import (
	"ditto/internal/dsklog"
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
// a file. Performs additional fast safety checks to avoid
// symlink traversal and non-regular special files.
// TODO: These additional checks need to be refactored out. Not sure what I was
//
//	thinking when I put them there.
func CheckFilePerms(path string) bool {
	cleanPath := filepath.Clean(path)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	// Fast path checks without following symlinks.
	fi, err := os.Lstat(absPath)
	if err != nil {
		return false
	}
	// Disallow symlinks
	if fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	// Only allow regular files (exclude sockets, devices, pipes, etc.)
	if !fi.Mode().IsRegular() {
		return false
	}

	// #nosec G304
	// Use O_NOFOLLOW for additional safety (avoid TOCTOU on symlinks)
	fd, err := syscall.Open(absPath, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	_ = syscall.Close(fd)
	return true
}

// GetFileSize will return size of file in bytes.
// If file name isn't provided we return zero. Will
// refactor later for better error handling.
func GetFileSize(file_name string) uint64 {
	if len(file_name) == 0 {
		dsklog.Dlogger.Warn("Empty file name provided")
		return 0
	}

	file, err := os.Stat(file_name)
	if err != nil {
		dsklog.Dlogger.Warnf("Error calling os.Stat on %s: %v", file_name, err)
		return 0
	}

	size := file.Size()
	// This shouldn't happen.
	if size < 0 {
		return 0
	}
	return uint64(size)
}
