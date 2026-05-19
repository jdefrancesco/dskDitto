package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/buildinfo"
	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
	"github.com/jdefrancesco/dskDitto/internal/rayui"
	"github.com/jdefrancesco/dskDitto/internal/ui"
	"github.com/jdefrancesco/dskDitto/pkg/utils"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

func init() {

	// Custom help message
	flag.Usage = func() {
		showHeader(false)
		fmt.Fprintf(os.Stderr, "Usage: dskDitto [options] PATHS\n\n")
		printFlagHelpTable()
		fmt.Fprintf(os.Stderr, "Notes:\n")
		fmt.Fprintf(os.Stderr, "  Display-oriented options like --bullet only render results; no files are removed.\n")
	}
}

type stringListFlag []string

func printFlagHelpTable() {
	type helpRow struct {
		option string
		usage  string
	}

	rows := make([]helpRow, 0, 24)
	maxOptionWidth := 0
	flag.VisitAll(func(fl *flag.Flag) {
		argName, usage := flag.UnquoteUsage(fl)
		option := "--" + fl.Name
		if argName != "" {
			option += " <" + argName + ">"
		}
		if len(option) > maxOptionWidth {
			maxOptionWidth = len(option)
		}
		rows = append(rows, helpRow{
			option: option,
			usage:  usage,
		})
	})

	fmt.Fprintf(os.Stderr, "Options:\n")
	for _, row := range rows {
		fmt.Fprintf(os.Stderr, "  %-*s  %s\n", maxOptionWidth, row.option, row.usage)
	}
	fmt.Fprintln(os.Stderr)
}

