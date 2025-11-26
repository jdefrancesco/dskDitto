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
const ver = "0.1"

func init() {

	// Custom help message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dskDitto [options] PATHS\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --no-banner                Do not show the dskDitto banner.\n")
		fmt.Fprintf(os.Stderr, "  --version                  Display version information.\n")
		fmt.Fprintf(os.Stderr, "  --profile <file>           Write CPU profile to disk for analysis.\n")
		fmt.Fprintf(os.Stderr, "  --time-only                Report scan duration only (for development).\n")
		fmt.Fprintf(os.Stderr, "  --min-size <bytes>         Skip files smaller than the given size.\n")
		fmt.Fprintf(os.Stderr, "  --max-size <bytes>         Skip files larger than the given size (default 4GiB).\n")
		fmt.Fprintf(os.Stderr, "  --text-output              Emit duplicate results in text-friendly format.\n")
		fmt.Fprintf(os.Stderr, "  --bullets                  Show duplicates as a formatted bullet list.\n")
		fmt.Fprintf(os.Stderr, "  --pretty                   Render duplicates as a tree (slower for large sets).\n")
		fmt.Fprintf(os.Stderr, "  --ignore-empty             Ignore empty files (default true).\n")
		fmt.Fprintf(os.Stderr, "  --no-symlinks              Skip symbolic links (default true).\n")
		fmt.Fprintf(os.Stderr, "  --no-hidden                Skip hidden dotfiles and directories (default true).\n")
		fmt.Fprintf(os.Stderr, "  --dups <count>             Require at least this many files per duplicate group (default 2).\n")
		fmt.Fprintf(os.Stderr, "  --remove <keep>            Delete duplicates, keeping only <keep> files per group.\n\n")
		fmt.Fprintf(os.Stderr, "Notes:\n")
		fmt.Fprintf(os.Stderr, "  Display-oriented options like --pretty only render results; no files are removed.\n")
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
		flTextOutput   = flag.Bool("text", false, "Dump results in grep/text friendly format. Useful for scripting.")
		flShowBullets  = flag.Bool("bullets", false, "Show duplicates as formatted bullet list.")
		flShowPretty   = flag.Bool("pretty", false, "Show pretty output of duplicates found as tree.")
		flIgnoreEmpty  = flag.Bool("ignore-empty", true, "Ignore empty files (0 bytes).")
		flSkipSymLinks = flag.Bool("no-symlinks", true, "Skip symbolic links. This is on by default.")
		flSkipHidden   = flag.Bool("no-hidden", true, "Skip hidden files and directories (dotfiles).")
		flMinDups      = flag.Uint("dups", 2, "Minimum number of duplicates required to display a group.")
		flKeep         = flag.Uint("remove", 0, "Delete duplicates, keeping only this many files per group.")
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
		os.Exit(0)
	}

	// TODO: Refactor and pull this out into proper package.
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

	// Keep SHA256 for now since BLAKE3 implementation is abysmal...
	hashAlgo := dfs.HashSHA256
	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Dmap stores duplicate file information. Failure is fatal.
	minDups := *flMinDups
	if minDups < 2 {
		fmt.Fprintf(os.Stderr, "invalid duplicate threshold %d; must be at least 2\n", minDups)
		os.Exit(1)
	}

	keepCount := *flKeep
	if keepCount == 0 {
		// Leave as zero to indicate no removal requested.
	}

	dMap, err := dmap.NewDmap(config.Config{
		SkipEmpty:     *flIgnoreEmpty,
		SkipSymLinks:  *flSkipSymLinks,
		SkipHidden:    *flSkipHidden,
		MinFileSize:   MinFileSize,
		MaxFileSize:   MaxFileSize,
		MinDuplicates: minDups,
		HashAlgorithm: hashAlgo,
	})

	if err != nil {
		dsklog.Dlogger.Fatal("Failed to make new Dmap: ", err)
		os.Exit(1)
	}

	// Receive files we need to process via this channel.
	// Use buffered channel to allow async file discovery and processing
	dFiles := make(chan *dfs.Dfile, 1000)

	walker := dwalk.NewDWalker(rootDirs, dFiles, hashAlgo, *flSkipHidden)
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

	// Zero value for moveKeep means don't remove anything..
	if keepCount > 0 {
		removedPaths, removeErr := dMap.RemoveDuplicates(keepCount)
		fmt.Printf("Removed %d duplicate files, kept %d per group.\n", len(removedPaths), keepCount)
		if removeErr != nil {
			fmt.Fprintf(os.Stderr, "Removal completed with errors: %v\n", removeErr)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	// For debugging to test speed
	if *flTimeOnly {
		os.Exit(0)
	}

	// Dump results in various format. No interactive results are shown. These
	// options are better for scripting or grepping through.
	switch {
	case *flTextOutput:
		dMap.PrintDmap()
		os.Exit(0)
	case *flShowPretty:
		dMap.ShowResultsPretty()
		os.Exit(0)
	case *flShowBullets:
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
