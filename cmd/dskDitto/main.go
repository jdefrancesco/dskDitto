package main

import (
	"context"
	"fmt"
	"math"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
	"github.com/jdefrancesco/dskDitto/internal/ui"
	"github.com/jdefrancesco/dskDitto/pkg/utils"

	"github.com/pterm/pterm"
)

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

	opts := parseOptions()

	if opts.gui {
		if err := validateGUIBuild(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	shallowMode := opts.nameOnly || opts.fileShallow != ""
	shallowTargetName, shallowErr := validateShallowMode(opts.nameOnly, opts.fileShallow, opts.singleFile, opts.backupFile)
	if shallowErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", shallowErr)
		os.Exit(1)
	}

	if opts.linkMode && opts.reflinkMode {
		fmt.Fprintf(os.Stderr, "invalid invocation: --link and --reflink cannot be combined\n")
		os.Exit(1)
	}

	fuzzyMode := opts.fuzzy
	if fuzzyErr := validateFuzzyMode(fuzzyMode, shallowMode, opts.singleFile, opts.backupFile, opts.restoreFile, opts.keep, opts.linkMode || opts.reflinkMode, opts.fuzzyThreshold); fuzzyErr != nil {
		fmt.Fprintf(os.Stderr, "invalid fuzzy invocation: %v\n", fuzzyErr)
		os.Exit(1)
	}

	var fuzzyMinFileSize int64
	if fuzzyMode {
		if opts.fuzzyMinSize != "" && opts.fuzzyMinSize != "0" {
			parsed, err := utils.ParseSize(opts.fuzzyMinSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --fuzzy-min-size value %q: %v\n", opts.fuzzyMinSize, err)
				os.Exit(1)
			}
			if parsed > uint64(math.MaxInt64) {
				fmt.Fprintf(os.Stderr, "--fuzzy-min-size %s exceeds supported file size limit (%d bytes)\n", opts.fuzzyMinSize, int64(math.MaxInt64))
				os.Exit(1)
			}
			fuzzyMinFileSize = int64(parsed) // #nosec G115 -- bounds checked above
		}
	}

	// Turn off default color scheme. This flag can be used when users terminal color pallete isn't
	// compatible with default TUI elements.
	if opts.colorSafe {
		ui.EnableSafeColors()
	}

	// Enable CPU profiling
	if opts.cpuProfile != "" {
		f, err := os.Create(opts.cpuProfile)
		if err != nil {
			dsklog.Dlogger.Info("profile failed")
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
	}

	if !opts.noBanner {
		showHeader(opts.colorSafe)
	}

	// Just show version then quit.
	if opts.showVersion {
		showVersion()
		os.Exit(0)
	}

	fmt.Printf("[!] Press CTRL+C to stop dskDitto at any time.\n")

	if opts.detectFS != "" {
		fs, err := dfs.DetectFilesystem(".")
		if err != nil {
			panic(err)
		}
		fmt.Printf("Filesystem: %s\n\n", fs)
	}

	if opts.restoreFile != "" {
		if err := validateRestoreMode(opts.restoreFile, opts.backupFile, opts.paths, opts.gui, opts.textOutput, opts.showBullets, opts.csvOut,
			opts.jsonOut, opts.singleFile, opts.fileShallow, opts.nameOnly, opts.keep, opts.linkMode || opts.reflinkMode); err != nil {
			fmt.Fprintf(os.Stderr, "invalid restore invocation: %v\n", err)
			os.Exit(1)
		}
		restoreOptions := manifest.RestoreOptions{
			DryRun:       opts.dryRun,
			Overwrite:    false,
			VerifyHash:   opts.verifyHash,
			RestoreMode:  true,
			RestoreMTime: true,
		}
		if err := manifest.RestoreManifest(opts.restoreFile, restoreOptions); err != nil {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Printf("Restore completed from manifest %s.\n", opts.restoreFile)
		os.Exit(0)
	}

	minFileSize := int64(0)

	if opts.minFileSize != "" {
		value, err := utils.ParseSize(opts.minFileSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value for --min-size: %v\n", err)
			os.Exit(1)
		}
		if value > uint64(math.MaxInt64) {
			fmt.Fprintf(os.Stderr, "--min-size %s exceeds supported file size limit (%d bytes)\n", opts.minFileSize, int64(math.MaxInt64))
			os.Exit(1)
		}

		minFileSize = int64(value) // #nosec G115 -- bounds checked above
		if minFileSize > 0 {
			fmt.Printf("Skipping files smaller than: ~ %s.\n", utils.DisplaySize(uint64(minFileSize)))
		}
		dsklog.Dlogger.Debugf("Min file size set to %d bytes.\n", minFileSize)
	}

	maxFileSize, err := resolveMaxFileSize(opts.allSizes, opts.maxFileSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if opts.allSizes || opts.maxFileSize != "" {
		if maxFileSize > 0 {
			fmt.Printf("Skipping files larger than: %s (%d bytes).\n", utils.DisplaySize(uint64(maxFileSize)), maxFileSize)
		} else {
			fmt.Printf("No maximum file size limit configured.\n")
		}
	}
	dsklog.Dlogger.Debugf("Max file size set to %d bytes.\n", maxFileSize)

	if opts.depth < -1 {
		fmt.Fprintf(os.Stderr, "invalid depth %d; must be -1 or greater\n", opts.depth)
		os.Exit(1)
	}

	maxDepth := -1
	if opts.depth >= 0 {
		maxDepth = opts.depth
	}

	// Don't recuse into any sub-directories
	if opts.noRecurse {
		maxDepth = 0
	}

	if maxDepth == 0 && (opts.noRecurse || opts.depth >= 0) {
		dsklog.Dlogger.Debug("Recursion disabled. Invoked with current flag. Only checking current directory for dups.")
	} else if maxDepth > 0 {
		dsklog.Dlogger.Debugf("Limiting recursion depth to %d level(s).\n", maxDepth)
	}

	hashAlgo, err := dfs.ParseHashAlgorithm(opts.hashAlgo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unsupported hash algorithm %q; must be 'sha256' or 'blake3'\n", opts.hashAlgo)
		os.Exit(1)
	}

	dsklog.Dlogger.Debugf("Using hash algorithm: %s", hashAlgo)
	hashOptions := dfs.HashOptions{NoCache: opts.noCache}

	var singleTarget *singleFileTarget
	if opts.singleFile != "" && !shallowMode {
		var prepErr error
		singleTarget, prepErr = prepareSingleFileTarget(opts.singleFile, hashAlgo, hashOptions)
		if prepErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", prepErr)
			os.Exit(1)
		}
		pterm.Info.Printf("Searching for duplicates of %s\n", singleTarget.filePath)
	}
	singleFileMode := singleTarget != nil
	if fuzzyMode {
		pterm.Info.Printf("Searching for near-duplicate file content (threshold >= %d%%)\n", opts.fuzzyThreshold)
		if opts.fuzzySameExt {
			pterm.Info.Println("Fuzzy mode extension filter enabled")
		}
		if fuzzyMinFileSize > 0 {
			pterm.Info.Printf("Fuzzy mode skipping files smaller than %s (use --fuzzy-min-size 0 to disable)\n", utils.DisplaySize(uint64(fuzzyMinFileSize)))
		}
	}
	if shallowMode && !fuzzyMode {
		if shallowTargetName != "" {
			pterm.Info.Printf("Searching for shallow duplicates named %s\n", shallowTargetName)
			if shallowTargetIsHidden(shallowTargetName) && !opts.includeHidden {
				pterm.Info.Println("Including hidden files and directories for hidden shallow target")
			}
		} else {
			pterm.Info.Println("Searching for shallow duplicates by exact file name")
		}
	}

	rootDirs := opts.paths
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Dmap stores duplicate file information. Failure is fatal.
	minDups := opts.minDups
	if minDups < 2 {
		pterm.Error.Printf("Duplicate threshold %d is invalid; --dups must be >= 2\n", minDups)
		os.Exit(1)
	}

	// If we remove a set of duplicates keep at least this amount.
	keepCount := opts.keep
	if keepCount != 0 {
		pterm.Info.Printf("Keep count set; will leave %d files at least\n", keepCount)
	}

	// Hold app config.
	appCfg := config.Config{
		SkipEmpty:      !opts.includeEmpty,
		SkipSymLinks:   opts.skipSymLinks,
		SkipHidden:     resolveSkipHidden(opts.includeHidden, shallowTargetName, opts.singleFile),
		SkipVirtualFS:  !opts.includeVFS,
		OneFileSystem:  opts.oneFileSystem || opts.xdev,
		ExcludePaths:   []string(opts.excludePaths),
		MaxDepth:       maxDepth,
		DirConcurrency: opts.dirConcurrency,
		NoCache:        opts.noCache,
		MinFileSize:    minFileSize,
		MaxFileSize:    maxFileSize,
		MinDuplicates:  minDups,
		HashAlgorithm:  hashAlgo,
	}

	dMap, err := dmap.NewDmap(appCfg.MinDuplicates)
	if err != nil {
		dsklog.Dlogger.Fatal("Failed to make new Dmap: ", err)
		os.Exit(1)
	}

	stats, err := scanFiles(ctx, dMap, scanSettings{
		options:           opts,
		config:            appCfg,
		roots:             rootDirs,
		singleTarget:      singleTarget,
		shallowTargetName: shallowTargetName,
		fuzzyMinFileSize:  fuzzyMinFileSize,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	showScanSummary(stats, fuzzyMode, shallowMode)

	if fuzzyMode {
		if dMap.IsEmpty() {
			pterm.Info.Println("No near-duplicate file-content matches found in the provided paths.")
			os.Exit(0)
		}
	} else if singleFileMode {
		dupCount := dMap.FilterToDigest(singleTarget.digest, singleTarget.filePath)
		if dupCount == 0 {
			if singleFileTargetIsHidden(singleTarget.filePath) {
				pterm.Info.Printf("No exact duplicates of %s found in the provided paths. Hidden file targets are matched by content; use --name-only or --file-shallow if you want same-name matching.\n", singleTarget.filePath)
			} else {
				pterm.Info.Printf("No duplicates of %s found in the provided paths.\n", singleTarget.filePath)
			}
			os.Exit(0)
		}
		pterm.Info.Printf("Found %d duplicate(s) of %s.\n", dupCount, singleTarget.filePath)
	}
	if !fuzzyMode && shallowTargetName != "" {
		files, _ := dMap.Get(dmap.NameDigest(shallowTargetName))
		if len(files) == 0 {
			pterm.Info.Printf("No shallow duplicates named %s found in the provided paths.\n", shallowTargetName)
			os.Exit(0)
		}
		pterm.Info.Printf("Found %d file(s) named %s.\n", len(files), shallowTargetName)
	}

	if err := processResults(dMap, opts, hashAlgo); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
