package main

import (
	"context"
	"fmt"
	"os"
	"time"

	_ "flag"
	_ "io/ioutil"
	_ "runtime"

	"ditto/dfs"

	_ "github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func main() {

	// var (
	// 	flStartDirectory = flag.String("dir", ".", "Root directory to search for duplicates")
	// 	flWorkers        = flag.Int("workers", runtime.NumCPU(), "Number of workers")
	// )
	// flag.Parse()

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	var rootDir string
	if len(os.Args) > 1 && os.Args[1] != "" {
		rootDir = os.Args[1]
		log.Info().Msgf("rootDir %s", rootDir)
	} else {
		rootDir = "."
		log.Info().Msgf("rootDir %s", rootDir)
	}

	go func() {
		// This will kill dskditto!
		os.Stdin.Read(make([]byte, 1))
		cancel()
	}()

	// Create DMap to house our duplicate file information.
	dMap, err := NewDMap()
	if err != nil {
		log.Fatal().Msgf("could not create new dmap: %s", err)
	}

	// dFiles will be the channel we recieve files to be processed over.
	dFiles := make(chan dfs.Dfile)
	walker := NewDWalker(rootDirs, dFiles)

	// Kickoff filesystem walking.
	walker.Run(ctx)

	var nfiles, nbytes int64
	tick := time.Tick(500 * time.Millisecond)

MainLoop:
	for {
		select {
		case <-ctx.Done():
			// Drain dFiles.
			for range dFiles {
			}
			break MainLoop
		case dFile, ok := <-dFiles:
			if !ok {
				break MainLoop
			}
			// Add dFile to our DMap
			dMap.Add(dFile)
		case <-tick:
			// Display progress information.
			log.Info().Msgf("Tick...")
		}
	}

	// Show final results.
	fmt.Printf("%d files %.1f GB\n", nfiles, float64(nbytes)/1e9)

}
