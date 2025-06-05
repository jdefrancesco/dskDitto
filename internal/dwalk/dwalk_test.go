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

// Write function to walk directory and return list of files.
// func TestWalkDir(t *testing.T) {

// 	root := "."
// 	os.Chdir("test_files")
// 	fileSystem := os.DirFS(root)
// 	fs.WalkDir(fileSystem, root, func(path string, d fs.DirEntry, err error) error {
// 		if err != nil {
// 			fmt.Println(err)
// 			return nil
// 		}
// 		if !d.IsDir() {
// 			fmt.Println(path)
// 		}
// 		return nil
// 	})

// 	return
// }
