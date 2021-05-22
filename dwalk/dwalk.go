// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
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
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

// DWalk is our primary object for traversing filesystem
// in a parallel manner.
type DWalk struct {
	rootDirs []string
	wg       *sync.WaitGroup

	// This channel will be created by monitor
	// loop to collect Dfile data needed to build
	// duplication map.
	// dFiles chan<- dfs.Dfile
	dFiles chan<- int64
	sem    *semaphore.Weighted
}

// NewDWalker returns a new DWalk instance that accepts a root directory, wait group, and
// channel dFiles over which we pass file names along with their hash for monitor loop.
func NewDWalker(rootDirs []string, dFiles chan<- int64) *DWalk {
	walker := &DWalk{
		rootDirs: rootDirs,
		dFiles:   dFiles,
	}
	walker.wg = new(sync.WaitGroup)
	walker.sem = semaphore.NewWeighted(int64(20))
	return walker
}

// Run kicks off filesystem walking. Run takes a context as
// its sole argument to handle cancellations.
func (d *DWalk) Run(ctx context.Context) {

	for _, root := range d.rootDirs {
		d.wg.Add(1)
		go walkDir(ctx, root, d, d.dFiles)
	}

	go func() {
		log.Info().Msgf("launched walkDir: waiting for jobs to finish")
		d.wg.Wait()
		close(d.dFiles)
	}()
}

// cancelled will poll to check is there in a cancellation.
func cancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// walkDir
func walkDir(ctx context.Context, dir string, d *DWalk, dFiles chan<- int64) {
	defer d.wg.Done()
	if cancelled(ctx) {
		return
	}

	for _, entry := range dirEntries(ctx, dir, d) {
		if entry.IsDir() {
			d.wg.Add(1)
			subDir := filepath.Join(dir, entry.Name())
			go walkDir(ctx, subDir, d, dFiles)
		} else {
			// send channel
			dFiles <- entry.Size()
		}
	}
}

// dirEntries returns contents of a directory specified by dir.
func dirEntries(ctx context.Context, dir string, d *DWalk) []os.FileInfo {
	// Handle cancellation.
	select {
	case <-ctx.Done():
		return nil // cancelled
	default:
		d.sem.Acquire(ctx, 1)
	}
	defer func() {
		d.sem.Release(1)
	}()

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Println("error readdir")
		return nil
	}
	return files
}
