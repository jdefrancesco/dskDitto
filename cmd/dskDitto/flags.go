package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jdefrancesco/dskDitto/internal/fuzzy"

	"github.com/pterm/pterm"
)

func init() {

	// Custom help message
	flag.Usage = func() {
		showHeader(false)
		fmt.Fprintf(os.Stderr, "Usage: dskDitto [options] PATHS\n\n")
		printFlagHelpTable()
	}
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

// commandOptions contains the parsed command-line settings.
type commandOptions struct {
	noBanner           bool
	showVersion        bool
	cpuProfile         string
	timeOnly           bool
	gui                bool
	colorSafe          bool
	noConfirm          bool
	minFileSize        string
	maxFileSize        string
	allSizes           bool
	includeEmpty       bool
	skipSymLinks       bool
	includeHidden      bool
	noRecurse          bool
	depth              int
	includeVFS         bool
	oneFileSystem      bool
	xdev               bool
	dirConcurrency     int
	noCache            bool
	minDups            uint
	hashAlgo           string
	singleFile         string
	nameOnly           bool
	fileShallow        string
	fuzzy              bool
	fuzzyThreshold     int
	fuzzySameExt       bool
	fuzzyMaxCandidates int
	fuzzyMinSize       string
	keep               uint
	linkMode           bool
	reflinkMode        bool
	textOutput         bool
	showBullets        bool
	csvOut             string
	jsonOut            string
	detectFS           string
	backupFile         string
	restoreFile        string
	dryRun             bool
	verifyHash         bool
	excludePaths       stringListFlag
	paths              []string
}

func parseOptions() commandOptions {
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

	return commandOptions{
		noBanner:           *flNoBanner,
		showVersion:        *flShowVersion,
		cpuProfile:         *flCpuProfile,
		timeOnly:           *flTimeOnly,
		gui:                *flGui,
		colorSafe:          *flColorSafe,
		noConfirm:          *flNoConfirm,
		minFileSize:        *flMinFileSize,
		maxFileSize:        *flMaxFileSize,
		allSizes:           *flAllSizes,
		includeEmpty:       *flIncludeEmpty,
		skipSymLinks:       *flSkipSymLinks,
		includeHidden:      *flIncludeHidden,
		noRecurse:          *flNoRecurse,
		depth:              *flDepth,
		includeVFS:         *flIncludeVFS,
		oneFileSystem:      *flOneFileSystem,
		xdev:               *flXdev,
		dirConcurrency:     *flDirConcurrency,
		noCache:            *flNoCache,
		minDups:            *flMinDups,
		hashAlgo:           *flHashAlgo,
		singleFile:         *flSingleFile,
		nameOnly:           *flNameOnly,
		fileShallow:        *flFileShallow,
		fuzzy:              *flFuzzy,
		fuzzyThreshold:     *flFuzzyThreshold,
		fuzzySameExt:       *flFuzzySameExt,
		fuzzyMaxCandidates: *flFuzzyMaxCandidates,
		fuzzyMinSize:       *flFuzzyMinSize,
		keep:               *flKeep,
		linkMode:           *flLinkMode,
		reflinkMode:        *flReflinkMode,
		textOutput:         *flTextOutput,
		showBullets:        *flShowBullets,
		csvOut:             *flCSVOut,
		jsonOut:            *flJSONOut,
		detectFS:           *flDetectFS,
		backupFile:         *flBackupFile,
		restoreFile:        *flRestoreFile,
		dryRun:             *flDryRun,
		verifyHash:         *flVerifyHash,
		excludePaths:       flExcludePaths,
		paths:              flag.Args(),
	}
}
