package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/internal/ui"
	"github.com/jdefrancesco/dskDitto/pkg/utils"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

// Version
const ver = "0.1"

func init() {

	// Custom help message
	flag.Usage = func() {
		showHeader()
		fmt.Fprintf(os.Stderr, "Usage: dskDitto [options] PATHS\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --no-banner                Do not show the dskDitto banner.\n")
		fmt.Fprintf(os.Stderr, "  --version                  Display version information.\n")
		fmt.Fprintf(os.Stderr, "  --profile <file>           Write CPU profile to disk for analysis.\n")
		fmt.Fprintf(os.Stderr, "  --time-only                Report scan duration only (for development).\n")
		fmt.Fprintf(os.Stderr, "  --min-size <size>          Skip files smaller than the given size (e.g. 512K, 5MiB).\n")
		fmt.Fprintf(os.Stderr, "  --max-size <size>          Skip files larger than the given size (default 4GiB).\n")
		fmt.Fprintf(os.Stderr, "  --text               	     Emit duplicate results in text-friendly format.\n")
		fmt.Fprintf(os.Stderr, "  --bullet                   Show duplicates as a formatted bullet list.\n")
		fmt.Fprintf(os.Stderr, "  --empty                    Include empty files (default: ignore).\n")
		fmt.Fprintf(os.Stderr, "  --no-symlinks              Skip symbolic links (default true).\n")
		fmt.Fprintf(os.Stderr, "  --hidden                   Include hidden dotfiles and directories (default: ignore).\n")
		fmt.Fprintf(os.Stderr, "  --current                  Do not descend into subdirectories.\n")
		fmt.Fprintf(os.Stderr, "  --depth <levels>           Limit recursion to <levels> directories below the start paths.\n")
		fmt.Fprintf(os.Stderr, "  --include-vfs              Include virtual filesystem directories like /proc or /dev.\n")
		fmt.Fprintf(os.Stderr, "  --dups <count>             Require at least this many files per duplicate group (default 2).\n")
		fmt.Fprintf(os.Stderr, "  --remove <keep>            Delete duplicates, keeping only <keep> files per group.\n")
		fmt.Fprintf(os.Stderr, "  --fs-detect <path>         Detect and display the filesystem containing path.\n\n")
		fmt.Fprintf(os.Stderr, "Notes:\n")
		fmt.Fprintf(os.Stderr, "  Display-oriented options like --bullet only render results; no files are removed.\n")
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
	dsklog.InitializeDlogger(".dskditto.log")
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
	// Note these messages aren't what user sees any longer. See flUsage for that.
	var (
		flNoBanner      = flag.Bool("no-banner", false, "Do not show the dskDitto banner.")
		flShowVersion   = flag.Bool("version", false, "Display version")
		flCpuProfile    = flag.String("profile", "", "Write CPU profile to disk for analysis.")
		flTimeOnly      = flag.Bool("time-only", false, "Use to show only the time taken to scan directory for duplicates.")
		flMinFileSize   = flag.String("min-size", "", "Skip files smaller than this size (supports suffixes like 512K, 5MiB).")
		flMaxFileSize   = flag.String("max-size", "", "Skip files larger than this size (default 4GiB).")
		flTextOutput    = flag.Bool("text", false, "Dump results in grep/text friendly format. Useful for scripting.")
		flShowBullets   = flag.Bool("bullet", false, "Show duplicates as formatted bullet list.")
		flIncludeEmpty  = flag.Bool("empty", false, "Include empty files (0 bytes).")
		flSkipSymLinks  = flag.Bool("no-symlinks", true, "Skip symbolic links. This is on by default.")
		flIncludeHidden = flag.Bool("hidden", false, "Include hidden files and directories (dotfiles).")
		flNoRecurse     = flag.Bool("current", false, "Only scan the provided directories without descending into subdirectories.")
		flDepth         = flag.Int("depth", -1, "Maximum recursion depth; 0 inspects only the provided paths, -1 means unlimited.")
		flIncludeVFS    = flag.Bool("include-vfs", false, "Include virtual filesystem mount points such as /proc and /dev.")
		flMinDups       = flag.Uint("dups", 2, "Minimum number of duplicates required to display a group.")
		flKeep          = flag.Uint("remove", 0, "Delete duplicates, keeping only this many files per group.")
		flDetectFS      = flag.String("fs-detect", "", "Detect filesystem in use by specified path")
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

	if *flDetectFS != "" {
		fs, err := dfs.DetectFilesystem(".")
		if err != nil {
			panic(err)
		}
		fmt.Printf("Filesystem: %s\n\n", fs)
	}

	// Largest maximum integer.
	maxUint := ^uint(0)

	MinFileSize := uint(0)
	if *flMinFileSize != "" {
		value, err := utils.ParseSize(*flMinFileSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value for --min-size: %v\n", err)
			os.Exit(1)
		}
		if value > uint64(maxUint) {
			fmt.Fprintf(os.Stderr, "--min-size %s exceeds platform limit (%d bytes)\n", *flMinFileSize, maxUint)
			os.Exit(1)
		}
		MinFileSize = uint(value)
		if MinFileSize > 0 {
			fmt.Printf("Skipping files smaller than: %s (%d bytes).\n", utils.DisplaySize(uint64(MinFileSize)), MinFileSize)
		}
		dsklog.Dlogger.Debugf("Min file size set to %d bytes.\n", MinFileSize)
	}

	MaxFileSize := dwalk.MAX_FILE_SIZE // Default is 4 GiB.
	if *flMaxFileSize != "" {
		value, err := utils.ParseSize(*flMaxFileSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value for --max-size: %v\n", err)
			os.Exit(1)
		}
		if value > uint64(math.MaxUint) {
			fmt.Fprintf(os.Stderr, "--max-size %s exceeds platform limit (%d bytes)\n", *flMaxFileSize, maxUint)
			os.Exit(1)
		}
		if value > 0 {
			MaxFileSize = uint(value)
			fmt.Printf("Skipping files larger than: %s (%d bytes).\n", utils.DisplaySize(uint64(MaxFileSize)), MaxFileSize)
		}
		dsklog.Dlogger.Debugf("Max file size set to %d bytes.\n", MaxFileSize)
	}

	if *flDepth < -1 {
		dsklog.Dlogger.Debugf("Invalid depth of %d \n", *flDepth)
		fmt.Fprintf(os.Stderr, "invalid depth %d; must be -1 or greater\n", *flDepth)
		os.Exit(1)
	}

	maxDepth := -1
	if *flDepth >= 0 {
		maxDepth = *flDepth
	}
	if *flNoRecurse {
		maxDepth = 0
	}

	if maxDepth == 0 && (*flNoRecurse || *flDepth >= 0) {
		dsklog.Dlogger.Debug("Recursion disabled. Invoked with current flag. Only checking current directory for dups.")
	} else if maxDepth > 0 {
		dsklog.Dlogger.Debugf("Limiting recursion depth to %d level(s).\n", maxDepth)
	}

	fmt.Printf("[!] Press CTRL+C to stop dskDitto at any time.\n")

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
		dsklog.Dlogger.Debug("No removal requested. keepCount is zero")
	}

	// Hold app config.
	appCfg := config.Config{
		SkipEmpty:     !*flIncludeEmpty,
		SkipSymLinks:  *flSkipSymLinks,
		SkipHidden:    !*flIncludeHidden,
		SkipVirtualFS: !*flIncludeVFS,
		MaxDepth:      maxDepth,
		MinFileSize:   MinFileSize,
		MaxFileSize:   MaxFileSize,
		MinDuplicates: minDups,
		HashAlgorithm: hashAlgo,
	}

	dMap, err := dmap.NewDmap(appCfg.MinDuplicates)

	if err != nil {
		dsklog.Dlogger.Fatal("Failed to make new Dmap: ", err)
		os.Exit(1)
	}

	// Receive files we need to process via this channel.
	// Settled on using 1 for high throughput and no hidden back pressure that
	// I may have been hiding with buffered channel.
	dFiles := make(chan *dfs.Dfile, 1)

	walker := dwalk.NewDWalker(rootDirs, dFiles, appCfg)
	walker.Run(ctx)

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

	fmt.Println()
	// Dump results in various format. No interactive results are shown. These
	// options are better for scripting or grepping through.
	switch {
	case *flTextOutput:
		dMap.PrintDmap()
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
	fmt.Printf("Github: https://github.com/jdefrancesco/dskDitto")
}
