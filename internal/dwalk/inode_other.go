//go:build !unix

package dwalk

import "os"

// fileIdentity is a no-op placeholder on non-Unix platforms where we don't
// currently attempt inode-based hardlink deduplication.
type fileIdentity struct{}

func getFileIdentity(info os.FileInfo) (fileIdentity, bool) {
	return fileIdentity{}, false
}
