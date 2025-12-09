// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { HashDigest --> [fileClone1, fileClone2, etc...]}
//
// That is, SHA256 hash of file will serve as our hash map key, which maps to a simple list of file names.
package dmap

import (
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
	dsklog.Dlogger.Debug("Dmap created with initial size: ", mapInitSize)

	return dmap, nil
}

// Add will take a dfile and add it the map.
func (d *Dmap) Add(dfile *dfs.Dfile) {
	hash := Digest(dfile.Hash())
	d.filesMap[hash] = append(d.filesMap[hash], dfile.FileName())
	d.fileCount++
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
		hash := fmt.Sprintf("%x", k)
		fmt.Printf("Hash: %s  \n", hash)
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
		h := fmt.Sprintf("%x", hash)
		pterm.Println(pterm.Green("Hash: ") + pterm.Cyan(h))
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

// MinDuplicates returns the current threshold for displaying duplicate groups.
func (d *Dmap) MinDuplicates() uint {
	return d.minDuplicates
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
			continue
		}

		d.filesMap[hash] = survivors
	}

	if len(errs) > 0 {
		return linked, errors.Join(errs...)
	}

	return linked, nil
}
