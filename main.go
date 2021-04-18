package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	_ "github.com/karrick/godirwalk"
)

var done = make(chan bool)

func main() {

	var (
		flStartDirectory = flag.String("dir", ".", "Root directory to search for duplicates")
		flWorkers        = flag.Int("workers", runtime.NumCPU(), "Number of workers")
	)
	flag.Parse()

	if len(os.Args) < 2 {
		fmt.Println("Usage: <STARTING_DIRECTORY>\n")
		return
	}

	// Our root directory. We start crawling from here.
	// TODO: Add more realistic user options, for now just supply
	//       a directory to start from for testing..
	rootDirectory = os.Args[1]

	go func() {
		// Read a single byte. This will interrupt ditto.
		os.Stdin.Read(make([]byte, 1))
		close(done)
	}()

	var count, total int
	var err error

	// Iterate through our FS
	for _, arg := range os.Args[1:] {
		count, err = buildFileDuplicateMap(arg)
		total += count
		if err != nil {
			break
		}
	}

	// Display files processed
	fmt.Fprintf(os.Stderr, "Processed %d files and directories\n", total)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}

}

// buildDupMap will walk our filesystem, hash our files, add
// to our primary map of potential duplicate files.
func buildDupMap(path string) error {

}
