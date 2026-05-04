// dwalk is a parallel, fast directory walker written for the needs of dskditto
package dwalk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"

	"golang.org/x/sync/semaphore"
)

const (
	MAX_FILE_SIZE           uint = 1024 * 1024 * 1024 * 4 // 4GB
	DEFAULT_DIR_CONCURRENCY      = 50                     // Optimal balance for directory reading
)

// These are generally special VFS directories a user normally wouldn't want to recurse into nor touch.
// In future versions we can expand user config abilities.
var defaultSkipDirPrefixes = []string{
	"/proc",
	"/sys",
	"/dev",
	"/run",
	"/var/run",
}

// DWalk is our primary object for traversing filesystem
// in a parallel manner.
type DWalk struct {
	rootDirs []string
	wg       sync.WaitGroup

	// Channel used to communicate with main monitor goroutine.
	dFiles          chan<- *dfs.Dfile
	candidateFiles  chan<- FileCandidate
	sem             *semaphore.Weighted
	skipHidden      bool
	skipEmpty       bool
	skipSymLinks    bool
	minFileSize     uint
	maxFileSize     uint
	hashAlgo        dfs.HashAlgorithm
	noCache         bool
	skipDirPrefixes []string
	excludePaths    []string
	maxDepth        int

	// seenFiles tracks unique files by device+inode so multiple hardlinks
	// are treated as a single file during scanning.
	seenMu    sync.Mutex
	seenFiles map[fileIdentity]struct{}
}

// FileCandidate is a regular file that survived the cheap walker filters.
// The content hash is intentionally deferred so callers can skip files that
// cannot be duplicates by size.
type FileCandidate struct {
	Path string
	Size int64
}

// NewDWalker returns a new DWalk instance that accepts traversal options.
func NewDWalker(rootDirs []string, dFiles chan<- *dfs.Dfile, cfg config.Config) *DWalk {
	return newDWalker(rootDirs, dFiles, nil, cfg)
}

// NewCandidateWalker returns a walker that emits file path and size without
// hashing file contents.
func NewCandidateWalker(rootDirs []string, candidates chan<- FileCandidate, cfg config.Config) *DWalk {
	return newDWalker(rootDirs, nil, candidates, cfg)
}

func newDWalker(rootDirs []string, dFiles chan<- *dfs.Dfile, candidates chan<- FileCandidate, cfg config.Config) *DWalk {

	hashAlgo := cfg.HashAlgorithm
	if hashAlgo == "" {
		hashAlgo = dfs.HashSHA256
	}

	maxSize := cfg.MaxFileSize
	if maxSize == 0 {
		maxSize = MAX_FILE_SIZE
	}

	maxDepth := cfg.MaxDepth
	if maxDepth < 0 {
		maxDepth = -1
	}

	var skipPrefixes []string
	if cfg.SkipVirtualFS {
		skipPrefixes = normalizeSkipPrefixes(defaultSkipDirPrefixes)
	}

	excludePaths := normalizeExcludePaths(cfg.ExcludePaths)

	walker := &DWalk{
		rootDirs:        normalizeRootDirs(rootDirs),
		dFiles:          dFiles,
		candidateFiles:  candidates,
		skipHidden:      cfg.SkipHidden,
		skipEmpty:       cfg.SkipEmpty,
		skipSymLinks:    cfg.SkipSymLinks,
		minFileSize:     cfg.MinFileSize,
		maxFileSize:     maxSize,
		hashAlgo:        hashAlgo,
		noCache:         cfg.NoCache,
		skipDirPrefixes: skipPrefixes,
		excludePaths:    excludePaths,
		maxDepth:        maxDepth,
		seenFiles:       make(map[fileIdentity]struct{}),
	}

	// Set semaphore to optimal value based on system resources unless the user
	// wants to benchmark a specific directory-read concurrency.
	optimalConcurrency, configuredConcurrency := resolveDirConcurrency(cfg.DirConcurrency)
	if !configuredConcurrency {
		dsklog.Dlogger.Infof("Setting directory concurrency to %d (based on %d CPUs)", optimalConcurrency, runtime.NumCPU())
	} else {
		dsklog.Dlogger.Infof("Setting directory concurrency to %d (configured; %d CPUs)", optimalConcurrency, runtime.NumCPU())
	}
	walker.sem = semaphore.NewWeighted(int64(optimalConcurrency))
	return walker

}

