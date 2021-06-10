package dfs

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestNewDfile(t *testing.T) {
	var tests = []struct {
		fileName    string
		fileSize    int64
		fileMd5Hash string
	}{
		{"test_files/fileOne.bin", 100, "891656230863b3136a7bee17222cabc8"},
		{"test_files/fileTwo.bin", 3, "ef3f9ad0663a925c16b1ebcba033c269"},
		{"test_files/fileThree.bin", 0, "d41d8cd98f00b204e9800998ecf8427e"},
		{"test_files/fileFour.bin", 1, "05d85804dd3e689e1f1a0aaa1975fb4c"},
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

		if test.fileMd5Hash != fileHash {
			t.Errorf("t.fileMd5Hash want = %s, got = %s\n", test.fileMd5Hash, fileHash)
		}
	}

}
