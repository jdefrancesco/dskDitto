// Implement our primary data structure Dmap.
//
// Dmap will be a hash map with roughly the following simple structure:
//
// { MD5_Key --> [file, fileClone1, fileClone2, etc...]}
//
// That is, MD5 hash of file will serve as our hash map key, which maps to a simple list of file names.
// MD5 will be used for the time being, mainly for the slight speed advantage.
package dmap

type Md5String string

type Dmap struct {
	filesMap  map[Md5String][]string
	fileCount uint64
}

func NewDmap() (*Dmap, error) {

}
