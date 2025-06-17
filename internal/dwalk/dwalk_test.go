package dwalk

import (
	"context"
	"ditto/internal/dfs"
	"fmt"
	"testing"
	"time"
)

// Test basic walking...
func TestNewDWalk(t *testing.T) {

	rootDirs := []string{"test_files"}

	dFiles := make(chan *dfs.Dfile)
	walker := NewDWalker(rootDirs, dFiles)

	// walker
	ctx, _ := context.WithCancel(context.Background())
	var MaxFileSize uint64 = 1024 * 1024 * 1024 * 1
	walker.Run(ctx, MaxFileSize)

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
			fmt.Println("Tick...")

		}
	}

	if nfiles != 5 {
		t.Errorf("want 12 files. got %d\n", nfiles)
	}
	fmt.Printf("%d files\n", nfiles)

}