func (s *stringListFlag) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringListFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
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
		flNoBanner       = flag.Bool("no-banner", false, "Do not show the dskDitto banner.")
		flShowVersion    = flag.Bool("version", false, "Display version")
		flCpuProfile     = flag.String("profile", "", "Write CPU profile to `file` for analysis.")
		flTimeOnly       = flag.Bool("time-only", false, "Use to show only the time taken to scan directory for duplicates.")
		flMinFileSize    = flag.String("min-size", "", "Skip files smaller than this `size` (supports suffixes like 512K, 5MiB).")
		flMaxFileSize    = flag.String("max-size", "", "Skip files larger than this `size` (default 4GiB).")
		flTextOutput     = flag.Bool("text", false, "Dump results in grep/text friendly format. Useful for scripting.")
		flShowBullets    = flag.Bool("bullet", false, "Show duplicates as formatted bullet list.")
		flIncludeEmpty   = flag.Bool("empty", false, "Include empty files (0 bytes).")
		flSkipSymLinks   = flag.Bool("no-symlinks", true, "Skip symbolic links. This is on by default.")
		flIncludeHidden  = flag.Bool("hidden", false, "Include hidden files and directories (dotfiles).")
		flExcludePaths   stringListFlag
		flNoRecurse      = flag.Bool("current", false, "Only scan the provided directories without descending into subdirectories.")
		flDepth          = flag.Int("depth", -1, "Maximum recursion `levels`; 0 inspects only the provided paths, -1 means unlimited.")
		flIncludeVFS     = flag.Bool("include-vfs", false, "Include virtual filesystem mount points such as /proc and /dev.")
		flDirConcurrency = flag.Int("dir-concurrency", 0, "Limit concurrent directory reads; <= 0 uses automatic tuning.")
		flNoCache        = flag.Bool("no-cache", false, "Ask supported platforms not to populate filesystem cache while hashing.")
		flMinDups        = flag.Uint("dups", 2, "Minimum duplicate file `count` required to display a group.")
		flHashAlgo       = flag.String("hash", "sha256", "Hash algorithm `algo`: sha256 (default) or blake3.")
		flKeep           = flag.Uint("remove", 0, "Operate on duplicates, keeping only this many `keep` files per group.")
		flLinkMode       = flag.Bool("link", false, "Convert extra duplicates into symlinks instead of deleting them (use with --remove).")
		flSingleFile     = flag.String("file", "", "Only search for duplicates of the specified `path` file.")
		flNameOnly       = flag.Bool("name-only", false, "Only compare exact file names, ignoring content and size.")
		flFileShallow    = flag.String("file-shallow", "", "Only search for files with the same exact name as the specified `path` file.")
		flCSVOut         = flag.String("csv-out", "", "Write duplicate groups to the specified CSV `file`.")
		flJSONOut        = flag.String("json-out", "", "Write duplicate groups to the specified JSON `file`.")
		flDetectFS       = flag.String("fs-detect", "", "Detect filesystem in use by specified `path`.")
		flColorSafe      = flag.Bool("color-safe", false, "Use a conservative ANSI-safe color palette for the TUI (for terminals with problematic color rendering).")
		flGui            = flag.Bool("gui", false, "Show results in an interactive raylib GUI")
		flNoConfirm      = flag.Bool("no-confirm", false, "Do not ask for confirmation codes before interactive delete/link actions.")
		flBackupFile     = flag.String("backup", "", "Write duplicate restore backup JSONL to the specified `file`.")
		flRestoreFile    = flag.String("restore", "", "Restore duplicate files from the specified JSONL `file`.")
		flDryRun         = flag.Bool("dry-run", false, "With --restore, print actions without writing files.")
		flVerifyHash     = flag.Bool("verify-hash", true, "With --restore, verify canonical file hashes before replay.")
	)
	// The exclude flag can take multiple path targets
	flag.Var(&flExcludePaths, "exclude", "Exclude a `path` from scanning (repeatable).")
	flag.Parse()

	shallowMode := *flNameOnly || *flFileShallow != ""
	shallowTargetName, shallowErr := validateShallowMode(*flNameOnly, *flFileShallow, *flSingleFile, *flBackupFile)
	if shallowErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", shallowErr)
		os.Exit(1)
	}

	// Turn off default color scheme. This flag can be used when users terminal color pallete isn't
	// compatible with default TUI elements.
	if *flColorSafe {
		ui.EnableSafeColors()
	}

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
		showHeader(*flColorSafe)
	}

	// Just show version then quit.
	if *flShowVersion {
		showVersion()
		os.Exit(0)
	}

	fmt.Printf("[!] Press CTRL+C to stop dskDitto at any time.\n")

	if *flDetectFS != "" {
		fs, err := dfs.DetectFilesystem(".")
		if err != nil {
			panic(err)
		}
		fmt.Printf("Filesystem: %s\n\n", fs)
	}

	if *flRestoreFile != "" {
		if err := validateRestoreMode(*flRestoreFile, *flBackupFile, flag.Args(), *flGui, *flTextOutput, *flShowBullets, *flCSVOut, *flJSONOut, *flSingleFile, *flFileShallow, *flNameOnly, *flKeep, *flLinkMode); err != nil {
			fmt.Fprintf(os.Stderr, "invalid restore invocation: %v\n", err)
			os.Exit(1)
		}
		restoreOptions := manifest.RestoreOptions{
			DryRun:       *flDryRun,
			Overwrite:    false,
			VerifyHash:   *flVerifyHash,
			RestoreMode:  true,
			RestoreMTime: true,
		}
		if err := manifest.RestoreManifest(*flRestoreFile, restoreOptions); err != nil {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Printf("Restore completed from manifest %s.\n", *flRestoreFile)
		os.Exit(0)
	}

	// Maximum uint size.
	maxUint := ^uint(0)
	MinFileSize := uint(0)

	// XXX: NOTE: This logic started to get a little messy. I need to refactor several blocks. If anyone cares to refactor
	//            things please do!
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
		fmt.Fprintf(os.Stderr, "invalid depth %d; must be -1 or greater\n", *flDepth)
		os.Exit(1)
	}

	maxDepth := -1
	if *flDepth >= 0 {
		maxDepth = *flDepth
	}

	// Don't recuse into any sub-directories
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
	hashOptions := dfs.HashOptions{NoCache: *flNoCache}

	// TODO: Refactor user messages and logging.
	singleFileMode := false
	var targetDigest dmap.Digest
	var targetFilePath string
	var targetFileSize int64
	var targetSampleDigest dmap.Digest
	if *flSingleFile != "" && !shallowMode {
		info, statErr := os.Stat(*flSingleFile)
		if statErr != nil {
			dsklog.Dlogger.Debugf("Unable to stat --file path %s: %v\n", *flSingleFile, statErr)
			os.Exit(1)
		}
		if !info.Mode().IsRegular() {
			dsklog.Dlogger.Debugf("filepath needs to be a regular file: %s\n", *flSingleFile)
			os.Exit(1)
		}
		targetDfile, hashErr := dfs.NewDfileWithOptions(*flSingleFile, info.Size(), hashAlgo, hashOptions)
		if hashErr != nil {
			dsklog.Dlogger.Debugf("Failed to hash --file target %s: %v\n", *flSingleFile, hashErr)
			os.Exit(1)
		}
		targetDigest = dmap.Digest(targetDfile.Hash())
		targetFilePath = targetDfile.FileName()
		targetFileSize = info.Size()
		targetSample, sampleErr := dfs.HashFileSampleWithOptions(targetFilePath, targetFileSize, hashAlgo, hashOptions)
		if sampleErr != nil {
			dsklog.Dlogger.Debugf("Failed to sample --file target %s: %v\n", *flSingleFile, sampleErr)
			os.Exit(1)
		}
		targetSampleDigest = dmap.Digest(targetSample.Digest)
		singleFileMode = true

		pterm.Info.Printf("Searching for duplicates of %s\n", targetFilePath)
	}
	if shallowMode {
		if shallowTargetName != "" {
			pterm.Info.Printf("Searching for shallow duplicates named %s\n", shallowTargetName)
			if shallowTargetIsHidden(shallowTargetName) && !*flIncludeHidden {
				pterm.Info.Println("Including hidden files and directories for hidden shallow target")
			}
		} else {
			pterm.Info.Println("Searching for shallow duplicates by exact file name")
		}
	}

	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Dmap stores duplicate file information. Failure is fatal.
	minDups := *flMinDups
	if minDups < 2 {
		pterm.Info.Printf("Duplicate threshold %d. Mu400Gst be >= 2", minDups)
		os.Exit(1)
	}

	// If we remove a set of duplicates keep at least this amount.
	keepCount := *flKeep
	if keepCount != 0 {
		pterm.Info.Printf("Keep count set; will leave %d files at least\n", keepCount)
	}

	// Hold app config.
	appCfg := config.Config{
		SkipEmpty:      !*flIncludeEmpty,
		SkipSymLinks:   *flSkipSymLinks,
		SkipHidden:     resolveSkipHidden(*flIncludeHidden, shallowTargetName),
		SkipVirtualFS:  !*flIncludeVFS,
		ExcludePaths:   []string(flExcludePaths),
		MaxDepth:       maxDepth,
		DirConcurrency: *flDirConcurrency,
		NoCache:        *flNoCache,
		MinFileSize:    MinFileSize,
		MaxFileSize:    MaxFileSize,
		MinDuplicates:  minDups,
		HashAlgorithm:  hashAlgo,
	}

	dMap, err := dmap.NewDmap(appCfg.MinDuplicates)
	if err != nil {
		dsklog.Dlogger.Fatal("Failed to make new Dmap: ", err)
		os.Exit(1)
	}

	start := time.Now()

	// Tried some weird stuff with progress timing
	showProgress := true
	var tickC <-chan time.Time
	updateProgress := func(string) {}
	stopProgress := func() {}
	if showProgress {
		tick := time.NewTicker(time.Duration(500) * time.Millisecond)
		defer tick.Stop()
		tickC = tick.C
		infoSpinner, _ := pterm.DefaultSpinner.Start()
		updateProgress = func(msg string) {
			infoSpinner.UpdateText(msg)
		}
		stopProgress = func() {
			infoSpinner.Stop()
		}
	}

	// First collect cheap file metadata. Hashing waits until after this pass so
	// unique file sizes never touch the expensive content path.
	candidateFiles := make(chan dwalk.FileCandidate, 4096)
	walker := dwalk.NewCandidateWalker(rootDirs, candidateFiles, appCfg)
	walker.Run(ctx)

	sizeGroups := make(map[int64][]dwalk.FileCandidate, 4096)
	nameGroups := make(map[string][]dwalk.FileCandidate, 4096)
	var scannedFiles uint

