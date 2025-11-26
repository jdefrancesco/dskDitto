// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"ditto/internal/dfs"
	"ditto/internal/dsklog"

	"golang.org/x/sync/semaphore"
)

const (
	MAX_FILE_SIZE           uint = 1024 * 1024 * 1024 * 4 // 4GB
	DEFAULT_DIR_CONCURRENCY      = 50                     // Optimal balance for directory reading
)

// DWalk is our primary object for traversing filesystem
// in a parallel manner.
type DWalk struct {
	rootDirs []string
	wg       sync.WaitGroup

	// Channel used to communicate with main monitor goroutine.
	dFiles        chan<- *dfs.Dfile
	sem           *semaphore.Weighted
	skipHidden    bool
	hashAlgorithm dfs.HashAlgorithm
}

// NewDWalker returns a new DWalk instance that accepts a root directory, wait group, and
// channel dFiles over which we pass file names along with their hash for monitor loop.
func NewDWalker(rootDirs []string, dFiles chan<- *dfs.Dfile, algo dfs.HashAlgorithm, skipHidden bool) *DWalk {

	if algo == "" {
		algo = dfs.HashSHA256
	}

	walker := &DWalk{
		rootDirs:      rootDirs,
		dFiles:        dFiles,
		skipHidden:    skipHidden,
		hashAlgorithm: algo,
	}

	// Set semaphore to optimal value based on system resources
	optimalConcurrency := getOptimalConcurrency()
	dsklog.Dlogger.Infof("Setting directory concurrency to %d (based on %d CPUs)", optimalConcurrency, runtime.NumCPU())
	walker.sem = semaphore.NewWeighted(int64(optimalConcurrency))
	return walker

}

// Run method kicks off filesystem crawl for file dupes.
func (d *DWalk) Run(ctx context.Context, minFileSize, maxFileSize uint) {

	for _, root := range d.rootDirs {
		d.wg.Add(1)
		go walkDir(ctx, root, d, d.dFiles, minFileSize, maxFileSize)
	}

	// Wait for all goroutines to finish.
	go func() {
		d.wg.Wait()
		// close channel.
		close(d.dFiles)
	}()

}

// cancelled polls, checking for cancellation.
func cancelled(ctx context.Context) bool {

	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}

}

// walkDir recursively walk directories and send files to our monitor go routine
// (in main.go) to be added to the duplication map.
func walkDir(ctx context.Context, dir string, d *DWalk, dFiles chan<- *dfs.Dfile, minFileSize, maxFileSize uint) {

	defer d.wg.Done()

	// Check for cancellation.
	if cancelled(ctx) {
		return
	}

	for _, entry := range dirEntries(ctx, dir, d) {
		// Handle processing of dotfiles (hidden)
		name := entry.Name()
		if d.skipHidden && strings.HasPrefix(name, ".") {
			dsklog.Dlogger.Debugf("Skipping hidden entry: %s", filepath.Join(dir, name))
			continue
		}

		if entry.IsDir() {
			d.wg.Add(1)
			subDir := filepath.Join(dir, name)
			go walkDir(ctx, subDir, d, d.dFiles, minFileSize, maxFileSize)
			continue
		}

		info, err := entry.Info()
		if err != nil {
			dsklog.Dlogger.Debugf("Error getting file info for %s: %v", name, err)
			continue
		}

		// Skip non-regular files (sockets, pipes, device files, etc.)
		if !info.Mode().IsRegular() {
			dsklog.Dlogger.Debugf("Skipping non-regular file: %s (mode: %s)", entry.Name(), info.Mode())
			continue
		}

		// Check file size properties the user set.
		fileSize := uint(max(info.Size(), 0)) // #nosec G115
		if minFileSize > 0 && fileSize < minFileSize {
			dsklog.Dlogger.Debugf("File %s smaller than minimum. Skipping", name)
			continue
		}
		if maxFileSize > 0 && fileSize >= maxFileSize {
			dsklog.Dlogger.Infof("File %s larger than maximum. Skipping", name)
			continue
		}

		absFileName := filepath.Join(dir, name)
		if !dfs.CheckFilePerms(absFileName) {
			dsklog.Dlogger.Debugf("Cannot access file. Invalid permissions: %s", absFileName)
			continue
		}

		dFileEntry, err := dfs.NewDfile(absFileName, info.Size(), d.hashAlgorithm)
		if err == nil {
			dFiles <- dFileEntry
		}
	}
}

// dirEntries returns contents of a directory specified by dir.
// The semaphore limits concurrency; preventing system resource
// exhaustion.
func dirEntries(ctx context.Context, dir string, d *DWalk) []os.DirEntry {

	if cancelled(ctx) {
		return nil
	}

	// Semaphore helps control concurrency.
	if err := d.sem.Acquire(ctx, 1); err != nil {
		return nil
	}
	defer d.sem.Release(1)

	entries, err := os.ReadDir(dir)
	if err != nil {
		dsklog.Dlogger.Errorf("Directory read error: %v", err)
		return nil
	}

	return entries
}

// getOptimalConcurrency returns optimal concurrency based on system resources
func getOptimalConcurrency() int {
	procs := runtime.GOMAXPROCS(0)
	if procs < 1 {
		procs = runtime.NumCPU()
	}
	concurrency := min(procs*4, 128)

	dsklog.Dlogger.Debugf("Directory walker concurrency: %d (procs=%d)", concurrency, procs)
	return concurrency
}
