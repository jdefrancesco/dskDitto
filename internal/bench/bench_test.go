// This package contains benchmark related logic/tests.
package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"
	"ditto/internal/dwalk"
)

// setupBenchmark initializes the logger and other necessary components
func setupBenchmark() {
	// Initialize the logger to prevent nil pointer dereference
	dsklog.InitializeDlogger("/dev/null") // Use /dev/null to avoid creating log files during benchmarks
}

// BenchmarkNewDfile benchmarks overhead of creating a new Dfile. A Dfile
// is the abstraction we use for files we crawl and analyze.
func BenchmarkNewDfile(b *testing.B) {

	tmpFile, err := os.CreateTemp("", "benchfile")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write up to 1MiB of data
	data := make([]byte, 1024*10124)
	tmpFile.Write(data)
	tmpFile.Close()

	info, _ := os.Stat(tmpFile.Name())

	b.ResetTimer()
	for b.Loop() {
		_, err := dfs.NewDfile(tmpFile.Name(), info.Size())
		if err != nil {
			b.Fatalf("NewDfile failed: %v:", err)
		}
	}
}

// BenchmarkDWalkRun benchmarks the DWalk.Run method for directory traversal
func BenchmarkDWalkRun(b *testing.B) {
	setupBenchmark()

	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "dwalk_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some test files and subdirectories
	for i := range 100 {
		subDir := filepath.Join(tmpDir, "subdir", "level1", "level2")
		os.MkdirAll(subDir, 0755)

		// Create files in different directories
		testFile := filepath.Join(subDir, fmt.Sprintf("testfile%d.txt", i))
		if err := os.WriteFile(testFile, []byte("test data for benchmarking"), 0644); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		// Create a channel to collect files
		dFiles := make(chan *dfs.Dfile, 1000)

		// Create DWalk instance
		walker := dwalk.NewDWalker([]string{tmpDir}, dFiles)

		// Run the walker with context
		ctx := context.Background()
		walker.Run(ctx, 1024*1024*10) // 10MB max file size

		// Drain the channel
		go func() {
			for range dFiles {
				// Consume files
			}
		}()
	}
}

// BenchmarkMonitorLoop benchmarks the monitor loop that processes files and builds the duplicate map
func BenchmarkMonitorLoop(b *testing.B) {
	setupBenchmark()

	// Create a temporary directory with some duplicate files
	tmpDir, err := os.MkdirTemp("", "monitor_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test data for duplicates
	testData := []byte("This is test data that will be duplicated")

	// Create several duplicate files
	for i := range 50 {
		file1 := filepath.Join(tmpDir, fmt.Sprintf("duplicate1_%d.txt", i))
		file2 := filepath.Join(tmpDir, fmt.Sprintf("duplicate2_%d.txt", i))

		if err := os.WriteFile(file1, testData, 0644); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(file2, testData, 0644); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		// Create a new dmap for each iteration
		dMap, err := dmap.NewDmap()
		if err != nil {
			b.Fatal(err)
		}

		// Create channel for files
		dFiles := make(chan *dfs.Dfile, 1000)

		// Start the monitor loop in a goroutine
		done := make(chan bool)
		go func() {
			defer close(done)
			for dfile := range dFiles {
				dMap.Add(dfile)
			}
		}()

		// Simulate file processing by creating Dfiles and sending them
		walker := dwalk.NewDWalker([]string{tmpDir}, dFiles)
		ctx := context.Background()
		walker.Run(ctx, 1024*1024*10)

		// Wait for monitor loop to finish
		<-done
	}
}

// BenchmarkDmapOperations benchmarks core Dmap operations
func BenchmarkDmapOperations(b *testing.B) {
	// Create test files
	tmpDir, err := os.MkdirTemp("", "dmap_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Pre-create some test files
	var testFiles []*dfs.Dfile
	for i := range 1000 {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("testfile%d.txt", i))
		data := []byte(fmt.Sprintf("test data %d", i))
		if err := os.WriteFile(testFile, data, 0644); err != nil {
			b.Fatal(err)
		}

		info, _ := os.Stat(testFile)
		dfile, err := dfs.NewDfile(testFile, info.Size())
		if err != nil {
			b.Fatal(err)
		}
		testFiles = append(testFiles, dfile)
	}

	b.ResetTimer()
	for b.Loop() {
		dMap, err := dmap.NewDmap()
		if err != nil {
			b.Fatal(err)
		}

		// Benchmark adding files to the map
		for _, dfile := range testFiles {
			dMap.Add(dfile)
		}

		// Benchmark map operations
		_ = dMap.MapSize()
		_ = dMap.GetMap()
	}
}
