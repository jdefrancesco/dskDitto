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
	"github.com/jdefrancesco/dskDitto/internal/fuzzy"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
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
	}
}

// Worker-pool tuning constants.
// These multipliers scale against GOMAXPROCS; the resulting count is always
// capped by utils.MaxWorkerCount (128) to prevent excessive goroutines and
// I/O contention on high-core or spinning-disk systems.
const (
	// hashWorkerMultiplier controls full-content hashing workers.
	// I/O-bound work benefits from more parallelism than pure CPU work.
	hashWorkerMultiplier = 4
	// sampleWorkerMultiplier controls sample/partial hashing workers.
	// Sample reads are smaller and faster, so the same multiplier as full
	// hashing provides adequate throughput without over-committing I/O.
	sampleWorkerMultiplier = 4
)

// hashWorkerCount returns the number of goroutines to use for full-file hashing.
func hashWorkerCount(total int) int {
	return utils.BoundedWorkerCount(total, hashWorkerMultiplier)
}

// sampleWorkerCount returns the number of goroutines to use for sample hashing.
func sampleWorkerCount(total int) int {
	return utils.BoundedWorkerCount(total, sampleWorkerMultiplier)
}

type stringListFlag []string

// flagCategory groups related flags together in --help output.
type flagCategory string

const (
	catGeneral flagCategory = "General"
	catFilter  flagCategory = "Filtering & Scanning"
	catScope   flagCategory = "Search Scope"
	catFuzzy   flagCategory = "Fuzzy Matching"
	catActions flagCategory = "Duplicate Actions"
	catOutput  flagCategory = "Output & Export"
	catRestore flagCategory = "Backup & Restore"
)

// flagCategoryOrder controls the section order --help prints flags in.
var flagCategoryOrder = []flagCategory{
	catGeneral, catFilter, catScope, catFuzzy, catActions, catOutput, catRestore,
}

// flagMeta records a single registered flag (and its optional shorthand) for
// the custom categorized --help renderer below; it stands in for flag.VisitAll,
// which only knows about individual *flag.Flag entries and can't tell us that
// two of them (e.g. "r" and "remove") are really one logical option.
type flagMeta struct {
	short    string
	long     string
	argName  string
	usage    string
	category flagCategory
}

var flagRegistry []flagMeta

// registerFlag records long (and, if non-empty, short) as one logical flag
// for --help purposes. It must run after both names are registered with the
// flag package so flag.UnquoteUsage can recover the `arg` placeholder.
func registerFlag(short, long string, category flagCategory) {
	f := flag.Lookup(long)
	if f == nil {
		panic("registerFlag: unknown flag --" + long)
	}
	argName, usage := flag.UnquoteUsage(f)
	flagRegistry = append(flagRegistry, flagMeta{
		short:    short,
		long:     long,
		argName:  argName,
		usage:    usage,
		category: category,
	})
}

// boolFlag/stringFlag/uintFlag/intFlag register a flag under its long name
// and, if short is non-empty, an additional single-letter alias that writes
// into the same variable, then record both under one entry for --help.
func boolFlag(long, short string, def bool, usage string, category flagCategory) *bool {
	p := new(bool)
	flag.BoolVar(p, long, def, usage)
	if short != "" {
		flag.BoolVar(p, short, def, usage)
	}
	registerFlag(short, long, category)
	return p
}

func stringFlag(long, short, def, usage string, category flagCategory) *string {
	p := new(string)
	flag.StringVar(p, long, def, usage)
	if short != "" {
		flag.StringVar(p, short, def, usage)
	}
	registerFlag(short, long, category)
	return p
}

func uintFlag(long, short string, def uint, usage string, category flagCategory) *uint {
	p := new(uint)
	flag.UintVar(p, long, def, usage)
	if short != "" {
		flag.UintVar(p, short, def, usage)
	}
	registerFlag(short, long, category)
	return p
}

func intFlag(long, short string, def int, usage string, category flagCategory) *int {
	p := new(int)
	flag.IntVar(p, long, def, usage)
	if short != "" {
		flag.IntVar(p, short, def, usage)
	}
	registerFlag(short, long, category)
	return p
}

