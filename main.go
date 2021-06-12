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
	"ditto/dmap"
	"ditto/dwalk"

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

	var rootDirs []string
	if len(os.Args) > 1 && os.Args[1] != "" {
		rootDirs = []string{os.Args[1]}
		log.Info().Msgf("rootDir %s\n", rootDirs)
	} else {
		rootDirs = []string{"."}
		log.Info().Msgf("rootDir %s\n", rootDirs)
	}

	go func() {
		// This will quit dskditto!
		os.Stdin.Read(make([]byte, 1))
		cancel()
	}()

	// Create Dmap to house our duplicate file information.
	dMap, err := dmap.NewDmap()
	if err != nil {
		log.Fatal().Msgf("could not create new dmap: %s\n", err)
	}

	// dFiles will be the channel we receive files to be added to the DMap.
	dFiles := make(chan *dfs.Dfile)
	walker := dwalk.NewDWalker(rootDirs, dFiles)

	// Kickoff filesystem walking.
	walker.Run(ctx)

	var nfiles int64
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
				log.Info().Msg("dFile channel closed\n")
				break MainLoop
			}
			// Add dFile to our DMap
			nfiles++
			dMap.Add(dFile)
		case <-tick:
			// Display progress information.
			log.Info().Msgf("Files processed: %d.\n", nfiles)
		}
	}

	// Show final results.
	fmt.Printf("%d files processed.\n", nfiles)
	dMap.PrintDmap()

}
