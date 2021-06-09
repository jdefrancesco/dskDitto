package dmap

import (
	"ditto/dfs"
	"fmt"
	"testing"
)

func TestNewDmap(t *testing.T) {

	dmap, err := NewDmap()
	if err != nil {
		t.Errorf("Couldn't create new dmap: %s", err)
	}

	var dfiles = []struct {
		fileName    string
		fileSize    int64
		fileMd5Hash string
	}{
		{"test_files/fileOne.bin", 100, "891656230863b3136a7bee17222cabc8"},
		{"test_files/fileTwo.bin", 3, "ef3f9ad0663a925c16b1ebcba033c269"},
		{"test_files/fileThree.bin", 0, "d41d8cd98f00b204e9800998ecf8427e"},
		{"test_files/fileFour.bin", 1, "05d85804dd3e689e1f1a0aaa1975fb4c"},
		{"test_files/fileFive.bin", 100, "891656230863b3136a7bee17222cabc8"},
	}

	for _, f := range dfiles {
		df, err := dfs.NewDfile(f.fileName)
		if err != nil {
			fmt.Errorf("Failed to read file %s: %v", f.fileName, err)
		}

		dmap.Add(df)
	}

	dmap.PrintDmap()

	size := dmap.MapSize()
	if size != 4 {
		t.Errorf("Size incorrect got %d\n", size)
	}

}
