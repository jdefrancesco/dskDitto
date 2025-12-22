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
const ver = "0.2"

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
		fmt.Fprintf(os.Stderr, "  --remove <keep>            Operate on duplicates, keeping only <keep> files per group.\n")
		fmt.Fprintf(os.Stderr, "  --link                     With --remove, convert extra duplicates into symlinks instead of deleting them.\n")
		fmt.Fprintf(os.Stderr, "  --hash <algo>              Hash algorithm: sha256 (default) or blake3.\n")
		fmt.Fprintf(os.Stderr, "  --file <path>              Only report duplicates of the specified file.\n")
		fmt.Fprintf(os.Stderr, "  --csv-out <file>           Write duplicate groups to a CSV file.\n")
		fmt.Fprintf(os.Stderr, "  --json-out <file>          Write duplicate groups to a JSON file.\n")
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
		flHashAlgo      = flag.String("hash", "sha256", "Hash algorithm to use: sha256 (default) or blake3.")
		flKeep          = flag.Uint("remove", 0, "Operate on duplicates, keeping only this many files per group.")
		flLinkMode      = flag.Bool("link", false, "Convert extra duplicates into symlinks instead of deleting them (use with --remove).")
		flSingleFile    = flag.String("file", "", "Only search for duplicates of the specified file.")
		flCSVOut        = flag.String("csv-out", "", "Write duplicate groups to the specified CSV file.")
		flJSONOut       = flag.String("json-out", "", "Write duplicate groups to the specified JSON file.")
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

	fmt.Printf("[!] Press CTRL+C to stop dskDitto at any time.\n")

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

	// Maximum uint size.
	maxUint := ^uint(0)
	MinFileSize := uint(0)

	if *flMinFileSize != "" {
		value, err := utils.ParseSize(*flMinFileSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value for --min-size: %v\n", err)
			os.Exit(1)
		}
		if value > uint64(math.MaxUint) {
			fmt.Fprintf(os.Stderr, "--min-size %s exceeds platform limit (%d bytes)\n", *flMinFileSize, maxUint)
			os.Exit(1)
		}

		MinFileSize = uint(value)
		if MinFileSize > 0 {
			fmt.Printf("Skipping files smaller than: ~ %s.\n", utils.DisplaySize(uint64(MinFileSize)))
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

	hashAlgo, err := dfs.ParseHashAlgorithm(*flHashAlgo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unsupported hash algorithm %q; must be 'sha256' or 'blake3'\n", *flHashAlgo)
		os.Exit(1)
	}
	dsklog.Dlogger.Debugf("Using hash algorithm: %s", hashAlgo)

	singleFileMode := false
	var targetDigest dmap.Digest
	var targetFilePath string
	if *flSingleFile != "" {
		dsklog.Dlogger.Debug("In single shot mode.")
		info, statErr := os.Stat(*flSingleFile)
		if statErr != nil {
			dsklog.Dlogger.Debugf("Unable to stat --file path %s: %v\n", *flSingleFile, statErr)
			fmt.Fprintf(os.Stderr, "unable to stat --file path %s: %v\n", *flSingleFile, statErr)
			os.Exit(1)
		}
		if !info.Mode().IsRegular() {
			fmt.Fprintf(os.Stderr, "--file path must be a regular file: %s\n", *flSingleFile)
			os.Exit(1)
		}
		targetDfile, hashErr := dfs.NewDfile(*flSingleFile, info.Size(), hashAlgo)
		if hashErr != nil {
			dsklog.Dlogger.Debugf("Failed to hash --file target %s: %v\n", *flSingleFile, hashErr)
			fmt.Fprintf(os.Stderr, "failed to hash --file target %s: %v\n", *flSingleFile, hashErr)
			os.Exit(1)
		}
		targetDigest = dmap.Digest(targetDfile.Hash())
		targetFilePath = targetDfile.FileName()
		singleFileMode = true
		dsklog.Dlogger.Infof("Single file mode enabled for %s (%d bytes)", targetFilePath, info.Size())
		pterm.Info.Printf("Searching for duplicates of %s\n", targetFilePath)
	}

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

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	if singleFileMode {
		dupCount := applySingleFileFilter(dMap, targetDigest, targetFilePath)
		if dupCount == 0 {
			dsklog.Dlogger.Infof("Single file mode complete: no duplicates found for %s", targetFilePath)
			pterm.Info.Printf("No duplicates of %s found in the provided paths.\n", targetFilePath)
			os.Exit(0)
		}
		dsklog.Dlogger.Infof("Single file mode complete: found %d duplicates for %s", dupCount, targetFilePath)
		pterm.Info.Printf("Found %d duplicate(s) of %s.\n", dupCount, targetFilePath)
	}

	// Status bar update
	finalInfo := "Total of " + pterm.LightWhite(nfiles) + " files processed in " +
		pterm.LightWhite(duration)
	pterm.Success.Println(finalInfo)

	// Dump to CSV, then exit without dropping into TUI
	if *flCSVOut != "" {
		dsklog.Dlogger.Infof("CSV export requested: %s", *flCSVOut)
		pterm.Info.Printf("Writing CSV to %s...\n", *flCSVOut)
		if err := dMap.WriteCSV(*flCSVOut); err != nil {
			dsklog.Dlogger.Errorf("CSV export failed for %s: %v", *flCSVOut, err)
			fmt.Fprintf(os.Stderr, "failed to write CSV output: %v\n", err)
			os.Exit(1)
		}
		dsklog.Dlogger.Infof("CSV export complete: %s", *flCSVOut)
		pterm.Success.Printf("CSV file %s written to disk.\n", *flCSVOut)
		os.Exit(0)
	}

	// Dump files to JSON then exit.
	if *flJSONOut != "" {
		dsklog.Dlogger.Infof("JSON export requested: %s", *flJSONOut)
		pterm.Info.Printf("Writing JSON to %s...\n", *flJSONOut)
		if err := dMap.WriteJSON(*flJSONOut); err != nil {
			dsklog.Dlogger.Errorf("JSON export failed for %s: %v", *flJSONOut, err)
			fmt.Fprintf(os.Stderr, "failed to write JSON output: %v\n", err)
			os.Exit(1)
		}
		dsklog.Dlogger.Infof("JSON export complete: %s", *flJSONOut)
		pterm.Success.Printf("JSON file %s written to disk.\n", *flJSONOut)
		os.Exit(0)
	}

	// Zero value for moveKeep means don't remove or relink anything.
	if keepCount > 0 {
		if *flLinkMode {
			linkedPaths, linkErr := dMap.LinkDuplicates(keepCount)
			fmt.Printf("Converted %d duplicate files to symlinks, kept %d real file(s) per group.\n", len(linkedPaths), keepCount)
			if linkErr != nil {
				fmt.Fprintf(os.Stderr, "Linking completed with errors: %v\n", linkErr)
				os.Exit(1)
			}
		} else {
			removedPaths, removeErr := dMap.RemoveDuplicates(keepCount)
			fmt.Printf("Removed %d duplicate files, kept %d per group.\n", len(removedPaths), keepCount)
			if removeErr != nil {
				fmt.Fprintf(os.Stderr, "Removal completed with errors: %v\n", removeErr)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}

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

// applySingleFileFilter filters the provided Dmap to retain only the entries matching
// the specified digest and prioritizes the given target path, returning the count of
// duplicates found for that target.
func applySingleFileFilter(dMap *dmap.Dmap, digest dmap.Digest, target string) int {
	if dMap == nil {
		return 0
	}
	filesMap := dMap.GetMap()
	if len(filesMap) == 0 {
		return 0
	}
	matches := append([]string(nil), filesMap[digest]...)
	dupCount := countDuplicates(matches, target)
	if dupCount == 0 {
		return 0
	}
	filesMap[digest] = ensurePathFirst(matches, target)
	for hash := range filesMap {
		if hash != digest {
			delete(filesMap, hash)
		}
	}
	return dupCount
}

// countDuplicates returns the number of entries in paths that do not match the target path.
func countDuplicates(paths []string, target string) int {
	if len(paths) == 0 {
		return 0
	}
	count := 0
	for _, p := range paths {
		if p == target {
			continue
		}
		count++
	}
	return count
}

// ensurePathFirst reorders the provided paths so that target appears first, inserting it if absent,
// or returning the original slice unchanged when target is empty or already leading.
func ensurePathFirst(paths []string, target string) []string {
	if target == "" {
		return paths
	}
	idx := -1
	for i, p := range paths {
		if p == target {
			idx = i
			break
		}
	}
	switch {
	case idx == 0:
		return paths
	case idx > 0:
		result := make([]string, 0, len(paths))
		result = append(result, target)
		result = append(result, paths[:idx]...)
		result = append(result, paths[idx+1:]...)
		return result
	default:
		result := make([]string, 0, len(paths)+1)
		result = append(result, target)
		result = append(result, paths...)
		return result
	}
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
