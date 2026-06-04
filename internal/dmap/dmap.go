// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { HashDigest --> [fileClone1, fileClone2, etc...]}
//
// That is, SHA256 hash of file will serve as our hash map key, which maps to a simple list of file names.
package dmap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"

	"github.com/pterm/pterm"
)

type Digest [32]byte

type MatchType string

const (
	MatchContent MatchType = "content"
	MatchName    MatchType = "name"
	MatchFuzzy   MatchType = "fuzzy"
)

type MatchInfo struct {
	Type MatchType
	Key  string
}

// DigestFromHex converts a hex string to Digest
func DigestFromHex(hexStr string) (Digest, error) {
	var hash Digest
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return hash, err
	}
	if len(bytes) != 32 {
		return hash, fmt.Errorf("invalid hash length: expected 32 bytes, got %d", len(bytes))
	}
	copy(hash[:], bytes)
	return hash, nil
}

// Initial size of our map. This will grow, but reasonable starting size helps performance.
const mapInitSize = 4096

// Dmap structure will hold our file duplication data.
// It is the primary data structure that will house the results
// that will eventually be returned to the user.
type Dmap struct {
	// Primary map structure.
	filesMap map[Digest][]string
	matches  map[Digest]MatchInfo

	// Files deffered for reasons such as size are stored here for later processing.
	deferredFiles []string
	// Number of files in our map.
	fileCount uint
	// Batches of duplicate files.
	// batchCount   uint
	minDuplicates uint
}

// NewDmap returns a new Dmap structure.
func NewDmap(minDuplicates uint) (*Dmap, error) {

	dmap := &Dmap{
		fileCount:     0,
		minDuplicates: minDuplicates,
	}
	if dmap.minDuplicates < 2 {
		dmap.minDuplicates = 2
	}
	// Initialize our map.
	dmap.filesMap = make(map[Digest][]string, mapInitSize)
	dmap.matches = make(map[Digest]MatchInfo, mapInitSize)
	dsklog.Dlogger.Debug("Dmap created with initial size: ", mapInitSize)

	return dmap, nil
}

// Add will take a dfile and add it the map.
func (d *Dmap) Add(dfile *dfs.Dfile) {
	d.AddPath(Digest(dfile.Hash()), dfile.FileName())
}

// AddPath records a path under an already computed digest.
func (d *Dmap) AddPath(hash Digest, path string) {
	if path == "" {
		return
	}
	if _, exists := d.matches[hash]; !exists {
		d.matches[hash] = MatchInfo{Type: MatchContent, Key: fmt.Sprintf("%x", hash)}
	}
	d.filesMap[hash] = append(d.filesMap[hash], path)
	d.fileCount++
}

// AddNamePath records a path under a shallow filename match key.
func (d *Dmap) AddNamePath(name, path string) {
	if name == "" || path == "" {
		return
	}
	hash := NameDigest(name)
	d.matches[hash] = MatchInfo{Type: MatchName, Key: name}
	d.filesMap[hash] = append(d.filesMap[hash], path)
	d.fileCount++
}

// AddFuzzyPath records a path under a fuzzy content-match key.
func (d *Dmap) AddFuzzyPath(key, path string) {
	if key == "" || path == "" {
		return
	}
	hash := FuzzyDigest(key)
	d.matches[hash] = MatchInfo{Type: MatchFuzzy, Key: key}
	d.filesMap[hash] = append(d.filesMap[hash], path)
	d.fileCount++
}

// NameDigest returns a stable synthetic digest for a shallow filename group.
func NameDigest(name string) Digest {
	sum := sha256.Sum256([]byte("dskditto:name:" + name))
	return Digest(sum)
}

// FuzzyDigest returns a stable synthetic digest for a fuzzy content group.
func FuzzyDigest(key string) Digest {
	sum := sha256.Sum256([]byte("dskditto:fuzzy:" + key))
	return Digest(sum)
}

// AddDeferredFile will add a file to the deferredFiles slice.
func (d *Dmap) AddDeferredFile(file string) {
	if file == "" {
		return
	}
	d.deferredFiles = append(d.deferredFiles, file)
}

// PrintDmap will print entries currently stored in map in more text friendly way.
func (d *Dmap) PrintDmap() {
	for k, v := range d.filesMap {
		if uint(len(v)) < d.minDuplicates {
			continue
		}
		fmt.Printf("%s  \n", d.headerFor(k))
		for i, f := range v {
			fmt.Printf(" %d: %s \n", i+1, f)
		}
		fmt.Printf("\n\n")
	}
}

// ShowResultsBullet will display duplicates held in our Dmap as
// a bullet list..
func (d *Dmap) ShowResultsBullet() {

	var bl []pterm.BulletListItem
	for hash, files := range d.filesMap {

		if uint(len(files)) < d.minDuplicates {
			continue
		}
		info := d.MatchInfo(hash)
		label := "Hash: "
		value := info.Key
		if info.Type == MatchName {
			label = "Name: "
		} else if info.Type == MatchFuzzy {
			label = "Similar: "
		}
		pterm.Println(pterm.Green(label) + pterm.Cyan(value))
		for _, f := range files {
			blContent := pterm.BulletListItem{Level: 0, Text: f}
			bl = append(bl, blContent)
		}
		pterm.DefaultBulletList.WithItems(bl).Render()
		bl = nil
	}

}

func (d *Dmap) IsEmpty() bool {
	return d.MapSize() == 0
}

// MapSize returns number of entries in the map.
func (d *Dmap) MapSize() int {
	return len(d.filesMap)
}

// FileCount will return the number of files our map currently
// references.
func (d *Dmap) FileCount() uint {
	return d.fileCount
}

