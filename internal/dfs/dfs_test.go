package dfs

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDfile(t *testing.T) {
	var tests = []struct {
		fileName string
		fileSize int64
		fileHash string
	}{
		{"test_files/fileOne.bin", 100, "3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61"},
		{"test_files/fileTwo.bin", 3, "f2e0e2beb73c21338a1dc872cd7b900c24c4547b6d9ae882e02bcd4257ac7bd4"},
		{"test_files/fileThree.bin", 0, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"test_files/fileFour.bin", 1, "5ee0dd4d4840229fab4a86438efbcaf1b9571af94f5ace5acc94de19e98ea9ab"},
	}

	for _, test := range tests {
		df, err := NewDfile(test.fileName, test.fileSize, HashSHA256)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", test.fileName, err)
		}

		fileSize := df.FileSize()
		fileName := df.FileName()
		fileBaseName := df.BaseName()
		fileHash := df.HashString() // Use HashString() for comparison with hex string

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

		if test.fileHash != fileHash {
			t.Errorf("t.fileHash want = %s, got = %s\n", test.fileHash, fileHash)
		}
	}

}

func TestNoCacheHashOptionsMatchDefaultHashing(t *testing.T) {
	data := bytes.Repeat([]byte("dskditto"), sampleChunkSize)
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	defaultDfile, err := NewDfile(path, int64(len(data)), HashSHA256)
	if err != nil {
		t.Fatalf("default NewDfile failed: %v", err)
	}
	noCacheDfile, err := NewDfileWithOptions(path, int64(len(data)), HashSHA256, HashOptions{NoCache: true})
	if err != nil {
		t.Fatalf("no-cache NewDfile failed: %v", err)
	}
	if defaultDfile.Hash() != noCacheDfile.Hash() {
		t.Fatalf("no-cache full hash differs from default")
	}

	defaultSample, err := HashFileSample(path, int64(len(data)), HashSHA256)
	if err != nil {
		t.Fatalf("default sample failed: %v", err)
	}
	noCacheSample, err := HashFileSampleWithOptions(path, int64(len(data)), HashSHA256, HashOptions{NoCache: true})
	if err != nil {
		t.Fatalf("no-cache sample failed: %v", err)
	}
	if defaultSample != noCacheSample {
		t.Fatalf("no-cache sample differs from default")
	}
}

func TestScopedHashOpenRejectsEscapingSymlink(t *testing.T) {
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.bin")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	scopedDir := t.TempDir()
	linkPath := filepath.Join(scopedDir, "link.bin")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink creation unsupported: %v", err)
	}

	if _, err := HashFileSample(linkPath, int64(len("outside")), HashSHA256); err == nil {
		t.Fatalf("HashFileSample followed symlink outside scoped parent")
	}

	if _, err := NewDfile(linkPath, int64(len("outside")), HashSHA256); err == nil {
		t.Fatalf("NewDfile followed symlink outside scoped parent")
	}
}

func TestHashFileSampleEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	sample, err := HashFileSample(path, 0, HashSHA256)
	if err != nil {
		t.Fatalf("HashFileSample failed: %v", err)
	}
	if !sample.CoversWholeFile {
		t.Fatalf("empty-file sample should cover the whole file")
	}

	if want := sha256.Sum256(nil); sample.Digest != want {
		t.Fatalf("empty-file sample digest mismatch")
	}
}
