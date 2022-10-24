package dfs

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestNewDfile(t *testing.T) {
	var tests = []struct {
		fileName       string
		fileSize       int64
		fileSHA256Hash string
	}{
		{"test_files/fileOne.bin", 100, "3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61"},
		{"test_files/fileTwo.bin", 3, "f2e0e2beb73c21338a1dc872cd7b900c24c4547b6d9ae882e02bcd4257ac7bd4"},
		{"test_files/fileThree.bin", 0, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"test_files/fileFour.bin", 1, "5ee0dd4d4840229fab4a86438efbcaf1b9571af94f5ace5acc94de19e98ea9ab"},
	}

	for _, test := range tests {
		df, err := NewDfile(test.fileName, test.fileSize)
		if err != nil {
			fmt.Errorf("Failed to read file %s: %v", test.fileName, err)
		}
		df.PrintDfile()

		fileSize := df.FileSize()
		fileName := df.FileName()
		fileBaseName := df.BaseName()
		fileHash := df.Hash()

		testBaseFileName := filepath.Base(test.fileName)

		testFullFileName, err := filepath.Abs(test.fileName)
		if err != nil {
			t.Errorf("filepath.Base() error: %s\n", err)
		}

		if testFullFileName != fileName {
			t.Errorf("testFullFileName want = %s, got = %s\n", testFullFileName, fileName)
		}

		if testBaseFileName != fileBaseName {
			t.Errorf("t.fileName want = %s, got = %s\n", testBaseFileName, fileBaseName)
		}

		if test.fileSize != fileSize {
			t.Errorf("t.fileSize want = %d, got = %d\n", test.fileSize, fileSize)
		}

		if test.fileSHA256Hash != fileHash {
			t.Errorf("t.fileSHA256Hash want = %s, got = %s\n", test.fileSHA256Hash, fileHash)
		}
	}

}
