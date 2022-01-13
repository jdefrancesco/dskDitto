package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"time"

	"ditto/dfs"
	"ditto/dmap"
	"ditto/dwalk"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
)

var fileLogger zerolog.Logger

func init() {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "dskditto-main")
	if err != nil {
		fmt.Printf("Error creating log file\n")
	}
	fileLogger := zerolog.New(tmpFile).With().Logger()
	fileLogger.Info().Msg("DskDitto Log:")

	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n\n")
		flag.PrintDefaults()
	}
}

func main() {

	var (
		flNoBanner   = flag.Bool("no-banner", false, "Do not show the dskDitto banner")
		flCpuProfile = flag.String("cpuprofile", "", "Write CPU profile to disk for analysis")
		flNoResults  = flag.Bool("time-only", false, "Use to show only the time taken to scan directory")
	)
	flag.Parse()

	// Enable CPU profiling
	if *flCpuProfile != "" {
		// Output to file
		f, err := os.Create(*flCpuProfile)
		if err != nil {
			fileLogger.Fatal().Err(err).Msgf("cpuprofile failed")
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

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
		fileLogger.Fatal().Msgf("could not create new dmap: %s\n", err)
	}

	// dFiles will be the channel we receive files to be added to the DMap.
	dFiles := make(chan *dfs.Dfile)
	walker := dwalk.NewDWalker(rootDirs, dFiles)

	walker.Run(ctx)
	start := time.Now()

	// Number of files we been sent for processing.
	var nfiles int64
	// Show progress to user at intervals specified by tick.
	tick := time.Tick(time.Duration(500) * time.Millisecond)

	infoSpinner, _ := pterm.DefaultSpinner.Start()

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
			dMap.Add(dFile)
			nfiles++
		case <-tick:
			// Display progress information.
			progressMsg := fmt.Sprintf("Processed %d files...", nfiles)
			infoSpinner.UpdateText(progressMsg)
		}
	}

	infoSpinner.Stop()
	// Get elapsed time of scan.
	duration := time.Since(start)

	// Scan success message
	var finalInfo string
	finalInfo = "Total of " + pterm.LightWhite(nfiles) + " files processed in " + pterm.LightWhite(duration) + ". Duplicates: "
	pterm.Success.Println(finalInfo)

	// XXX: FOR DEBUGGING TO TEST SPEED
	if *flNoResults {
		os.Exit(0)
	}
	// TODO: If more than 2-3 duplicates simply print duplicate count out to user as result to save space.
	// The actual; results we need to write to a file so they can be processed according to users desire (rmeove or keep them)
	dMap.PrintDmap()

	// Show final results.
	// dMap.ShowResults()

}

// showHeader prints colorful dskDitto fileLoggero.
func showHeader() {

	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		pterm.NewLettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		pterm.NewLettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}
