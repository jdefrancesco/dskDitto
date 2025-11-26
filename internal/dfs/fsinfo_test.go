package dfs

import (
	_ "fmt"
	"os"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dsklog"
)

// Need to initialize logger
func TestMain(m *testing.M) {
	dsklog.InitializeDlogger("/dev/null")
	os.Exit(m.Run())
}

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
func TestGetFileSize(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some data to the file
	data := []byte("hello world")
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test: valid file
	got := GetFileSize(tmpFile.Name())
	want := uint64(len(data))
	if got != want {
		t.Errorf("GetFileSize(%q) = %d, want %d", tmpFile.Name(), got, want)
	}

	// Test: empty file name
	got = GetFileSize("")
	if got != 0 {
		t.Errorf("GetFileSize(\"\") = %d, want 0", got)
	}

	// Test: non-existent file
	got = GetFileSize("non_existent_file.txt")
	if got != 0 {
		t.Errorf("GetFileSize(non_existent_file.txt) = %d, want 0", got)
	}
}