// Get will get slice of files associated with hash.
func (d *Dmap) Get(hash Digest) (files []string, err error) {
	res, ok := d.filesMap[hash]
	if !ok {
		return []string{}, err
	}

	return res, nil
}

// GetMap will return the map.
func (d *Dmap) GetMap() map[Digest][]string {
	return d.filesMap
}

func (d *Dmap) MatchInfo(hash Digest) MatchInfo {
	if d == nil {
		return MatchInfo{Type: MatchContent, Key: ""}
	}
	if info, ok := d.matches[hash]; ok {
		if info.Type == "" {
			info.Type = MatchContent
		}
		return info
	}
	return MatchInfo{Type: MatchContent, Key: fmt.Sprintf("%x", hash)}
}

func (d *Dmap) headerFor(hash Digest) string {
	info := d.MatchInfo(hash)
	if info.Type == MatchName {
		return fmt.Sprintf("Name: %s", info.Key)
	}
	if info.Type == MatchFuzzy {
		return fmt.Sprintf("Similar: %s", info.Key)
	}
	return fmt.Sprintf("Hash: %s", info.Key)
}

// MinDuplicates returns the current threshold for displaying duplicate groups.
func (d *Dmap) MinDuplicates() uint {
	return d.minDuplicates
}

// FilterToDigest narrows the map to a single-file-mode result. It removes every
// group except the one matching digest, promotes targetPath to the first position
// within that group, and returns the number of duplicate paths found (i.e. entries
// other than targetPath). Returns 0 when the digest is absent or has no duplicates.
func (d *Dmap) FilterToDigest(digest Digest, targetPath string) int {
	if d == nil {
		return 0
	}
	paths := append([]string(nil), d.filesMap[digest]...)
	dups := countDuplicateEntries(paths, targetPath)
	if dups == 0 {
		return 0
	}
	d.filesMap[digest] = promotePathFirst(paths, targetPath)
	for hash := range d.filesMap {
		if hash != digest {
			delete(d.filesMap, hash)
			delete(d.matches, hash)
		}
	}
	return dups
}

// countDuplicateEntries returns how many entries in paths differ from target.
func countDuplicateEntries(paths []string, target string) int {
	count := 0
	for _, p := range paths {
		if p != target {
			count++
		}
	}
	return count
}

// promotePathFirst returns a copy of paths with target moved to index 0.
// If target is absent it is prepended. If target is already first the
// original slice is returned unchanged.
func promotePathFirst(paths []string, target string) []string {
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

// RemoveDuplicates removes duplicates, leaving at most "keep" files per group. Returns removed file paths.
func (d *Dmap) RemoveDuplicates(keep uint) ([]string, error) {
	if keep == 0 {
		return nil, errors.New("keep count must be greater than zero")
	}

	// Guard against integer overflow
	if keep > uint(math.MaxInt) {
		dsklog.Dlogger.Debug("keep value overflow")
		return nil, fmt.Errorf("keep count of %d exceeds maximum %d", keep, math.MaxInt)
	}
	keepThreshold := int(keep)

	var removed []string
	var errs []error

	for hash, files := range d.filesMap {
		if uint(len(files)) <= keep {
			continue
		}

		keepCount := keepThreshold
		if keepCount > len(files) {
			keepCount = len(files)
		}

		survivors := append([]string(nil), files[:keepCount]...)

		for _, path := range files[keepCount:] {
			if err := os.Remove(path); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
				survivors = append(survivors, path)
				continue
			}
			dsklog.Dlogger.Infof("Removed duplicate file: %s", path)
			removed = append(removed, path)
			if d.fileCount > 0 {
				d.fileCount--
			}
		}

		if len(survivors) == 0 {
			delete(d.filesMap, hash)
			delete(d.matches, hash)
			continue
		}

		d.filesMap[hash] = survivors
	}

	if len(errs) > 0 {
		return removed, errors.Join(errs...)
	}

	return removed, nil
}

// LinkDuplicates converts duplicates to symbolic links, leaving at most "keep" real files per group.
// It removes each extra duplicate and recreates it as a symlink pointing to one of the kept files.
// It returns the paths that were successfully converted to symlinks.
func (d *Dmap) LinkDuplicates(keep uint) ([]string, error) {
	if keep == 0 {
		return nil, errors.New("keep count must be greater than zero")
	}

	// Guard against integer overflow
	if keep > uint(math.MaxInt) {
		dsklog.Dlogger.Debug("keep value overflow")
		return nil, fmt.Errorf("keep count of %d exceeds maximum %d", keep, math.MaxInt)
	}
	keepThreshold := int(keep)

	var linked []string
	var errs []error

	for hash, files := range d.filesMap {
		if uint(len(files)) <= keep {
			continue
		}

		keepCount := keepThreshold
		if keepCount > len(files) {
			keepCount = len(files)
		}

		// Survivors remain as real files. We point all converted symlinks at the first survivor.
		survivors := append([]string(nil), files[:keepCount]...)
		target := survivors[0]

		for _, path := range files[keepCount:] {
			if err := os.Remove(path); err != nil {
				errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
				survivors = append(survivors, path)
				continue
			}
			if err := os.Symlink(target, path); err != nil {
				errs = append(errs, fmt.Errorf("symlink %s -> %s: %w", path, target, err))
				// Try to preserve logical membership if the symlink creation fails.
				survivors = append(survivors, path)
				continue
			}
			dsklog.Dlogger.Infof("Converted duplicate to symlink: %s -> %s", path, target)
			linked = append(linked, path)
		}

		if len(survivors) == 0 {
			delete(d.filesMap, hash)
			delete(d.matches, hash)
			continue
		}

		d.filesMap[hash] = survivors
	}

	if len(errs) > 0 {
		return linked, errors.Join(errs...)
	}

	return linked, nil
}
