// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { HashDigest --> [fileClone1, fileClone2, etc...]}
//
// That is, MD5 hash of file will serve as our hash map key, which maps to a simple list of file names.
// MD5 will be used for the time being, mainly for the slight speed advantage.
package dmap

import (
	"ditto/internal/config"
	"ditto/internal/dfs"
	"ditto/internal/dsklog"
	"encoding/hex"
	"fmt"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
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
	ignoreEmpty   bool
	skipSymLinks  bool
	minDuplicates uint
}

// NewDmap returns a new Dmap structure.
func NewDmap(cfg config.Config) (*Dmap, error) {

	dmap := &Dmap{
		fileCount:     0,
		ignoreEmpty:   cfg.SkipEmpty,
		skipSymLinks:  cfg.SkipSymLinks,
		minDuplicates: cfg.MinDuplicates,
	}
	if dmap.minDuplicates < 2 {
		dmap.minDuplicates = 2
	}
	// Initialize our map.
	dmap.filesMap = make(map[Digest][]string, mapInitSize)
	dsklog.Dlogger.Debug("Dmap created with initial size: ", mapInitSize)
	dsklog.Dlogger.Debug("Skipping empty files: ", cfg.SkipEmpty)
	dsklog.Dlogger.Debug("Skipping symbolic links: ", cfg.SkipSymLinks)

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

// TODO: Refactor ShowResults function that will display results in the various formats.
// PrintDmap will print entries currently stored in map in more text friendly way.
func (d *Dmap) PrintDmap() {
	for k, v := range d.filesMap {
		if uint(len(v)) < d.minDuplicates {
			continue
		}
		hash := fmt.Sprintf("%x", k)
		fmt.Printf("Hash: %s  \n ---> Files: \n", hash)
		for i, f := range v {
			fmt.Printf("\t%d: %s \n", i, f)
		}
		fmt.Println("--------------------------")
	}
}

// ShowResultsPretty will display duplicates held in our Dmap as
// a pretty tree.
// NOTE: Pterm takes a very long time to render this table for some reason.
//
//	The primary method of viewing the results is via the TUI.
func (d *Dmap) ShowResultsPretty() {

	// Banner
	var leveledList pterm.LeveledList

	for hash, files := range d.filesMap {
		// Only show files that have at least one other duplicate.
		if uint(len(files)) < d.minDuplicates {
			continue
		}
		// Our hash value will be our level 0 item from which all duplicate files
		// will be subitems.
		listItem := pterm.LeveledListItem{Level: 0, Text: fmt.Sprintf("%x", hash)}
		leveledList = append(leveledList, listItem)
		for _, f := range files {
			listItem = pterm.LeveledListItem{Level: 1, Text: f}
			leveledList = append(leveledList, listItem)
		}
	}

	root := putils.TreeFromLeveledList(leveledList)
	pterm.DefaultTree.WithRoot(root).Render()
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
