// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"

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
	dFiles       chan<- *dfs.Dfile
	sem          *semaphore.Weighted
	skipHidden   bool
	skipEmpty    bool
	skipSymLinks bool
	minFileSize  uint
	maxFileSize  uint
	hashAlgo     dfs.HashAlgorithm
}

// NewDWalker returns a new DWalk instance that accepts traversal options.
// TODO: Fix blake3 support. Currently only SHA256 is properly implemented.
func NewDWalker(rootDirs []string, dFiles chan<- *dfs.Dfile, cfg config.Config) *DWalk {

	hashAlgo := cfg.HashAlgorithm
	if hashAlgo == "" {
		hashAlgo = dfs.HashSHA256
	}

	maxSize := cfg.MaxFileSize
	if maxSize == 0 {
		maxSize = MAX_FILE_SIZE
	}

	walker := &DWalk{
		rootDirs:     rootDirs,
		dFiles:       dFiles,
		skipHidden:   cfg.SkipHidden,
		skipEmpty:    cfg.SkipEmpty,
		skipSymLinks: cfg.SkipSymLinks,
		minFileSize:  cfg.MinFileSize,
		maxFileSize:  maxSize,
		hashAlgo:     dfs.HashSHA256, // Only use SHA256 for now. Need to debug performance issues with Blake3
	}

	// Set semaphore to optimal value based on system resources
	optimalConcurrency := getOptimalConcurrency()
	dsklog.Dlogger.Infof("Setting directory concurrency to %d (based on %d CPUs)", optimalConcurrency, runtime.NumCPU())
	walker.sem = semaphore.NewWeighted(int64(optimalConcurrency))
	return walker

}

// Run method kicks off filesystem crawl for file dupes.
func (d *DWalk) Run(ctx context.Context) {

	for _, root := range d.rootDirs {
		d.wg.Add(1)
		go walkDir(ctx, root, d, d.dFiles)
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
func walkDir(ctx context.Context, dir string, d *DWalk, dFiles chan<- *dfs.Dfile) {

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

		if d.skipSymLinks && entry.Type()&os.ModeSymlink != 0 {
			dsklog.Dlogger.Debugf("Skipping symlink: %s", filepath.Join(dir, name))
			continue
		}

		if entry.IsDir() {
			d.wg.Add(1)
			subDir := filepath.Join(dir, name)
			go walkDir(ctx, subDir, d, d.dFiles)
			continue
		}

		info, err := entry.Info()
		if err != nil {
			dsklog.Dlogger.Debugf("Error getting file info for %s: %v", name, err)
			continue
		}

		if d.skipSymLinks && info.Mode()&os.ModeSymlink != 0 {
			dsklog.Dlogger.Debugf("Skipping symlink (resolved): %s", filepath.Join(dir, name))
			continue
		}

		// Skip non-regular files (sockets, pipes, device files, etc.)
		if !info.Mode().IsRegular() {
			dsklog.Dlogger.Debugf("Skipping non-regular file: %s (mode: %s)", entry.Name(), info.Mode())
			continue
		}

		// Check file size properties the user set.
		fileSize := uint(max(info.Size(), 0)) // #nosec G115
		if d.skipEmpty && fileSize == 0 {
			dsklog.Dlogger.Debugf("Skipping empty file: %s", name)
			continue
		}
		if d.minFileSize > 0 && fileSize < d.minFileSize {
			dsklog.Dlogger.Debugf("File %s smaller than minimum. Skipping", name)
			continue
		}
		if d.maxFileSize > 0 && fileSize >= d.maxFileSize {
			dsklog.Dlogger.Infof("File %s larger than maximum. Skipping", name)
			continue
		}

		absFileName := filepath.Join(dir, name)
		if !dfs.CheckFilePerms(absFileName) {
			dsklog.Dlogger.Debugf("Cannot access file. Invalid permissions: %s", absFileName)
			continue
		}

		dFileEntry, err := dfs.NewDfile(absFileName, info.Size(), d.hashAlgo)
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
