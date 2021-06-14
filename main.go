package main

import (
	"context"
	"fmt"
	"os"
	"time"

	_ "flag"

	"ditto/dfs"
	"ditto/dmap"
	"ditto/dwalk"

	"github.com/pterm/pterm"
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

	showHeader()

	if os.Args[1] == "--header" {
		os.Exit(0)
	}

	rootDirs := os.Args[1:]
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	// Allow user to quit dskditto.
	go func() {
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
	// This is so we can show progress to user every half second.
	// TODO: Make configurable via command options
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
			nfiles++
		case <-tick:
			// Display progress information.
			log.Info().Msgf("Files processed: %d", nfiles)
		}
	}

	// Show final results.
	pterm.Success.Println("Total of", nfiles, "files processed. Duplicates:")
	dMap.ShowResults()

}

// showHeader prints colorful dskDitto logo.
func showHeader() {

	// Tiny little space between the shell prompt and our logo.
	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		pterm.NewLettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		pterm.NewLettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}