CollectLoop:
	for {
		select {
		case <-ctx.Done():
			for range candidateFiles {
			}
			break CollectLoop

		case candidate, ok := <-candidateFiles:
			if !ok {
				break CollectLoop
			}
			scannedFiles++
			if shallowMode {
				name := filepath.Base(candidate.Path)
				if shallowTargetName != "" && name != shallowTargetName {
					continue
				}
				nameGroups[name] = append(nameGroups[name], candidate)
				continue
			}
			if singleFileMode && candidate.Size != targetFileSize {
				continue
			}
			sizeGroups[candidate.Size] = append(sizeGroups[candidate.Size], candidate)

		case <-tickC:
			progressMsg := fmt.Sprintf("Scanned %d files...", scannedFiles)
			updateProgress(progressMsg)
		}
	}

	var sampledFiles uint
	var fullHashedFiles uint

	if shallowMode {
		addedGroups, skippedByName := addNameOnlyGroups(dMap, nameGroups, minDups)
		nameGroups = nil
		dsklog.Dlogger.Debugf("Added %d shallow filename groups; skipped %d files with unique names", addedGroups, skippedByName)
	} else {
		sampleList, skippedBySize := eligibleHashCandidates(sizeGroups, minDups, singleFileMode)
		sizeGroups = nil
		dsklog.Dlogger.Debugf("Skipped %d files with unique sizes before sample hashing", skippedBySize)

		sampleGroups := make(map[sampleKey][]sampledFile, 4096)

		if len(sampleList) > 0 {
			sampleJobs := make(chan dwalk.FileCandidate, min(len(sampleList), 4096))
			sampledFileCh := make(chan sampledFile, min(len(sampleList), 4096))
			workerCount := sampleWorkerCount(len(sampleList))

			var sampleWG sync.WaitGroup
			sampleWG.Add(workerCount)
			for i := 0; i < workerCount; i++ {
				go func() {
					defer sampleWG.Done()
					for candidate := range sampleJobs {
						sample, sampleErr := dfs.HashFileSampleWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
						if sampleErr != nil {
							dsklog.Dlogger.Debugf("Skipping file after sample failure %s: %v", candidate.Path, sampleErr)
							continue
						}
						select {
						case <-ctx.Done():
							return
						case sampledFileCh <- sampledFile{
							candidate:       candidate,
							digest:          dmap.Digest(sample.Digest),
							coversWholeFile: sample.CoversWholeFile,
						}:
						}
					}
				}()
			}

			go func() {
				defer close(sampleJobs)
				for _, candidate := range sampleList {
					select {
					case <-ctx.Done():
						return
					case sampleJobs <- candidate:
					}
				}
			}()

			go func() {
				sampleWG.Wait()
				close(sampledFileCh)
			}()

		SampleLoop:
			for {
				select {
				case sample, ok := <-sampledFileCh:
					if !ok {
						break SampleLoop
					}
					sampledFiles++
					if singleFileMode && sample.digest != targetSampleDigest {
						continue
					}
					key := sampleKey{size: sample.candidate.Size, digest: sample.digest}
					sampleGroups[key] = append(sampleGroups[key], sample)

				case <-tickC:
					progressMsg := fmt.Sprintf("Sampled %d/%d candidate files...", sampledFiles, len(sampleList))
					updateProgress(progressMsg)
				}
			}
		}

		directFiles, fullHashList, skippedBySample := eligibleSampleCandidates(sampleGroups, minDups, singleFileMode)
		sampleGroups = nil
		dsklog.Dlogger.Debugf("Skipped %d files with unique samples before full hashing", skippedBySample)

		for _, file := range directFiles {
			dMap.AddPath(file.digest, file.candidate.Path)
		}

		if len(fullHashList) > 0 {
			hashJobs := make(chan dwalk.FileCandidate, min(len(fullHashList), 4096))
			hashedFiles := make(chan *dfs.Dfile, min(len(fullHashList), 4096))
			workerCount := hashWorkerCount(len(fullHashList))

			var hashWG sync.WaitGroup
			hashWG.Add(workerCount)
			for i := 0; i < workerCount; i++ {
				go func() {
					defer hashWG.Done()
					for candidate := range hashJobs {
						dFile, hashErr := dfs.NewDfileWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
						if hashErr != nil {
							dsklog.Dlogger.Debugf("Skipping file after hash failure %s: %v", candidate.Path, hashErr)
							continue
						}
						select {
						case <-ctx.Done():
							return
						case hashedFiles <- dFile:
						}
					}
				}()
			}

			go func() {
				defer close(hashJobs)
				for _, candidate := range fullHashList {
					select {
					case <-ctx.Done():
						return
					case hashJobs <- candidate:
					}
				}
			}()

			go func() {
				hashWG.Wait()
				close(hashedFiles)
			}()

		HashLoop:
			for {
				select {
				case dFile, ok := <-hashedFiles:
					if !ok {
						break HashLoop
					}
					if dFile == nil {
						dsklog.Dlogger.Warn("Received nil dFile, skipping...")
						continue
					}
					dMap.Add(dFile)
					fullHashedFiles++

				case <-tickC:
					progressMsg := fmt.Sprintf("Hashed %d/%d full candidate files...", fullHashedFiles, len(fullHashList))
					updateProgress(progressMsg)
				}
			}
		}
	}

	stopProgress()
	duration := time.Since(start)

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	if singleFileMode {
		dupCount := applySingleFileFilter(dMap, targetDigest, targetFilePath)
		if dupCount == 0 {
			pterm.Info.Printf("No duplicates of %s found in the provided paths.\n", targetFilePath)
			os.Exit(0)
		}
		pterm.Info.Printf("Found %d duplicate(s) of %s.\n", dupCount, targetFilePath)
	}
	if shallowTargetName != "" {
		files, _ := dMap.Get(dmap.NameDigest(shallowTargetName))
		if len(files) == 0 {
			pterm.Info.Printf("No shallow duplicates named %s found in the provided paths.\n", shallowTargetName)
			os.Exit(0)
		}
		pterm.Info.Printf("Found %d file(s) named %s.\n", len(files), shallowTargetName)
	}

	// Status bar update
	finalInfo := "Scanned " + pterm.LightWhite(scannedFiles) + " files, sampled " +
		pterm.LightWhite(sampledFiles) + " candidates, fully hashed " +
		pterm.LightWhite(fullHashedFiles) + " in " + pterm.LightWhite(duration)
	if shallowMode {
		finalInfo = "Scanned " + pterm.LightWhite(scannedFiles) + " files by name in " + pterm.LightWhite(duration)
	}
	pterm.Success.Println(finalInfo)

	interactiveMode := keepCount == 0 && !*flTimeOnly && !*flTextOutput && !*flShowBullets && *flCSVOut == "" && *flJSONOut == ""
	if *flBackupFile != "" && !interactiveMode {
		entries, manifestErr := manifest.EntriesFromDmap(dMap, hashAlgo)
		if manifestErr != nil {
			fmt.Fprintf(os.Stderr, "failed to build restore manifest: %v\n", manifestErr)
			os.Exit(1)
		}
		if manifestErr := manifest.Write(*flBackupFile, entries); manifestErr != nil {
			fmt.Fprintf(os.Stderr, "failed to write restore manifest: %v\n", manifestErr)
			os.Exit(1)
		}
		pterm.Success.Printf("Restore backup file with %d entries written to %s.\n", len(entries), *flBackupFile)
	}

	// Dump to CSV, then exit without dropping into TUI
	if *flCSVOut != "" {
		pterm.Info.Printf("Writing CSV to %s...\n", *flCSVOut)
		if err := dMap.WriteCSV(*flCSVOut); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write CSV output: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Printf("CSV file %s written to disk.\n", *flCSVOut)
		os.Exit(0)
	}

	// Dump files to JSON then exit.
	if *flJSONOut != "" {
		pterm.Info.Printf("Writing JSON to %s...\n", *flJSONOut)
		if err := dMap.WriteJSON(*flJSONOut); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write JSON output: %v\n", err)
			os.Exit(1)
		}
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

	// Dump text formats. This assumes you don't wish to use the interactive UIs
	switch {
	case *flTextOutput:
		dMap.PrintDmap()
		os.Exit(0)
	case *flShowBullets:
		dMap.ShowResultsBullet()
		os.Exit(0)
	}

	// Can now use a Raylib GUI or the sleeker TUI
	applyOptions := dupview.ApplyOptions{
		BackupPath:    *flBackupFile,
		HashAlgorithm: hashAlgo,
		SkipConfirm:   *flNoConfirm,
	}
	if *flGui {
		rayui.Launch(dMap, applyOptions)
	} else {
		ui.LaunchTUI(dMap, applyOptions)
	}
}

