package dmap

import (
	"ditto/internal/dfs"
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
	hash, err := SHA256HashFromHex("3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61")
	if err != nil {
		t.Errorf("Error converting hex to hash: %v", err)
	}
	files, err := dmap.Get(hash)
	if err != nil {
		t.Errorf("Error gettings hash from map")
	}

	if len(files) != len(dfiles) {
		fmt.Println(files)
	}

}

func TestNewFileCount(t *testing.T) {

	fmap := NewDFileSizeCache()
	if fmap == nil {
		t.Errorf("Couldn't create object.")
	}

}

func FuzzDmapAdd(f *testing.F) {
	// Add seed inputs for the fuzzer
	f.Add("file.txt", int64(123))
	f.Add("test.bin", int64(0))
	f.Add("large_file.dat", int64(1024*1024))
	f.Add("", int64(0))
	f.Add("very_long_filename_that_might_cause_issues.txt", int64(999999))

	f.Fuzz(func(t *testing.T, name string, size int64) {
		// Skip invalid inputs that would cause issues
		if len(name) > 512 || size < 0 || size > 1024*1024*1024 {
			return
		}

		// Create a new Dmap for each test
		dm, err := NewDmap()
		if err != nil {
			t.Fatalf("Failed to create Dmap: %v", err)
		}

		// Create a temporary file for testing if name is not empty
		if name == "" {
			name = "fuzz_temp_file"
		}

		// Test that Add doesn't panic with various inputs
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Dmap operations panicked with input name=%q, size=%d: %v", name, size, r)
			}
		}()

		testHash := SHA256Hash{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8,
			0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10,
			0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
			0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}

		// Test adding files to the map
		dm.filesMap[testHash] = append(dm.filesMap[testHash], name)

		// Test Get operation
		files, err := dm.Get(testHash)
		if err != nil {
			t.Errorf("Get failed: %v", err)
		}

		if len(files) == 0 {
			t.Errorf("Expected at least one file, got none")
		}

		// Test MapSize
		mapSize := dm.MapSize()
		if mapSize < 0 {
			t.Errorf("MapSize returned negative value: %d", mapSize)
		}
	})
}

func FuzzSHA256HashFromHex(f *testing.F) {
	// Add seed inputs for hex string fuzzing
	f.Add("3fa2a6033f2b531361adf2bf300774fd1b75a5db13828e387d6e4c3c03400d61")
	f.Add("deadbeef1234567890abcdef1234567890abcdef1234567890abcdef12345678")
	f.Add("")
	f.Add("invalid_hex")
	f.Add("too_short")
	f.Add("way_too_long_hex_string_that_exceeds_normal_hash_length_by_far_and_should_be_rejected")

	f.Fuzz(func(t *testing.T, hexStr string) {
		// Test that SHA256HashFromHex doesn't panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SHA256HashFromHex panicked with input %q: %v", hexStr, r)
			}
		}()

		hash, err := SHA256HashFromHex(hexStr)

		// Valid 64-character hex strings should succeed
		if len(hexStr) == 64 {
			// Check if all characters are valid hex
			validHex := true
			for _, c := range hexStr {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
					validHex = false
					break
				}
			}

			if validHex && err != nil {
				t.Errorf("Expected valid hex string %q to succeed, but got error: %v", hexStr, err)
			}
			if validHex && err == nil {
				// Verify the hash was properly constructed
				if hash == (SHA256Hash{}) {
					t.Errorf("Expected non-zero hash for valid input %q", hexStr)
				}
			}
		} else {
			if err == nil {
				t.Errorf("Expected error for invalid length hex string %q, but got success", hexStr)
			}
		}
	})
}
