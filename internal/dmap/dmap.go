// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { SHA256Hash --> [fileClone1, fileClone2, etc...]}
//
// That is, MD5 hash of file will serve as our hash map key, which maps to a simple list of file names.
// MD5 will be used for the time being, mainly for the slight speed advantage.
package dmap

import (
	"ditto/internal/dfs"
	"encoding/hex"
	"fmt"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

type SHA256Hash [32]byte

// SHA256HashFromHex converts a hex string to SHA256Hash
func SHA256HashFromHex(hexStr string) (SHA256Hash, error) {
	var hash SHA256Hash
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

const mapInitSize = 256

// Dmap structure will hold our file duplication data.
// It is the primary data structure that will house the results
// that will eventually be returned to the user.
// TODO: Add ClusterCount functions
type Dmap struct {
	filesMap map[SHA256Hash][]string

	// Files deffered for reasons such as size are stored here for later processing.
	deferredFiles []string
	// Number of files in our map.
	fileCount    uint
	clusterCount uint // Number of files dup clusters.
}

// NewDmap returns a new Dmap structure.
func NewDmap() (*Dmap, error) {

	dmap := &Dmap{
		fileCount: 0,
	}
	// Initialize our map.
	dmap.filesMap = make(map[SHA256Hash][]string, mapInitSize)

	return dmap, nil
}

// Add will take a dfile and add it the map.
func (d *Dmap) Add(dfile *dfs.Dfile) {
	hash := SHA256Hash(dfile.Hash())
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
		if len(v) < 2 {
			continue
		}
		fmt.Printf("Hash: %s  \n ---> Files: \n", k)
		for i, f := range v {
			fmt.Printf("\t%d: %s \n", i, f)
		}
		fmt.Println("--------------------------")
	}
}

// ShowResults will display duplicates held in our Dmap as
// a pretty tree.
func (d *Dmap) ShowResults() {

	// Banner
	var leveledList pterm.LeveledList

	for hash, files := range d.filesMap {
		// Only show files that have at least one other duplicate.
		if len(files) < 2 {
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
func (d *Dmap) ShowAllResults() {

	var bl []pterm.BulletListItem
	for hash, files := range d.filesMap {

		if len(files) < 2 {
			continue
		}
		pterm.Println(pterm.Green("Hash: ") + pterm.Cyan(hash))
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
func (d *Dmap) Get(hash SHA256Hash) (files []string, err error) {
	res, ok := d.filesMap[hash]
	if !ok {
		return []string{}, err
	}

	return res, nil
}

// GetMap will return the map.
func (d *Dmap) GetMap() map[SHA256Hash][]string {
	return d.filesMap
}