func eligibleHashCandidates(sizeGroups map[int64][]dwalk.FileCandidate, minDups uint, singleFileMode bool) ([]dwalk.FileCandidate, uint) {
	if minDups < 2 {
		minDups = 2
	}

	total := 0
	for _, files := range sizeGroups {
		if singleFileMode || uint(len(files)) >= minDups {
			total += len(files)
		}
	}

	candidates := make([]dwalk.FileCandidate, 0, total)
	var skipped uint
	for _, files := range sizeGroups {
		if singleFileMode || uint(len(files)) >= minDups {
			candidates = append(candidates, files...)
			continue
		}
		skipped += uint(len(files))
	}

	return candidates, skipped
}

func addNameOnlyGroups(dMap *dmap.Dmap, nameGroups map[string][]dwalk.FileCandidate, minDups uint) (uint, uint) {
	if dMap == nil {
		return 0, 0
	}
	if minDups < 2 {
		minDups = 2
	}

	names := make([]string, 0, len(nameGroups))
	for name := range nameGroups {
		names = append(names, name)
	}
	sort.Strings(names)

	var addedGroups uint
	var skipped uint
	for _, name := range names {
		files := nameGroups[name]
		if uint(len(files)) < minDups {
			skipped += uint(len(files))
			continue
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
		for _, file := range files {
			dMap.AddNamePath(name, file.Path)
		}
		addedGroups++
	}
	return addedGroups, skipped
}

func resolveSkipHidden(includeHidden bool, shallowTargetName string) bool {
	if includeHidden || shallowTargetIsHidden(shallowTargetName) {
		return false
	}
	return true
}

func shallowTargetIsHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

type sampleKey struct {
	size   int64
	digest dmap.Digest
}

type sampledFile struct {
	candidate       dwalk.FileCandidate
	digest          dmap.Digest
	coversWholeFile bool
}

func eligibleSampleCandidates(sampleGroups map[sampleKey][]sampledFile, minDups uint, singleFileMode bool) ([]sampledFile, []dwalk.FileCandidate, uint) {
	if minDups < 2 {
		minDups = 2
	}

	var directFiles []sampledFile
	var fullHashList []dwalk.FileCandidate
	var skipped uint

	for _, files := range sampleGroups {
		if len(files) == 0 {
			continue
		}
		if !singleFileMode && uint(len(files)) < minDups {
			skipped += uint(len(files))
			continue
		}
		if files[0].coversWholeFile {
			directFiles = append(directFiles, files...)
			continue
		}
		for _, file := range files {
			fullHashList = append(fullHashList, file.candidate)
		}
	}

	return directFiles, fullHashList, skipped
}

func sampleWorkerCount(total int) int {
	return boundedWorkerCount(total, 8)
}

func hashWorkerCount(total int) int {
	return boundedWorkerCount(total, 8)
}

func boundedWorkerCount(total int, procMultiplier int) int {
	if total <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0) * procMultiplier
	if workers < 1 {
		workers = 1
	}
	workers = min(workers, 128)
	return min(workers, total)
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

func validateShallowMode(nameOnly bool, fileShallow, singleFile, backupFile string) (string, error) {
	shallowMode := nameOnly || fileShallow != ""
	if !shallowMode {
		return "", nil
	}
	if singleFile != "" && fileShallow != "" {
		return "", fmt.Errorf("--file cannot be combined with --file-shallow")
	}
	if backupFile != "" {
		return "", fmt.Errorf("restore backups are not supported for shallow/name-only finds; rerun without --backup")
	}
	if singleFile != "" {
		return shallowFileName(singleFile, "--file")
	}
	if fileShallow == "" {
		return "", nil
	}
	return shallowFileName(fileShallow, "--file-shallow")
}

func shallowFileName(path, flagName string) (string, error) {
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(os.PathSeparator) {
		return "", fmt.Errorf("%s must name a file path", flagName)
	}
	return name, nil
}

func validateRestoreMode(restoreManifest, backupFile string, args []string, gui, textOutput, bulletOutput bool, csvOut, jsonOut, singleFile, fileShallow string, nameOnly bool, keep uint, linkMode bool) error {
	if restoreManifest == "" {
		return fmt.Errorf("--restore path must not be empty")
	}
	if backupFile != "" {
		return fmt.Errorf("--restore cannot be combined with --backup")
	}
	if len(args) > 0 {
		return fmt.Errorf("path arguments are not allowed with --restore")
	}
	if gui || textOutput || bulletOutput || csvOut != "" || jsonOut != "" || keep > 0 || linkMode || singleFile != "" || fileShallow != "" || nameOnly {
		return fmt.Errorf("--restore cannot be combined with scan/output/mutation flags")
	}
	return nil
}

// showHeader prints dskDitto banner.
// When safe is true, it avoids explicit colors to maximize contrast across themes.
func showHeader(safe bool) {

	fmt.Println("")

	leftStyle := pterm.NewStyle(pterm.FgLightGreen)
	rightStyle := pterm.NewStyle(pterm.FgLightWhite)
	if safe {
		leftStyle = pterm.NewStyle()
		rightStyle = pterm.NewStyle()
	}

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", leftStyle),
		putils.LettersFromStringWithStyle("Ditto", rightStyle),
	).Render()
}

func showVersion() {
	fmt.Printf("Version: %s\n", buildinfo.Version)
	fmt.Printf("Github: https://github.com/jdefrancesco/dskDitto\n")
	// Get rid of pesky percent sign some shells show if new line isn't printed correctly.
	fmt.Println("")
}
