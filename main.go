package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {

	var (
		startDirectory = flag.String("dir", ".", "Root directory to search for duplicates")
	)

	if len(os.Args) != 2 {
		fmt.Println("Usage: ", os.Args[0], "<STARTING_DIRECTORY>")
		return
	}

	rootDirectory = *startDirectory

	go func() {
		// Read a single byte. This will interrupt ditto.
		os.Stdin.Read(make([]byte, 1))
		close(done)
	}()

}
