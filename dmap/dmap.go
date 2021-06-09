// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { MD5_Key --> [file, fileClone1, fileClone2, etc...]}
//
// That is, MD5 hash of file will serve as our hash map key, which maps to a simple list of file names.
// MD5 will be used for the time being, mainly for the slight speed advantage.
package dmap

import (
	"ditto/dfs"
	"fmt"
)

type Md5Hash string

type Dmap struct {
	filesMap  map[Md5Hash][]string
	fileCount uint64
}

// NewDmap returns a new Dmap structure.
func NewDmap() (*Dmap, error) {

	dmap := &Dmap{
		fileCount: 0,
	}
	dmap.filesMap = make(map[Md5Hash][]string)

	return dmap, nil
}

// Add will take a dfile and add it the map.
func (d *Dmap) Add(dfile *dfs.Dfile) {
	d.filesMap[Md5Hash(dfile.Hash())] = append(d.filesMap[Md5Hash(dfile.Hash())], dfile.FileName())
}

// PrintDmap will print entries currently stored in map.
func (d *Dmap) PrintDmap() {
	for k, v := range d.filesMap {
		fmt.Println("hash: ", k, "files: ", v)
	}
}

// MapSize returns number of entries in the map.
func (d *Dmap) MapSize() int {
	return len(d.filesMap)
}
