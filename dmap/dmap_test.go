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
		fileName       string
		fileSize       int64
		fileSHA256Hash string
	}{
		{"test_files/fileOne.bin", 101, "3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61"},
		{"test_files/fileTwo.bin", 3, "f2e0e2beb73c21338a1dc872cd7b900c24c4547b6d9ae882e02bcd4257ac7bd4"},
		{"test_files/fileThree.bin", 0, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"test_files/fileFour.bin", 1, "5ee0dd4d4840229fab4a86438efbcaf1b9571af94f5ace5acc94de19e98ea9ab"},
	}

	for _, f := range dfiles {
		df, err := dfs.NewDfile(f.fileName, f.fileSize)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", f.fileName, err)
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
	files, err := dmap.Get("3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61")
	if err != nil {
		t.Errorf("Error gettings hash from map")
	}

	if len(files) != len(dfiles) {
		fmt.Println(files)
	}

}
