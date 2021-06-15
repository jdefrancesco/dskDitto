package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"ditto/dfs"
	"ditto/dmap"
	"ditto/dwalk"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n\n")
		flag.PrintDefaults()
	}
}

func main() {

	var (
		flNoBanner     = flag.Bool("no-banner", false, "Do not show the dskDitto banner")
		flProgressTime = flag.Int("show-progress", 500, "Progress time in miliseconds")
	)
	flag.Parse()

	// Toggle banner.
	if !*flNoBanner {
		showHeader()
	}

	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	// Allow user to quit dskDitto.
	go func() {
		os.Stdin.Read(make([]byte, 1))
		cancel()
	}()

	// Dmap stores duplicate file information.
	dMap, err := dmap.NewDmap()
	if err != nil {
		log.Fatal().Msgf("could not create new dmap: %s\n", err)
	}

	// dFiles will be the channel we receive files to be added to the DMap.
	dFiles := make(chan *dfs.Dfile)
	walker := dwalk.NewDWalker(rootDirs, dFiles)

	// Kickoff filesystem walking.
	walker.Run(ctx)
	// Start our clock so we can track scan time.
	start := time.Now()

	// Number of files we been sent for processing.
	var nfiles int64

	// This is so we can show progress to user at specified interval.
	tick := time.Tick(time.Duration(*flProgressTime) * time.Millisecond)

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

	// Get elapsed time of entire scan.
	duration := time.Since(start)

	// Show final results.
	pterm.Success.Println("Total of", nfiles, "files processed in", duration, "Duplicates:")
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
