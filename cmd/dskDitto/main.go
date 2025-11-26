package main

import (
	"context"
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"ditto/internal/config"
	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"
	"ditto/internal/dwalk"
	"ditto/internal/ui"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

// Version
const ver = "0.0.1"

func init() {

	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n")
		flag.PrintDefaults()
		fmt.Println("")
		fmt.Printf("[note] Display options like --pretty will only show results. No file removal occurs.\n")
		fmt.Printf("[note] Double dash notation works too. Example: --no-banner.\n")
	}
}

// signalHandler will handle SIGINT and others in order to
// gracefully shutdown.
func signalHandler(ctx context.Context, sig os.Signal) {
	dsklog.Dlogger.Infoln("Signal received")

	// The terminal settings might be in a state that messes up
	// future output. To be safe I reset them.
	ui.StopTUI()

	switch sig {
	case syscall.SIGINT:
		fmt.Fprintf(os.Stderr, "\r[!] SIGINT! Quitting...\n")
		ctx.Done()
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "\r[!] Unhandled/Unknown signal.\n")
		ctx.Done()
		os.Exit(1)
	}
}

func main() {

	// Initialize logger
	dsklog.InitializeDlogger("app.log")
	dsklog.Dlogger.Info("Logger initialized")

	// Setup signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			sig := <-sigChan
			signalHandler(ctx, sig)
		}
	}()

	// Parse command flags.
	var (
		flNoBanner     = flag.Bool("no-banner", false, "Do not show the dskDitto banner.")
		flShowVersion  = flag.Bool("version", false, "Display version")
		flCpuProfile   = flag.String("profile", "", "Write CPU profile to disk for analysis.")
		flTimeOnly     = flag.Bool("time-only", false, "Use to show only the time taken to scan directory for duplicates. Useful for development.")
		flMinFileSize  = flag.Uint("min-size", 0, "Skip files smaller than this size in bytes.")
		flMaxFileSize  = flag.Uint("max-size", 0, "Max file size is 4 GiB by default.")
		flTextOutput   = flag.Bool("text-output", false, "Dump results in grep/text friendly format. Useful for scripting.")
		flShowBullets  = flag.Bool("bullets", false, "Show duplicates as formatted bullet list.")
		flShowPretty   = flag.Bool("pretty", false, "Show pretty output of duplicates found as tree.")
		flIgnoreEmpty  = flag.Bool("ignore-empty", true, "Ignore empty files (0 bytes).")
		flSkipSymLinks = flag.Bool("no-symlinks", true, "Skip symbolic links. This is on by default.")
	)
	flag.Parse()

	// Enable CPU profiling
	if *flCpuProfile != "" {
		f, err := os.Create(*flCpuProfile)
		if err != nil {
			dsklog.Dlogger.Info("profile failed")
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
	}

	if !*flNoBanner {
		showHeader()
	}

	// Just show version then quit.
	if *flShowVersion {
		showVersion()
	}

	var MinFileSize uint = 0
	if *flMinFileSize != 0 {
		MinFileSize = *flMinFileSize
		fmt.Printf("Skipping files smaller than: %d bytes.\n", MinFileSize)
		dsklog.Dlogger.Infof("Min file size set to %d bytes.\n", MinFileSize)
	}

	var MaxFileSize uint = dwalk.MAX_FILE_SIZE // Default is 4 GiB.
	if *flMaxFileSize != 0 {
		MaxFileSize = *flMaxFileSize
		fmt.Printf("Skipping files larger than: %d bytes.\n", MaxFileSize)
		dsklog.Dlogger.Infof("Max file size set to %d bytes.\n", MaxFileSize)
	}

	fmt.Printf("[!] Press CTRL+C to stop dskDitto at any time.\n")

	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Dmap stores duplicate file information. Failure is fatal.
	dMap, err := dmap.NewDmap(config.Config{
		SkipEmpty:    *flIgnoreEmpty,
		SkipSymLinks: *flSkipSymLinks,
		MinFileSize:  MinFileSize,
		MaxFileSize:  MaxFileSize,
	})

	if err != nil {
		dsklog.Dlogger.Fatal("Failed to make new Dmap: ", err)
		os.Exit(1)
	}

	// Receive files we need to process via this channel.
	// Use buffered channel to allow async file discovery and processing
	dFiles := make(chan *dfs.Dfile, 1000)

	walker := dwalk.NewDWalker(rootDirs, dFiles)
	walker.Run(ctx, MinFileSize, MaxFileSize)

	start := time.Now()

	// Show progress to user at intervals specified by tick.
	tick := time.Tick(time.Duration(500) * time.Millisecond)
	infoSpinner, _ := pterm.DefaultSpinner.Start()

	// Number of files we have processed so far.
	var nfiles uint

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
			if dFile == nil {
				dsklog.Dlogger.Warn("Received nil dFile, skipping...")
				continue
			}
			// Add the file to our map.
			dMap.Add(dFile)
			nfiles++

		case <-tick:
			// Display progress information.
			progressMsg := fmt.Sprintf("Processed %d files...", nfiles)
			infoSpinner.UpdateText(progressMsg)
		}
	}

	infoSpinner.Stop()
	duration := time.Since(start)

	finalInfo := "Total of " + pterm.LightWhite(nfiles) + " files processed in " +
		pterm.LightWhite(duration)
	pterm.Success.Println(finalInfo)

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	// For debugging to test speed
	if *flTimeOnly {
		os.Exit(0)
	}

	// Dump results to stdout. Useful for scripting.
	if *flTextOutput {
		dMap.PrintDmap()
		os.Exit(0)
	}

	// Dump pretty using Pterm. Not default because it can be slow for large sets.
	if *flShowPretty {
		dMap.ShowResultsPretty()
		os.Exit(0)
	}

	// Dump as bullets.
	if *flShowBullets {
		dMap.ShowResultsBullet()
		os.Exit(0)
	}

	// Show TUI interactive interface.
	ui.LaunchTUI(dMap)
}

// showHeader prints colorful dskDitto banner.
func showHeader() {

	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		putils.LettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}

func showVersion() {
	fmt.Printf("Version: %s\n", ver)
}
