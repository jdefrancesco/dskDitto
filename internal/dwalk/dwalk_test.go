package dwalk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ditto/internal/dfs"
	"ditto/internal/dsklog"
)

// Test basic walking...
func TestNewDWalk(t *testing.T) {

	// Initialize logger for testing
	dsklog.InitializeDlogger("test.log")

	rootDirs := []string{"test_files"}

	dFiles := make(chan *dfs.Dfile)
	walker := NewDWalker(rootDirs, dFiles, dfs.HashSHA256, true)

	// walker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var MaxFileSize uint = 1024 * 1024 * 1024 * 1
	walker.Run(ctx, 0, MaxFileSize)

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

	fmt.Printf("%d files\n", nfiles)

}

func TestSkipHiddenFiles(t *testing.T) {
	// Ensure logger is initialized once for tests.
	dsklog.InitializeDlogger("/dev/null")

	t.Run("SkipHidden", func(t *testing.T) {
		names := collectFiles(t, true)
		if contains(names, ".hidden.txt") {
			t.Fatalf("expected hidden file to be skipped, names=%v", names)
		}
		if !contains(names, "visible.txt") {
			t.Fatalf("expected visible file to be processed, names=%v", names)
		}
	})

	t.Run("IncludeHidden", func(t *testing.T) {
		names := collectFiles(t, false)
		if !contains(names, ".hidden.txt") {
			t.Fatalf("expected hidden file to be included when skipHidden=false, names=%v", names)
		}
		if !contains(names, "visible.txt") {
			t.Fatalf("expected visible file to be processed, names=%v", names)
		}
	})
}

func collectFiles(t *testing.T, skipHidden bool) []string {
	t.Helper()

	dir := t.TempDir()

	visible := filepath.Join(dir, "visible.txt")
	hidden := filepath.Join(dir, ".hidden.txt")

	if err := os.WriteFile(visible, []byte("visible"), 0o644); err != nil {
		t.Fatalf("failed to write visible file: %v", err)
	}
	if err := os.WriteFile(hidden, []byte("hidden"), 0o644); err != nil {
		t.Fatalf("failed to write hidden file: %v", err)
	}

	dFiles := make(chan *dfs.Dfile, 4)
	walker := NewDWalker([]string{dir}, dFiles, dfs.HashSHA256, skipHidden)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const maxFileSize uint = MAX_FILE_SIZE
	walker.Run(ctx, 0, maxFileSize)

	var names []string
	for df := range dFiles {
		names = append(names, filepath.Base(df.FileName()))
	}
	return names
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
