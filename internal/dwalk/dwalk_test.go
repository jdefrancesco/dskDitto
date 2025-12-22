package dwalk

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
)

// Test basic walking...
func TestNewDWalk(t *testing.T) {

	// Initialize logger for testing
	dsklog.InitializeDlogger("test.log")

	rootDirs := []string{"test_files"}

	dFiles := make(chan *dfs.Dfile)
	walker := NewDWalker(rootDirs, dFiles, config.Config{SkipHidden: true, SkipVirtualFS: true, HashAlgorithm: dfs.HashSHA256, MaxDepth: -1})

	// walker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	walker := NewDWalker([]string{dir}, dFiles, config.Config{SkipHidden: skipHidden, SkipVirtualFS: true, HashAlgorithm: dfs.HashSHA256, MaxDepth: -1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	walker.Run(ctx)

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

func TestSkipVirtualFSToggle(t *testing.T) {
	dsklog.InitializeDlogger("/dev/null")

	dFiles := make(chan *dfs.Dfile)

	walkerSkip := NewDWalker([]string{"/"}, dFiles, config.Config{SkipVirtualFS: true, HashAlgorithm: dfs.HashSHA256, MaxDepth: -1})
	if !walkerSkip.shouldSkipPath("/proc") {
		t.Fatalf("expected /proc to be skipped when SkipVirtualFS is true")
	}

	walkerInclude := NewDWalker([]string{"/"}, dFiles, config.Config{SkipVirtualFS: false, HashAlgorithm: dfs.HashSHA256, MaxDepth: -1})
	if walkerInclude.shouldSkipPath("/proc") {
		t.Fatalf("expected /proc not to be skipped when SkipVirtualFS is false")
	}
}

func TestMaxDepthLimit(t *testing.T) {
	dsklog.InitializeDlogger("/dev/null")

	root := t.TempDir()
	level1 := filepath.Join(root, "level1")
	level2 := filepath.Join(level1, "level2")

	if err := os.MkdirAll(level2, 0o755); err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	files := []struct {
		path string
		data string
	}{
		{filepath.Join(root, "root.txt"), "root"},
		{filepath.Join(level1, "one.txt"), "level1"},
		{filepath.Join(level2, "two.txt"), "level2"},
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.data), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f.path, err)
		}
	}

	collect := func(depth int) []string {
		cfg := config.Config{
			HashAlgorithm: dfs.HashSHA256,
			SkipVirtualFS: true,
			MaxDepth:      depth,
		}
		return collectRelativePaths(t, root, cfg)
	}

	all := collect(-1)
	expectPathsEqual(t, all, []string{"level1/level2/two.txt", "level1/one.txt", "root.txt"})

	depth0 := collect(0)
	expectPathsEqual(t, depth0, []string{"root.txt"})

	depth1 := collect(1)
	expectPathsEqual(t, depth1, []string{"level1/one.txt", "root.txt"})
}

func TestMaxFileSizeLimit(t *testing.T) {
	dsklog.InitializeDlogger("/dev/null")

	root := t.TempDir()
	small := filepath.Join(root, "small.dat")
	large := filepath.Join(root, "large.dat")

	if err := os.WriteFile(small, bytes.Repeat([]byte("a"), 1024), 0o644); err != nil {
		t.Fatalf("failed to create small file: %v", err)
	}
	if err := os.WriteFile(large, bytes.Repeat([]byte("b"), 5*1024*1024), 0o644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	cfg := config.Config{
		HashAlgorithm: dfs.HashSHA256,
		SkipVirtualFS: true,
		MaxFileSize:   2048, // 2 KiB
	}

	paths := collectRelativePaths(t, root, cfg)
	expectPathsEqual(t, paths, []string{"small.dat"})
}

func TestExcludePaths(t *testing.T) {
	dsklog.InitializeDlogger("/dev/null")

	root := t.TempDir()
	keepDir := filepath.Join(root, "keep")
	skipDir := filepath.Join(root, "skip")

	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("failed to create keep dir: %v", err)
	}
	if err := os.MkdirAll(skipDir, 0o755); err != nil {
		t.Fatalf("failed to create skip dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(keepDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("failed to write keep file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skipDir, "skip.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("failed to write skip file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "root.txt"), []byte("root"), 0o644); err != nil {
		t.Fatalf("failed to write root file: %v", err)
	}

	cfg := config.Config{
		HashAlgorithm: dfs.HashSHA256,
		SkipVirtualFS: true,
		MaxDepth:      -1,
		ExcludePaths:  []string{skipDir},
	}

	paths := collectRelativePaths(t, root, cfg)
	expectPathsEqual(t, paths, []string{"keep/keep.txt", "root.txt"})
}

func collectRelativePaths(t *testing.T, root string, cfg config.Config) []string {
	t.Helper()

	dFiles := make(chan *dfs.Dfile, 16)
	walker := NewDWalker([]string{root}, dFiles, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	walker.Run(ctx)

	var names []string
	for df := range dFiles {
		rel, err := filepath.Rel(root, df.FileName())
		if err != nil {
			t.Fatalf("failed to compute relative path: %v", err)
		}
		names = append(names, filepath.ToSlash(rel))
	}

	sort.Strings(names)
	return names
}

func expectPathsEqual(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected path count: got %d want %d (values=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected paths: got %v want %v", got, want)
		}
	}
}
