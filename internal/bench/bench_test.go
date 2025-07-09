// This package contains benchmark related logic/tests.
package bench

import (
	"os"
	"testing"

	"ditto/internal/dfs"
)

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
	for i := 0; i < b.N; i++ {
		_, err := dfs.NewDfile(tmpFile.Name(), info.Size())
		if err != nil {
			b.Fatalf("NewDfile failed: %v:", err)
		}
	}
}