// Run method kicks off filesystem crawl for file dupes.
func (d *DWalk) Run(ctx context.Context) {

	for _, root := range d.rootDirs {
		if d.shouldSkipPath(root) {
			dsklog.Dlogger.Infof("Skipping directory %s due to restricted filesystem", root)
			continue
		}
		d.wg.Add(1)
		go walkDir(ctx, root, 0, d)
	}

	// Wait for all goroutines to finish.
	go func() {
		d.wg.Wait()
		if d.dFiles != nil {
			close(d.dFiles)
		}
		if d.candidateFiles != nil {
			close(d.candidateFiles)
		}
	}()

}

// cancelled polls, checking for cancellation.
func cancelled(ctx context.Context) bool {

	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}

}

// walkDir recursively walk directories and send files to our monitor go routine
// (in main.go) to be added to the duplication map.
func walkDir(ctx context.Context, dir string, depth int, d *DWalk) {
	defer func() {
		if r := recover(); r != nil {
			dsklog.Dlogger.Errorf("Recovered panic while walking directory %s: %v", dir, r)
		}
		d.wg.Done()
	}()

	// Check for cancellation.
	if cancelled(ctx) {
		return
	}

	if d.shouldSkipPath(dir) {
		dsklog.Dlogger.Debugf("Skipping directory %s due to restricted filesystem", dir)
		return
	}

	for _, entry := range dirEntries(ctx, dir, d) {
		// Handle processing of dotfiles (hidden)
		name := entry.Name()
		if d.skipHidden && strings.HasPrefix(name, ".") {
			dsklog.Dlogger.Debugf("Skipping hidden entry: %s", filepath.Join(dir, name))
			continue
		}

		if d.skipSymLinks && entry.Type()&os.ModeSymlink != 0 {
			dsklog.Dlogger.Debugf("Skipping symlink: %s", filepath.Join(dir, name))
			continue
		}

		if entry.IsDir() {
			subDir := filepath.Join(dir, name)
			if d.shouldSkipPath(subDir) {
				dsklog.Dlogger.Debugf("Skipping directory %s due to restricted filesystem", subDir)
				continue
			}
			if d.maxDepth >= 0 && depth >= d.maxDepth {
				dsklog.Dlogger.Debugf("Skipping directory %s due to max depth %d", subDir, d.maxDepth)
				continue
			}
			d.wg.Add(1)
			go walkDir(ctx, subDir, depth+1, d)
			continue
		}

		absFileName := filepath.Join(dir, name)
		meta, err := statFile(absFileName)
		if err != nil {
			dsklog.Dlogger.Debugf("Error getting file info for %s: %v", absFileName, err)
			continue
		}

		if d.skipSymLinks && meta.mode&os.ModeSymlink != 0 {
			dsklog.Dlogger.Debugf("Skipping symlink (resolved): %s", absFileName)
			continue
		}

		// Skip non-regular files (sockets, pipes, device files, etc.)
		if !meta.mode.IsRegular() {
			dsklog.Dlogger.Debugf("Skipping non-regular file: %s (mode: %s)", entry.Name(), meta.mode)
			continue
		}

		// Check file size properties the user set.
		fileSize := uint(max(meta.size, 0)) // #nosec G115
		if d.skipEmpty && fileSize == 0 {
			dsklog.Dlogger.Debugf("Skipping empty file: %s", name)
			continue
		}
		if d.minFileSize > 0 && fileSize < d.minFileSize {
			dsklog.Dlogger.Debugf("File %s smaller than minimum. Skipping", name)
			continue
		}
		if d.maxFileSize > 0 && fileSize >= d.maxFileSize {
			dsklog.Dlogger.Infof("File %s larger than maximum. Skipping", name)
			continue
		}

		if d.shouldSkipPath(absFileName) {
			dsklog.Dlogger.Debugf("Skipping excluded path: %s", absFileName)
			continue
		}

		// Treat multiple hardlinks to the same underlying inode as a single file
		// to avoid redundant hashing and duplicate entries.
		if meta.hasIdentity {
			d.seenMu.Lock()
			if _, exists := d.seenFiles[meta.identity]; exists {
				d.seenMu.Unlock()
				dsklog.Dlogger.Debugf("Skipping hardlink duplicate: %s", filepath.Join(dir, name))
				continue
			}
			d.seenFiles[meta.identity] = struct{}{}
			d.seenMu.Unlock()
		}

		d.emitFile(ctx, absFileName, meta.size)
	}
}

