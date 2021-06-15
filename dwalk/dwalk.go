// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
	"ditto/dfs"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/semaphore"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

// DWalk is our primary object for traversing filesystem
// in a parallel manner.
type DWalk struct {
	rootDirs []string
	wg       *sync.WaitGroup

	// Channel used to communicate with main monitor goroutine.
	dFiles chan<- *dfs.Dfile
	// Semaphore limits concurrency, preventing resource exhaustion.
	sem *semaphore.Weighted
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
			// Handle special files.
			if !entry.Mode().IsRegular() {
				// For now we will skip over special files...
				log.Info().Msgf("Skipping file %s", entry.Name())
				continue
			}

			absFileName := filepath.Join(dir, entry.Name())
			// Create new Dfile for file entry.
			dFileEntry, err := dfs.NewDfile(absFileName, entry.Size())
			if err != nil {
				log.Error().Msgf("Error creating dFile: %s\n", err)
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
		fmt.Println(err)
		return nil
	}

	return entries
}
