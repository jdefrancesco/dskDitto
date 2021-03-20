package dutil

import (
	"testing"
)

func TestNewDfile(t *testing.T) {
	var tests = []struct {
		fileName    string
		fileSize    uint64
		fileMd5Hash string
	}{
		{"test_files/fileOne.bin", 100, "891656230863b3136a7bee17222cabc8"},
		{"test_files/fileTwo.bin", 3, "ef3f9ad0663a925c16b1ebcba033c269"},
		{"test_files/fileThree.bin", 0, "d41d8cd98f00b204e9800998ecf8427e"},
		{"test_files/fileFour.bin", 1, "05d85804dd3e689e1f1a0aaa1975fb4c"},
	}

	for _, t := range tests {

	}

}
