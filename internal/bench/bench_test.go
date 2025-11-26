// This package contains benchmark related logic/tests.
package bench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"ditto/internal/config"
	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"
	"ditto/internal/dwalk"
)

var benchmarkInit sync.Once

// setupBenchmark ensures the logger is initialised exactly once for benchmarks.
func setupBenchmark(tb testing.TB) {
	tb.Helper()
	benchmarkInit.Do(func() {
		dsklog.InitializeDlogger("/dev/null")
	})
}

// BenchmarkNewDfile benchmarks overhead of creating a new Dfile. A Dfile
// is the abstraction we use for files we crawl and analyze.
func BenchmarkNewDfile(b *testing.B) {
	setupBenchmark(b)

	dir := b.TempDir()
	path := makeSizedFile(b, dir, "benchfile.dat", 1<<20) // 1 MiB
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := dfs.NewDfile(path, info.Size()); err != nil {
			b.Fatalf("NewDfile failed: %v", err)
		}
	}
}

// BenchmarkHashFileSHA256 measures hashing throughput for a range of file sizes.
func BenchmarkHashFileSHA256(b *testing.B) {
	setupBenchmark(b)

	tests := []struct {
		name string
		size int
	}{
		{"4KiB", 4 * 1024},
		{"64KiB", 64 * 1024},
		{"1MiB", 1 << 20},
		{"8MiB", 8 << 20},
	}

	for _, tc := range tests {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			dir := b.TempDir()
			path := makeSizedFile(b, dir, "hash.dat", tc.size)
			info, err := os.Stat(path)
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for b.Loop() {
				if _, err := dfs.NewDfile(path, info.Size()); err != nil {
					b.Fatalf("hash failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkDWalkRun exercises the directory walker under different tree shapes.
func BenchmarkDWalkRun(b *testing.B) {
	setupBenchmark(b)

	scenarios := []struct {
		name        string
		depth       int
		breadth     int
		filesPerDir int
		fileSize    int
	}{
		{"Shallow", 1, 4, 8, 4 * 1024},
		{"Deep", 3, 2, 4, 2 * 1024},
		{"LargeFiles", 2, 3, 3, 512 * 1024},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		b.Run(scenario.name, func(b *testing.B) {
			root := b.TempDir()
			paths := createDirectoryTree(b, root, scenario.depth, scenario.breadth, scenario.filesPerDir, scenario.fileSize)
			expected := len(paths)

			b.ResetTimer()
			for b.Loop() {
				dFiles := make(chan *dfs.Dfile, expected)
				walker := dwalk.NewDWalker([]string{root}, dFiles)
				ctx := context.Background()
				walker.Run(ctx, 0, dwalk.MAX_FILE_SIZE)

				count := 0
				for range dFiles {
					count++
				}

				if count != expected {
					b.Fatalf("unexpected file count: got %d want %d", count, expected)
				}
			}
		})
	}
}

// BenchmarkMonitorLoop benchmarks the monitor loop that processes files and builds the duplicate map.
func BenchmarkMonitorLoop(b *testing.B) {
	setupBenchmark(b)

	root := b.TempDir()
	paths := createDuplicateFiles(b, root, 128, 8*1024)
	infos := mustStatPaths(b, paths)
	prehashed := mustMakeDfiles(b, paths, infos)
	expected := uint(len(prehashed))

	b.Run("Prehashed", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			result := runMonitorLoop(b, prehashed, nil)
			if result != expected {
				b.Fatalf("unexpected file count: got %d want %d", result, expected)
			}
		}
	})

	b.Run("WithHashing", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			result := runMonitorLoop(b, nil, func() []*dfs.Dfile {
				files := make([]*dfs.Dfile, 0, len(paths))
				for idx, path := range paths {
					df, err := dfs.NewDfile(path, infos[idx].Size())
					if err != nil {
						b.Fatalf("NewDfile failed: %v", err)
					}
					files = append(files, df)
				}
				return files
			})
			if result != expected {
				b.Fatalf("unexpected file count: got %d want %d", result, expected)
			}
		}
	})

	b.Run("ConcurrentProducers", func(b *testing.B) {
		workers := runtime.GOMAXPROCS(0)
		b.ResetTimer()
		for b.Loop() {
			result := runMonitorLoopConcurrent(b, prehashed, workers)
			if result != expected {
				b.Fatalf("unexpected file count: got %d want %d", result, expected)
			}
		}
	})
}

// BenchmarkDmapOperations benchmarks core Dmap operations.
func BenchmarkDmapOperations(b *testing.B) {
	setupBenchmark(b)

	root := b.TempDir()
	paths := createDuplicateFiles(b, root, 256, 4*1024)
	infos := mustStatPaths(b, paths)
	prehashed := mustMakeDfiles(b, paths, infos)

	b.ResetTimer()
	for b.Loop() {
		dMap, err := dmap.NewDmap(config.Config{})
		if err != nil {
			b.Fatal(err)
		}

		for _, df := range prehashed {
			dMap.Add(df)
		}

		_ = dMap.MapSize()
		_ = dMap.GetMap()
	}
}

// runMonitorLoop is a helper that runs the monitor loop using either a fixed slice
// of dfiles or a factory function that returns a fresh slice.
func runMonitorLoop(b *testing.B, cached []*dfs.Dfile, factory func() []*dfs.Dfile) uint {
	b.Helper()

	dMap, err := dmap.NewDmap(config.Config{})
	if err != nil {
		b.Fatal(err)
	}

	var files []*dfs.Dfile
	if cached != nil {
		files = cached
	} else {
		files = factory()
	}

	dFiles := make(chan *dfs.Dfile, len(files))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for df := range dFiles {
			dMap.Add(df)
		}
	}()

	for _, df := range files {
		dFiles <- df
	}
	close(dFiles)
	wg.Wait()

	return dMap.FileCount()
}

