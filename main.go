package main

import (
	_ "flag"
	"fmt"
	"os"
	_ "runtime"
	// "time"
	_ "io/ioutil"
	
	_"ditto/dfs"

	_ "github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog"
)


func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

var done = make(chan struct{})

func main() {

	// var (
	// 	flStartDirectory = flag.String("dir", ".", "Root directory to search for duplicates")
	// 	flWorkers        = flag.Int("workers", runtime.NumCPU(), "Number of workers")
	// )
	// flag.Parse()

	var rootDir string
	if len(os.Args) > 1 && os.Args[1] != "" {
		rootDir = os.Args[1] 
		log.Info().Msgf("rootDir %s", rootDir)
	} else {
		rootDir = "."
		log.Info().Msgf("rootDir %s", rootDir)
	}

	go func() {
		// Read a single byte. This will interrupt ditto.
		os.Stdin.Read(make([]byte, 1))
		// Replace me with cancelled() 
		close(done)
	}()

	// Read starting directory.

	// dfiles := make(chan dfs.Dfile)
	// walker = dwalk.NewDWalker(rootDir, walker)
	// walker
	


	// Print the results periodically.
	// tick := time.Tick(500 * time.Millisecond)

	// var total int64
	// total = 0

	// Monitor loop
	// TODO: CORE LOOP
	// dfiles := make(chan dfs.Dfile)
	// for {
	// 	select {
	// 	case <-tick:
	// 		// display progress
	// 		fmt.Println("tick...")
	// 	}

	// }

	// Display files processed
	// fmt.Fprintf(os.Stderr, "Processed %d files and directories\n", total)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	// 	os.Exit(1)
	// }
	fmt.Println(total)

}

// buildDupMap will walk our filesystem, hash our files, add
// to our primary map of potential duplicate files.
// func buildDupMap(path string) error {

// }
