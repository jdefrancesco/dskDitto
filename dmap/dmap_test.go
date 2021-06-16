package dmap

import (
	"ditto/dfs"
	"fmt"
	"testing"
)

// Test Dmap type.. Eventually I should make these tests far more robust.
// For now, lets just get things working so I can see all the pieces in place.
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
		// absFileName := filepath.Join(dir, f.Name())
		// Create new Dfile for file entry.
		df, err := dfs.NewDfile(f.fileName, f.fileSize)
		if err != nil {
			fmt.Errorf("Failed to read file %s: %v", f.fileName, err)
		}

		dmap.Add(df)
	}

	dmap.PrintDmap()

	size := dmap.MapSize()
	// Size should be four because we have one duplciate entry.
	if size != 4 {
		t.Errorf("Size incorrect got %d\n", size)
	}

	fmt.Println("Testing dmap.Get()")
	files, err := dmap.Get("891656230863b3136a7bee17222cabc8")
	if err != nil {
		t.Errorf("Error gettings hash from map")
	}

	if len(files) != len(dfiles) {
		fmt.Println(files)
	}

}
