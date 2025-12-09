//go:build unix

package dwalk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/config"
	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
)

// TestHardlinkDeduplicationUnix verifies that multiple hardlinks to the same
// inode are treated as a single file by the walker.
func TestHardlinkDeduplicationUnix(t *testing.T) {
	dsklog.InitializeDlogger("/dev/null")

	root := t.TempDir()
	orig := filepath.Join(root, "orig.txt")
	link := filepath.Join(root, "link.txt")

	if err := os.WriteFile(orig, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create original file: %v", err)
	}

	if err := os.Link(orig, link); err != nil {
		// If the platform does not support hard links for some reason, skip.
		t.Skipf("hard links not supported: %v", err)
	}

	cfg := config.Config{
		HashAlgorithm: dfs.HashSHA256,
		SkipVirtualFS: true,
		MaxDepth:      -1,
	}

	paths := collectRelativePaths(t, root, cfg)
	if len(paths) != 1 {
		t.Fatalf("expected hardlinked files to be treated as one; got %d paths: %v", len(paths), paths)
	}
}
