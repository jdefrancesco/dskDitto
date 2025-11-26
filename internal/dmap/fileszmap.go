// DFileSize cache is a simple KV store of files and sizes we have seen so far. We won't
// need to hash any file without an entry in the map.
package dmap

import (
	"fmt"
)

const (
	kInitMapEntries = 1000
)

type DFileSizeCache struct {
	// A small store that keeps file sizes cached so we reference
	// it in order to decide if hashing the entire file is necessary.
	// i.e if file has size 100, the entry will be the file size as key
	// value the number of files with that size. If entry has more than one
	// file of specific size we may need to hash things or filter through
	// another heuristic.
	sizeMap map[uint64]uint64
}

func NewDFileSizeCache() *DFileSizeCache {
	fileCache := &DFileSizeCache{}

	fileCache.sizeMap = make(map[uint64]uint64)
	return fileCache
}

func (b *DFileSizeCache) displayMap() {
	fmt.Printf("%v+", b.sizeMap)
}
