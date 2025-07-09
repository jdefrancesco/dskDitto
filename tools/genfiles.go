//go:build tools
// +build tools

package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var sizes = []int64{
	1024,              // 1KB
	1024 * 1024,       // 1MB
	100 * 1024 * 1024, // 100MB
}

func randBytes(size int64) []byte {
	b := make([]byte, size)
	_, _ = rand.Read(b)
	return b
}

func createFiles(dir string, n int) error {
	fmt.Println("createFiles running...")
	for i := 0; i < n; i++ {
		size := sizes[i%len(sizes)]
		fname := fmt.Sprintf("file_%d.dat", i)
		path := filepath.Join(dir, fname)

		fmt.Printf("Creating: %s\n", path)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, &randReader{remaining: size})
		if err != nil {
			return err
		}
	}
	return nil
}

type randReader struct {
	remaining int64
}

func (r *randReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := rand.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func main() {
	dir := "./files"
	os.MkdirAll(dir, 0755)
	n := 100
	if err := createFiles(dir, n); err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Created", n, "test files in", dir)
}
