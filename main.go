package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"ditto/dfs"
	"ditto/dmap"
	"ditto/dwalk"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/rivo/tview"
	"github.com/rs/zerolog"
)

var fileLogger zerolog.Logger

func init() {

	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n\n")
		flag.PrintDefaults()
	}
}

// signalHandler will handle SIGINT and others in order to
// gracefully shutdown.
func signalHandler(ctx context.Context, sig os.Signal) {
	switch sig {
	case syscall.SIGINT:
		fmt.Printf("\r[!] Signal %s (SIGINT). Quitting....\n", sig.String())
		ctx.Done()
		os.Exit(0)
	default:
		fmt.Printf("\r[!] Unhandled/Unknown signal\n")
		ctx.Done()
		os.Exit(0)
	}
}

func main() {

	// Setup channel for signal handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	// Watch for signal events, call handler to shutdown
	go func() {
		for {
			sig := <-sigChan
			signalHandler(ctx, sig)
		}
	}()

	// Parse command flags.
	var (
		flNoBanner    = flag.Bool("no-banner", false, "Do not show the dskDitto banner")
		flCpuProfile  = flag.String("cpuprofile", "", "Write CPU profile to disk for analysis")
		flNoResults   = flag.Bool("time-only", false, "Use to show only the time taken to scan directory")
		flMaxFileSize = flag.Int64("max-file-size", 1, "Max file size is 1 GiB by default")
	)
	flag.Parse()

	_ = flMaxFileSize

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
	finalInfo := "Total of " + pterm.LightWhite(nfiles) + " files processed in " +
		pterm.LightWhite(duration) + ". \nDuplicates: "
	pterm.Success.Println(finalInfo)

	//  FOR DEBUGGING TO TEST SPEED
	if *flNoResults {
		os.Exit(0)
	}

	if dMap.MapSize() < 50 {
		dMap.ShowAllResults()
		os.Exit(0)
	} else {
		var prompt string
		pterm.Success.Print("There are too many results to show. Launch TUI? (y/n) ")
		fmt.Scanf("%s", &prompt)
		// We exit without showing user anything...
		if prompt == "n" {
			os.Exit(0)
		}
	}

	// Launch TUI!
	// TODO: Refactor this into a function when I get my bearings...
	app := tview.NewApplication()
	list := tview.NewList()

	// for hash, files := range dMap.GetMap() {
	// 	if len(files) > 1 {
	// 		for _, file := range files {
	// 			list.AddItem(file, hash, rune(hash[0]), func() {
	// 				// Here you could delete the file or perform other actions.
	// 			})
	// 		}
	// 	}
	// }

	// list.SetSelectedFunc(func(i int, s string, r string, ru rune) {
	// 	// Delete the file when selected.
	// 	if err := os.Remove(s); err != nil {
	// 		panic(err)
	// 	}
	// 	list.RemoveItem(i)
	// })

	if err := app.SetRoot(list, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

	fmt.Println("")

}

// showHeader prints colorful dskDitto fileLoggero.
func showHeader() {

	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		putils.LettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}