// helpColorEnabled reports whether --help output should use pterm styling.
// It scans the raw args directly (rather than a parsed flag) so the decision
// doesn't depend on --color-safe appearing before --help on the command line.
// pterm still auto-strips ANSI codes on non-TTY output or when NO_COLOR is set.
func helpColorEnabled() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--color-safe" {
			return false
		}
	}
	return true
}

func formatFlagOption(m flagMeta) string {
	var option string
	if m.short != "" {
		option = fmt.Sprintf("-%s, --%s", m.short, m.long)
	} else {
		option = "    --" + m.long
	}
	if m.argName != "" {
		option += " <" + m.argName + ">"
	}
	return option
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// printFlagHelpTable renders every registered flag grouped by category, with
// short/long forms combined onto one line (e.g. "-r, --remove <keep>").
func printFlagHelpTable() {
	headerStyle := pterm.NewStyle(pterm.FgLightCyan, pterm.Bold)
	flagStyle := pterm.NewStyle(pterm.FgLightGreen, pterm.Bold)
	usageStyle := pterm.NewStyle(pterm.FgGray)
	if !helpColorEnabled() {
		headerStyle, flagStyle, usageStyle = pterm.NewStyle(), pterm.NewStyle(), pterm.NewStyle()
	}

	rowsByCategory := make(map[flagCategory][]flagMeta, len(flagCategoryOrder))
	maxOptionWidth := 0
	for _, m := range flagRegistry {
		rowsByCategory[m.category] = append(rowsByCategory[m.category], m)
		if w := len(formatFlagOption(m)); w > maxOptionWidth {
			maxOptionWidth = w
		}
	}

	for _, cat := range flagCategoryOrder {
		rows := rowsByCategory[cat]
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(os.Stderr, "\n  %s\n", headerStyle.Sprint(string(cat)))
		for _, m := range rows {
			option := padRight(formatFlagOption(m), maxOptionWidth)
			fmt.Fprintf(os.Stderr, "    %s  %s\n", flagStyle.Sprint(option), usageStyle.Sprint(m.usage))
		}
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

	// Parse command flags. Most common flags also have a single-letter shorthand
	// (e.g. -r is --remove); rarer/advanced flags are long-form only. See
	// printFlagHelpTable for how these are grouped and rendered in --help.
	var (
		// General
		flNoBanner    = boolFlag("no-banner", "", false, "Do not show the dskDitto banner.", catGeneral)
		flShowVersion = boolFlag("version", "v", false, "Display version and exit.", catGeneral)
		flCpuProfile  = stringFlag("profile", "", "", "Write CPU profile to `file` for analysis.", catGeneral)
		flTimeOnly    = boolFlag("time-only", "", false, "Use to show only the time taken to scan directory for duplicates.", catGeneral)
		flGui         = boolFlag("gui", "g", false, "Review results in an interactive raylib GUI (requires a GUI build).", catGeneral)
		flColorSafe   = boolFlag("color-safe", "", false, "Use a conservative ANSI-safe color palette for the TUI/help output (for terminals with problematic color rendering).", catGeneral)
		flNoConfirm   = boolFlag("no-confirm", "y", false, "Do not ask for confirmation codes before interactive delete/link/reflink actions.", catGeneral)

		// Filtering & Scanning
		flMinFileSize    = stringFlag("min-size", "", "", "Skip files smaller than this `size` (supports suffixes like 512K, 5MiB).", catFilter)
		flMaxFileSize    = stringFlag("max-size", "", "", "Skip files larger than this `size` (default 4GiB, 0 disables).", catFilter)
		flAllSizes       = boolFlag("all-sizes", "", false, "Scan files of any size; disables the default 4GiB maximum.", catFilter)
		flIncludeEmpty   = boolFlag("empty", "", false, "Include empty files (0 bytes).", catFilter)
		flSkipSymLinks   = boolFlag("no-symlinks", "", true, "Skip symbolic links. This is on by default.", catFilter)
		flIncludeHidden  = boolFlag("hidden", "", false, "Include hidden files and directories (dotfiles).", catFilter)
		flExcludePaths   stringListFlag
		flNoRecurse      = boolFlag("current", "", false, "Only scan the provided directories without descending into subdirectories.", catFilter)
		flDepth          = intFlag("depth", "d", -1, "Maximum recursion `levels`; 0 inspects only the provided paths, -1 means unlimited.", catFilter)
		flIncludeVFS     = boolFlag("include-vfs", "", false, "Include virtual filesystem mount points such as /proc and /dev.", catFilter)
		flOneFileSystem  = boolFlag("one-file-system", "", false, "Do not descend into directories on a different filesystem device.", catFilter)
		flXdev           = boolFlag("xdev", "", false, "Alias for --one-file-system.", catFilter)
		flDirConcurrency = intFlag("dir-concurrency", "", 0, "Limit concurrent directory reads; <= 0 uses automatic tuning.", catFilter)
		flNoCache        = boolFlag("no-cache", "", false, "Ask supported platforms not to populate filesystem cache while hashing.", catFilter)
		flMinDups        = uintFlag("dups", "", 2, "Minimum duplicate file `count` required to display a group.", catFilter)
		flHashAlgo       = stringFlag("hash", "H", "sha256", "Hash algorithm `algo`: sha256 (default) or blake3.", catFilter)

		// Search Scope
		flSingleFile  = stringFlag("file", "f", "", "Only search for duplicates of the specified `path` file.", catScope)
		flNameOnly    = boolFlag("name-only", "", false, "Only compare exact file names, ignoring content and size.", catScope)
		flFileShallow = stringFlag("file-shallow", "", "", "Only search for files with the same exact name as the specified `path` file.", catScope)

		// Fuzzy Matching
		flFuzzy              = boolFlag("fuzzy", "F", false, "Enable fuzzy content matching to find near-duplicate files.", catFuzzy)
		flFuzzyThreshold     = intFlag("fuzzy-threshold", "", fuzzy.DefaultMinSimilarity, "Minimum fuzzy similarity `percent` (0-100).", catFuzzy)
		flFuzzySameExt       = boolFlag("fuzzy-same-ext", "", false, "In fuzzy mode, only compare files with the same extension.", catFuzzy)
		flFuzzyMaxCandidates = intFlag("fuzzy-max-candidates", "", fuzzy.DefaultMaxFuzzyCandidates, "Max files to group in fuzzy mode (0=default, -1=unlimited).", catFuzzy)
		flFuzzyMinSize       = stringFlag("fuzzy-min-size", "", fuzzy.DefaultFuzzyMinSizeStr, "Skip files smaller than this `size` in fuzzy mode (e.g. 4K, 1MiB).", catFuzzy)

		// Duplicate Actions
		flKeep        = uintFlag("remove", "r", 0, "Operate on duplicates, keeping only this many `keep` files per group.", catActions)
		flLinkMode    = boolFlag("link", "l", false, "Convert extra duplicates into symlinks instead of deleting them (use with --remove).", catActions)
		flReflinkMode = boolFlag("reflink", "R", false, "Convert extra duplicates into reflinks (copy-on-write clones) instead of deleting them (use with --remove; requires a reflink-capable filesystem such as APFS, Btrfs, or XFS with reflink=1).", catActions)

		// Output & Export
		flTextOutput  = boolFlag("text", "t", false, "Dump results in grep/text friendly format. Useful for scripting.", catOutput)
		flShowBullets = boolFlag("bullet", "b", false, "Show duplicates as formatted bullet list.", catOutput)
		flCSVOut      = stringFlag("csv-out", "", "", "Write duplicate groups to the specified CSV `file`.", catOutput)
		flJSONOut     = stringFlag("json-out", "", "", "Write duplicate groups to the specified JSON `file`.", catOutput)
		flDetectFS    = stringFlag("fs-detect", "", "", "Detect filesystem in use by specified `path`.", catOutput)

		// Backup & Restore
		flBackupFile  = stringFlag("backup", "", "", "Write duplicate restore backup JSONL to the specified `file`.", catRestore)
		flRestoreFile = stringFlag("restore", "", "", "Restore duplicate files from the specified JSONL `file`.", catRestore)
		flDryRun      = boolFlag("dry-run", "n", false, "With --restore, print actions without writing files.", catRestore)
		flVerifyHash  = boolFlag("verify-hash", "", true, "With --restore, verify canonical file hashes before replay.", catRestore)
	)
	// The exclude flag can take multiple path targets; -x is its shorthand.
	flag.Var(&flExcludePaths, "exclude", "Exclude a `path` from scanning (repeatable).")
	flag.Var(&flExcludePaths, "x", "Exclude a `path` from scanning (repeatable).")
	registerFlag("x", "exclude", catFilter)
	flag.Parse()

	if *flGui {
		if err := validateGUIBuild(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	shallowMode := *flNameOnly || *flFileShallow != ""
	shallowTargetName, shallowErr := validateShallowMode(*flNameOnly, *flFileShallow, *flSingleFile, *flBackupFile)
	if shallowErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", shallowErr)
		os.Exit(1)
	}

	if *flLinkMode && *flReflinkMode {
		fmt.Fprintf(os.Stderr, "invalid invocation: --link and --reflink cannot be combined\n")
		os.Exit(1)
	}

	fuzzyMode := *flFuzzy
	if fuzzyErr := validateFuzzyMode(fuzzyMode, shallowMode, *flSingleFile, *flBackupFile, *flRestoreFile, *flKeep, *flLinkMode || *flReflinkMode, *flFuzzyThreshold); fuzzyErr != nil {
		fmt.Fprintf(os.Stderr, "invalid fuzzy invocation: %v\n", fuzzyErr)
		os.Exit(1)
	}

	var fuzzyMinFileSize int64
	if fuzzyMode {
		if *flFuzzyMinSize != "" && *flFuzzyMinSize != "0" {
			parsed, err := utils.ParseSize(*flFuzzyMinSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --fuzzy-min-size value %q: %v\n", *flFuzzyMinSize, err)
				os.Exit(1)
			}
			if parsed > uint64(math.MaxInt64) {
				fmt.Fprintf(os.Stderr, "--fuzzy-min-size %s exceeds supported file size limit (%d bytes)\n", *flFuzzyMinSize, int64(math.MaxInt64))
				os.Exit(1)
			}
			fuzzyMinFileSize = int64(parsed) // #nosec G115 -- bounds checked above
		}
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
		if err := validateRestoreMode(*flRestoreFile, *flBackupFile, flag.Args(), *flGui, *flTextOutput, *flShowBullets, *flCSVOut,
			*flJSONOut, *flSingleFile, *flFileShallow, *flNameOnly, *flKeep, *flLinkMode || *flReflinkMode); err != nil {
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

	MinFileSize := int64(0)

	// XXX: NOTE: This logic started to get a little messy. I need to refactor several blocks. If anyone cares to refactor
	//            things please do!
	if *flMinFileSize != "" {
		value, err := utils.ParseSize(*flMinFileSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid value for --min-size: %v\n", err)
			os.Exit(1)
		}
		if value > uint64(math.MaxInt64) {
			fmt.Fprintf(os.Stderr, "--min-size %s exceeds supported file size limit (%d bytes)\n", *flMinFileSize, int64(math.MaxInt64))
			os.Exit(1)
		}

		MinFileSize = int64(value) // #nosec G115 -- bounds checked above
		if MinFileSize > 0 {
			fmt.Printf("Skipping files smaller than: ~ %s.\n", utils.DisplaySize(uint64(MinFileSize)))
		}
		dsklog.Dlogger.Debugf("Min file size set to %d bytes.\n", MinFileSize)
	}

	MaxFileSize, err := resolveMaxFileSize(*flAllSizes, *flMaxFileSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if *flAllSizes || *flMaxFileSize != "" {
		if MaxFileSize > 0 {
			fmt.Printf("Skipping files larger than: %s (%d bytes).\n", utils.DisplaySize(uint64(MaxFileSize)), MaxFileSize)
		} else {
			fmt.Printf("No maximum file size limit configured.\n")
		}
	}
	dsklog.Dlogger.Debugf("Max file size set to %d bytes.\n", MaxFileSize)

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

	var singleTarget *singleFileTarget
	if *flSingleFile != "" && !shallowMode {
		var prepErr error
		singleTarget, prepErr = prepareSingleFileTarget(*flSingleFile, hashAlgo, hashOptions)
		if prepErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", prepErr)
			os.Exit(1)
		}
		pterm.Info.Printf("Searching for duplicates of %s\n", singleTarget.filePath)
	}
	singleFileMode := singleTarget != nil
	if fuzzyMode {
		pterm.Info.Printf("Searching for near-duplicate file content (threshold >= %d%%)\n", *flFuzzyThreshold)
		if *flFuzzySameExt {
			pterm.Info.Println("Fuzzy mode extension filter enabled")
		}
		if fuzzyMinFileSize > 0 {
			pterm.Info.Printf("Fuzzy mode skipping files smaller than %s (use --fuzzy-min-size 0 to disable)\n", utils.DisplaySize(uint64(fuzzyMinFileSize)))
		}
	}
	if shallowMode && !fuzzyMode {
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
		pterm.Error.Printf("Duplicate threshold %d is invalid; --dups must be >= 2\n", minDups)
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
		SkipHidden:     resolveSkipHidden(*flIncludeHidden, shallowTargetName, *flSingleFile),
		SkipVirtualFS:  !*flIncludeVFS,
		OneFileSystem:  *flOneFileSystem || *flXdev,
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
	fuzzyCandidates := make([]dwalk.FileCandidate, 0, 4096)
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
			if fuzzyMode {
				if fuzzyMinFileSize > 0 && candidate.Size < fuzzyMinFileSize {
					continue
				}
				fuzzyCandidates = append(fuzzyCandidates, candidate)
				continue
			}
			if shallowMode {
				name := filepath.Base(candidate.Path)
				if shallowTargetName != "" && name != shallowTargetName {
					continue
				}
				nameGroups[name] = append(nameGroups[name], candidate)
				continue
			}
			if singleFileMode && candidate.Size != singleTarget.fileSize {
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
	var fuzzyProcessed uint
	var fuzzySkipped uint

	if fuzzyMode {
		addedGroups, processed, skippedBySignature, fuzzyErr := addFuzzyContentGroups(dMap, fuzzyCandidates, minDups, *flFuzzyThreshold, *flFuzzySameExt, *flFuzzyMaxCandidates,
			func(done uint, processed uint, skipped uint, total uint) {
				progressMsg := fmt.Sprintf("Scanned %d files, fuzzy-processed %d/%d (kept %d, skipped %d)...", scannedFiles, done, total, processed, skipped)
				updateProgress(progressMsg)
			},
			func(done, total int) {
				progressMsg := fmt.Sprintf("Scanned %d files, fuzzy-grouping %d/%d...", scannedFiles, done, total)
				updateProgress(progressMsg)
			},
		)
		if fuzzyErr != nil {
			fmt.Fprintf(os.Stderr, "fuzzy scan failed: %v\n", fuzzyErr)
			os.Exit(1)
		}
		fuzzyProcessed = processed
		fuzzySkipped = skippedBySignature
		dsklog.Dlogger.Debugf("Added %d fuzzy content groups; skipped %d files during signature stage", addedGroups, skippedBySignature)
	} else if shallowMode {
		addedGroups, skippedByName := addNameOnlyGroups(dMap, nameGroups, minDups)
		nameGroups = nil
		dsklog.Dlogger.Debugf("Added %d shallow filename groups; skipped %d files with unique names", addedGroups, skippedByName)
	} else {
		sampleList, skippedBySize := eligibleHashCandidates(sizeGroups, minDups, singleFileMode)
		sizeGroups = nil
		dsklog.Dlogger.Debugf("Skipped %d files with unique sizes before sample hashing", skippedBySize)
		sampledFiles, fullHashedFiles = runContentPipeline(ctx, dMap, sampleList, minDups, singleTarget, hashAlgo, hashOptions, tickC, updateProgress)
	}

	stopProgress()
	duration := time.Since(start)

	// Stop profiling after this point. Profile data should now be
	// written to disk.
	pprof.StopCPUProfile()

	// Status bar update
	finalInfo := "Scanned " + pterm.LightWhite(scannedFiles) + " files, sampled " +
		pterm.LightWhite(sampledFiles) + " candidates, fully hashed " +
		pterm.LightWhite(fullHashedFiles) + " in " + pterm.LightWhite(duration)
	if fuzzyMode {
		finalInfo = "Scanned " + pterm.LightWhite(scannedFiles) + " files, fuzzy-signatured " + pterm.LightWhite(fuzzyProcessed) +
			" (skipped " + pterm.LightWhite(fuzzySkipped) + ") in " + pterm.LightWhite(duration)
	} else if shallowMode {
		finalInfo = "Scanned " + pterm.LightWhite(scannedFiles) + " files by name in " + pterm.LightWhite(duration)
	}
	pterm.Success.Println(finalInfo)

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

	// Write backup manifest before any batch-mode action. In interactive mode
	// the manifest is written lazily by the TUI/GUI via applyOptions.BackupPath.
	if *flBackupFile != "" && (keepCount > 0 || *flTimeOnly || *flTextOutput || *flShowBullets || *flCSVOut != "" || *flJSONOut != "") {
		writeBackupManifest(dMap, hashAlgo, *flBackupFile)
	}

	applyOptions := dupview.ApplyOptions{
		BackupPath:    *flBackupFile,
		HashAlgorithm: hashAlgo,
		SkipConfirm:   *flNoConfirm,
	}

	switch {
	case keepCount > 0 && *flLinkMode:
		linkedPaths, linkErr := dMap.LinkDuplicates(keepCount)
		fmt.Printf("Converted %d duplicate files to symlinks, kept %d real file(s) per group.\n", len(linkedPaths), keepCount)
		if linkErr != nil {
			fmt.Fprintf(os.Stderr, "Linking completed with errors: %v\n", linkErr)
			os.Exit(1)
		}
	case keepCount > 0 && *flReflinkMode:
		reflinkedPaths, reflinkErr := dMap.ReflinkDuplicates(keepCount)
		fmt.Printf("Converted %d duplicate files to reflinks, kept %d real file(s) per group.\n", len(reflinkedPaths), keepCount)
		if reflinkErr != nil {
			fmt.Fprintf(os.Stderr, "Reflinking completed with errors: %v\n", reflinkErr)
			os.Exit(1)
		}
	case keepCount > 0:
		removedPaths, removeErr := dMap.RemoveDuplicates(keepCount)
		fmt.Printf("Removed %d duplicate files, kept %d per group.\n", len(removedPaths), keepCount)
		if removeErr != nil {
			fmt.Fprintf(os.Stderr, "Removal completed with errors: %v\n", removeErr)
			os.Exit(1)
		}
	case *flCSVOut != "":
		pterm.Info.Printf("Writing CSV to %s...\n", *flCSVOut)
		if err := dMap.WriteCSV(*flCSVOut); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write CSV output: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Printf("CSV file %s written to disk.\n", *flCSVOut)
	case *flJSONOut != "":
		pterm.Info.Printf("Writing JSON to %s...\n", *flJSONOut)
		if err := dMap.WriteJSON(*flJSONOut); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write JSON output: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Printf("JSON file %s written to disk.\n", *flJSONOut)
	case *flTimeOnly:
		// scan-only benchmark mode; elapsed time already printed above
	case *flTextOutput:
		dMap.PrintDmap()
	case *flShowBullets:
		dMap.ShowResultsBullet()
	case *flGui:
		if err := launchGUI(dMap, applyOptions); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	default:
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

func resolveSkipHidden(includeHidden bool, shallowTargetName string, singleFilePath string) bool {
	if includeHidden || shallowTargetIsHidden(shallowTargetName) || singleFileTargetIsHidden(singleFilePath) {
		return false
	}
	return true
}

// singleFileTarget holds precomputed data for --file mode.
type singleFileTarget struct {
	filePath     string
	fileSize     int64
	digest       dmap.Digest
	sampleDigest dmap.Digest
}

// prepareSingleFileTarget stats, hashes, and sample-hashes the --file target.
func prepareSingleFileTarget(path string, hashAlgo dfs.HashAlgorithm, opts dfs.HashOptions) (*singleFileTarget, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat --file path %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--file path must be a regular file: %s", path)
	}
	dfile, err := dfs.NewDfileWithOptions(path, info.Size(), hashAlgo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to hash --file target %s: %w", path, err)
	}
	sample, err := dfs.HashFileSampleWithOptions(dfile.FileName(), info.Size(), hashAlgo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to sample --file target %s: %w", path, err)
	}
	return &singleFileTarget{
		filePath:     dfile.FileName(),
		fileSize:     info.Size(),
		digest:       dmap.Digest(dfile.Hash()),
		sampleDigest: dmap.Digest(sample.Digest),
	}, nil
}

func shallowTargetIsHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func singleFileTargetIsHidden(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(path), ".")
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

// runContentPipeline runs the two-phase sample-then-full-hash pipeline and populates dMap.
// It returns the count of files sampled and the count fully hashed.
func runContentPipeline(
	ctx context.Context,
	dMap *dmap.Dmap,
	sampleList []dwalk.FileCandidate,
	minDups uint,
	singleTarget *singleFileTarget,
	hashAlgo dfs.HashAlgorithm,
	hashOptions dfs.HashOptions,
	tickC <-chan time.Time,
	updateProgress func(string),
) (sampledFiles, fullHashedFiles uint) {
	singleFileMode := singleTarget != nil
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
					sample, err := dfs.HashFileSampleWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
					if err != nil {
						dsklog.Dlogger.Debugf("Skipping file after sample failure %s: %v", candidate.Path, err)
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
				if singleFileMode && sample.digest != singleTarget.sampleDigest {
					continue
				}
				key := sampleKey{size: sample.candidate.Size, digest: sample.digest}
				sampleGroups[key] = append(sampleGroups[key], sample)
			case <-tickC:
				updateProgress(fmt.Sprintf("Sampled %d/%d candidate files...", sampledFiles, len(sampleList)))
			}
		}
	}

	directFiles, fullHashList, skippedBySample := eligibleSampleCandidates(sampleGroups, minDups, singleFileMode)
	sampleGroups = nil
	dsklog.Dlogger.Debugf("Skipped %d files with unique samples before full hashing", skippedBySample)

	for _, file := range directFiles {
		dMap.AddPath(file.digest, file.candidate.Path)
	}

	if len(fullHashList) == 0 {
		return
	}

	hashJobs := make(chan dwalk.FileCandidate, min(len(fullHashList), 4096))
	hashedFiles := make(chan *dfs.Dfile, min(len(fullHashList), 4096))
	workerCount := hashWorkerCount(len(fullHashList))

	var hashWG sync.WaitGroup
	hashWG.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer hashWG.Done()
			for candidate := range hashJobs {
				dFile, err := dfs.NewDfileWithOptions(candidate.Path, candidate.Size, hashAlgo, hashOptions)
				if err != nil {
					dsklog.Dlogger.Debugf("Skipping file after hash failure %s: %v", candidate.Path, err)
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
			updateProgress(fmt.Sprintf("Hashed %d/%d full candidate files...", fullHashedFiles, len(fullHashList)))
		}
	}
	return
}
func writeBackupManifest(dMap *dmap.Dmap, algo dfs.HashAlgorithm, path string) {
	entries, err := manifest.EntriesFromDmap(dMap, algo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build restore manifest: %v\n", err)
		os.Exit(1)
	}
	if err := manifest.Write(path, entries); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write restore manifest: %v\n", err)
		os.Exit(1)
	}
	pterm.Success.Printf("Restore backup file with %d entries written to %s.\n", len(entries), path)
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

func resolveMaxFileSize(allSizes bool, maxSizeValue string) (int64, error) {
	if allSizes && maxSizeValue != "" {
		return 0, fmt.Errorf("--all-sizes cannot be combined with --max-size")
	}
	if allSizes {
		return 0, nil
	}
	if maxSizeValue == "" {
		return dwalk.MAX_FILE_SIZE, nil
	}

	value, err := utils.ParseSize(maxSizeValue)
	if err != nil {
		return 0, fmt.Errorf("invalid value for --max-size: %v", err)
	}
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("--max-size %s exceeds supported file size limit (%d bytes)", maxSizeValue, int64(math.MaxInt64))
	}
	return int64(value), nil // #nosec G115 -- bounds checked above
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