// runMonitorLoopConcurrent stresses the monitor loop with multiple producers feeding the channel.
func runMonitorLoopConcurrent(b *testing.B, files []*dfs.Dfile, workers int) uint {
	b.Helper()

	dMap, err := dmap.NewDmap(config.Config{})
	if err != nil {
		b.Fatal(err)
	}

	if workers <= 0 {
		workers = 1
	}

	dFiles := make(chan *dfs.Dfile, len(files))
	var consumer sync.WaitGroup
	consumer.Add(1)
	go func() {
		defer consumer.Done()
		for df := range dFiles {
			dMap.Add(df)
		}
	}()

	chunk := chunkSize(len(files), workers)
	var producers sync.WaitGroup
	for i := 0; i < workers; i++ {
		start := i * chunk
		if start >= len(files) {
			break
		}
		end := start + chunk
		if end > len(files) {
			end = len(files)
		}
		slice := files[start:end]
		producers.Add(1)
		go func(batch []*dfs.Dfile) {
			defer producers.Done()
			for _, df := range batch {
				dFiles <- df
			}
		}(slice)
	}

	producers.Wait()
	close(dFiles)
	consumer.Wait()

	return dMap.FileCount()
}

// createDirectoryTree builds a directory tree with predictable fanout.
func createDirectoryTree(tb testing.TB, root string, depth, breadth, filesPerDir, fileSize int) []string {
	tb.Helper()

	var paths []string
	var build func(level int, dir string)
	build = func(level int, dir string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatalf("mkdir %s: %v", dir, err)
		}

		for i := 0; i < filesPerDir; i++ {
			name := fmt.Sprintf("file_%d_%d.dat", level, i)
			paths = append(paths, makeSizedFile(tb, dir, name, fileSize))
		}

		if level >= depth {
			return
		}

		for i := 0; i < breadth; i++ {
			sub := filepath.Join(dir, fmt.Sprintf("dir_%d_%d", level+1, i))
			build(level+1, sub)
		}
	}

	build(0, root)
	return paths
}

// createDuplicateFiles creates matching pairs of files so that hashing and monitor loop benchmarks see duplicates.
func createDuplicateFiles(tb testing.TB, dir string, duplicates, size int) []string {
	tb.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", dir, err)
	}

	var paths []string
	for i := range duplicates {
		content := bytes.Repeat([]byte{byte(i % 251)}, size)
		left := filepath.Join(dir, fmt.Sprintf("dupA_%03d.dat", i))
		right := filepath.Join(dir, fmt.Sprintf("dupB_%03d.dat", i))

		if err := os.WriteFile(left, content, 0o644); err != nil {
			tb.Fatalf("write %s: %v", left, err)
		}
		if err := os.WriteFile(right, content, 0o644); err != nil {
			tb.Fatalf("write %s: %v", right, err)
		}

		paths = append(paths, left, right)
	}

	return paths
}

// makeSizedFile writes a file of the requested size using deterministic content.
func makeSizedFile(tb testing.TB, dir, name string, size int) string {
	tb.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", dir, err)
	}

	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		tb.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()

	if size == 0 {
		return path
	}

	const chunkSize = 32 * 1024
	chunk := bytes.Repeat([]byte{0xA5}, min(size, chunkSize))
	remaining := size
	for remaining > 0 {
		writeLen := min(remaining, chunkSize)
		if _, err := file.Write(chunk[:writeLen]); err != nil {
			tb.Fatalf("write %s: %v", path, err)
		}
		remaining -= writeLen
	}

	return path
}

// mustStatPaths retrieves os.FileInfo for each path.
func mustStatPaths(tb testing.TB, paths []string) []os.FileInfo {
	tb.Helper()

	infos := make([]os.FileInfo, len(paths))
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			tb.Fatalf("stat %s: %v", path, err)
		}
		infos[i] = info
	}
	return infos
}

// mustMakeDfiles precomputes dfs.Dfile instances for the supplied paths.
func mustMakeDfiles(tb testing.TB, paths []string, infos []os.FileInfo) []*dfs.Dfile {
	tb.Helper()

	files := make([]*dfs.Dfile, 0, len(paths))
	for i, path := range paths {
		df, err := dfs.NewDfile(path, infos[i].Size())
		if err != nil {
			tb.Fatalf("NewDfile(%s) failed: %v", path, err)
		}
		files = append(files, df)
	}
	return files
}

// chunkSize calculates work distribution for concurrent producers.
func chunkSize(total, workers int) int {
	if workers <= 0 {
		return total
	}
	size := total / workers
	if size*workers < total {
		size++
	}
	if size == 0 {
		size = 1
	}
	return size
}
