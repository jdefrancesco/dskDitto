package dwalk

import (
	"context"
	"ditto/dfs"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
)

// Test basic walking...
func TestNewDWalk(t *testing.T) {

	rootDirs := []string{"test_files"}

	dFiles := make(chan *dfs.Dfile)
	walker := NewDWalker(rootDirs, dFiles)

	// walker
	ctx, _ := context.WithCancel(context.Background())
	walker.Run(ctx)

	var nfiles int64
	tick := time.Tick(500 * time.Millisecond)

loop:
	for {

		select {
		case _, ok := <-dFiles:
			if !ok {
				break loop
			}
			// Test dir and subdirs should only have 14 files
			nfiles++

		case <-tick:
			log.Info().Msgf("Tick...")
		}
	}

	if nfiles != 14 {
		t.Errorf("want 15 files. got %d\n", nfiles)
	}
	fmt.Printf("%d files\n", nfiles)

}