func (d *DWalk) emitFile(ctx context.Context, path string, size int64) {
	if d.candidateFiles != nil {
		select {
		case <-ctx.Done():
		case d.candidateFiles <- FileCandidate{Path: path, Size: size}:
		}
		return
	}

	if d.dFiles == nil {
		return
	}

	dFileEntry, err := dfs.NewDfileWithOptions(path, size, d.hashAlgo, dfs.HashOptions{NoCache: d.noCache})
	if err != nil {
		return
	}
	select {
	case <-ctx.Done():
	case d.dFiles <- dFileEntry:
	}
}

// dirEntries returns contents of a directory specified by dir.
// The semaphore limits concurrency; preventing system resource
// exhaustion.
func dirEntries(ctx context.Context, dir string, d *DWalk) []os.DirEntry {

	if cancelled(ctx) {
		return nil
	}

	// Semaphore helps control concurrency.
	if err := d.sem.Acquire(ctx, 1); err != nil {
		return nil
	}
	defer d.sem.Release(1)

	entries, err := os.ReadDir(dir)
	if err != nil {
		dsklog.Dlogger.Errorf("Directory read error: %v", err)
		return nil
	}

	return entries
}

// getOptimalConcurrency returns optimal concurrency based on system resources
func getOptimalConcurrency() int {
	procs := runtime.GOMAXPROCS(0)
	if procs < 1 {
		procs = runtime.NumCPU()
	}
	concurrency := min(procs*4, 128)

	dsklog.Dlogger.Debugf("Directory walker concurrency: %d (procs=%d)", concurrency, procs)
	return concurrency
}

func resolveDirConcurrency(configured int) (int, bool) {
	if configured > 0 {
		return configured, true
	}
	return getOptimalConcurrency(), false
}

// normalizeSkipPrefixes filters and deduplicates skip prefixes by cleaning their absolute paths,
// discarding empty entries and root-only paths before returning the normalized list.
func normalizeSkipPrefixes(prefixes []string) []string {
	cleaned := make([]string, 0, len(prefixes))
	seen := make(map[string]struct{}, len(prefixes))
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		normalized := cleanAbsPath(p)
		if normalized == "" || normalized == string(os.PathSeparator) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cleaned = append(cleaned, normalized)
	}
	return cleaned
}

func normalizeExcludePaths(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		normalized := cleanAbsPath(p)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cleaned = append(cleaned, normalized)
	}
	return cleaned
}

func normalizeRootDirs(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	roots := make([]string, 0, len(paths))
	for _, p := range paths {
		normalized := cleanAbsPath(p)
		if normalized == "" {
			continue
		}
		roots = append(roots, normalized)
	}
	return roots
}

func (d *DWalk) shouldSkipPath(path string) bool {
	if len(d.excludePaths) == 0 && len(d.skipDirPrefixes) == 0 {
		return false
	}
	normalized := cleanAbsPath(path)
	if normalized == "" {
		return false
	}
	for _, excluded := range d.excludePaths {
		if normalized == excluded || strings.HasPrefix(normalized, excluded+string(os.PathSeparator)) {
			return true
		}
	}
	for _, prefix := range d.skipDirPrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func cleanAbsPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(abs)
	}
	return abs
}
