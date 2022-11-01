package dfs

import (
	"testing"
)

// Check if we can correctly enumerate disk information.
func TestListFileSystems(t *testing.T) {
	ListFileSystems()
}

func TestGetFileUidGid(t *testing.T) {
	test_files := []string{"./test_files/fileOne.bin",
		"./test_files/fileTwo.bin"}

	for _, test := range test_files {
		uid, gid := GetFileUidGid(test)
		if uid != 1000 || gid != 1000 {
			t.Errorf("uid incorrect (%v, %v)", uid, gid)
		}
	}
}
