//go:build !unix

package dwalk

import "os"

// fileIdentity is a no-op placeholder on non-Unix platforms where we don't
// currently attempt inode-based hardlink deduplication.
type fileIdentity struct{}

type fileMeta struct {
	size        int64
	mode        os.FileMode
	identity    fileIdentity
	hasIdentity bool
}

func statFile(path string) (fileMeta, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fileMeta{}, err
	}
	return fileMeta{
		size: info.Size(),
		mode: info.Mode(),
	}, nil
}
