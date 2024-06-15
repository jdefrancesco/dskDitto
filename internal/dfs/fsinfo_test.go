package dfs

import (
	"testing"
    "os"
    _ "fmt"
)

// Check if we can correctly enumerate disk information.
func TestListFileSystems(t *testing.T) {
	ListFileSystems()
}

func TestGetFileUidGid(t *testing.T) {
	test_files := []string{"./test_files/fileOne.bin",
		"./test_files/fileTwo.bin"}

    curr_uid := os.Getuid()
	// XXX: Need to fix this test and get UID from env variable
	for _, test := range test_files {
		uid, _ := GetFileUidGid(test)
		if uid != curr_uid {
			t.Errorf("uid incorrect (%v)", uid)
		}
	}
}
