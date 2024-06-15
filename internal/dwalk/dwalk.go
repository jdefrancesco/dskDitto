// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
	"ditto/dfs"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/semaphore"
)

const (
	MAX_FILE_SIZE = 1024 * 1024 * 1024 * 1 // 1GB
)

func init() {
	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n\n")
		flag.PrintDefaults()
	}
}

// DWalk is our primary object for traversing filesystem
// in a parallel manner.
type DWalk struct {
	rootDirs    []string
	wg          *sync.WaitGroup
	maxFileSize uint64 // Skip over files that are greater than maxFileSize

	// Channel used to communicate with main monitor goroutine.
	dFiles chan<- *dfs.Dfile
	sem    *semaphore.Weighted
}

// NewDWalker returns a new DWalk instance that accepts a root directory, wait group, and
// channel dFiles over which we pass file names along with their hash for monitor loop.
func NewDWalker(rootDirs []string, dFiles chan<- *dfs.Dfile) *DWalk {

	walker := &DWalk{
		rootDirs: rootDirs,
		dFiles:   dFiles,
	}
	walker.wg = new(sync.WaitGroup)
	walker.sem = semaphore.NewWeighted(int64(20))
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
		if entry.IsDir() {
			d.wg.Add(1)
			subDir := filepath.Join(dir, entry.Name())
			go walkDir(ctx, subDir, d, dFiles)
		} else {

			// Deal with large or irregular files..
			// MAX_FILE_SIZE is currently a gig for now. Need to make this configurable.
			if entry.Size() >= MAX_FILE_SIZE {
				// TODO: Add to a list of files that were skipped.
				continue
			}

			// Handle special files.
			if !entry.Mode().IsRegular() {
				continue
			}

			absFileName := filepath.Join(dir, entry.Name())
			dFileEntry, err := dfs.NewDfile(absFileName, entry.Size())
			if err != nil {
				continue
			}

			// send our dFile to our monitor routine.
			dFiles <- dFileEntry
		}
	}

}

// dirEntries returns contents of a directory specified by dir.
// The semaphore limits concurrency; preventing system resource
// exhaustion.
func dirEntries(ctx context.Context, dir string, d *DWalk) []os.FileInfo {

	// Handle cancellation and semaphore acquisition.
	select {
	case <-ctx.Done():
		// cancel asserted.
		return nil
	default:
		d.sem.Acquire(ctx, 1)
	}
	defer func() {
		d.sem.Release(1)
	}()

	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\rerror: %v\n", err)
		return nil
	}

	return entries
}
