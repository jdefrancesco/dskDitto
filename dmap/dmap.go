// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { MD5_Key --> [fileClone1, fileClone2, etc...]}
//
// That is, MD5 hash of file will serve as our hash map key, which maps to a simple list of file names.
// MD5 will be used for the time being, mainly for the slight speed advantage.
package dmap

import (
	"ditto/dfs"
	"fmt"
)

type Md5Hash string

// Dmap structure will hold our file duplication data.
// It is the primary data structure that will house the results
// that will eventually be returned to the user.
type Dmap struct {
	filesMap  map[Md5Hash][]string
	fileCount uint
}

// NewDmap returns a new Dmap structure.
func NewDmap() (*Dmap, error) {

	dmap := &Dmap{
		fileCount: 0,
	}
	// Initialize our map.
	dmap.filesMap = make(map[Md5Hash][]string)

	return dmap, nil
}

// Add will take a dfile and add it the map.
func (d *Dmap) Add(dfile *dfs.Dfile) {
	d.filesMap[Md5Hash(dfile.Hash())] = append(d.filesMap[Md5Hash(dfile.Hash())], dfile.FileName())
	d.fileCount++
}

// PrintDmap will print entries currently stored in map.
func (d *Dmap) PrintDmap() {
	for k, v := range d.filesMap {
		fmt.Printf("Hash: %s  Files: \n", k)
		for i, f := range v {
			fmt.Printf("\t%d: %s \n", i, f)
		}
		fmt.Println("--------------------------")
	}
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
func (d *Dmap) Get(hash Md5Hash) (files []string, err error) {
	res, ok := d.filesMap[hash]
	if !ok {
		return []string{}, err
	}

	return res, nil
}
