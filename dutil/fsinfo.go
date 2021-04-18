// Utility functions and types foir querying information about
// File System we wish to run unditto on.
package dutil

import (
	"fmt"
	"os"

	sigar "github.com/cloudfoundry/gosigar"
)

const OutputFormat = "%-15s %4s %4s %5s %4s %-15s\n"

// type DittoFsInfo struct {
// }

// // DittoFsInfo constructor.
// func NewDittoFsInfo() (*DittoFsInfo, error) {

// }

// // FsInfo will print out filesystem stats.
// func (fs *DittoFsInfo) PrintFsInfo(path string) {

// }

// List all filesystems on machine
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
