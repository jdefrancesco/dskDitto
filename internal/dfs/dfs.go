package dfs

import (
	"crypto/sha256"
	"errors"
	"io"
	"path/filepath"

	"fmt"
	"os"
)

// For now this will be our max open-file descriptor limit. This value
// is used by hashing function semaphore logic.
const OpenFileDescLimMax = 8192

// Dfile structure will describe a given file. We
// only care about the few file properties that will
// allow us to detect a duplicate.
type Dfile struct {
	fileName string
	fileSize int64
	// SHA256 of file as raw bytes (more efficient than hex string)
	fileSHA256Hash [32]byte
}

// New creates a new Dfile.
func NewDfile(fName string, fSize int64) (*Dfile, error) {

	if fName == "" {
		fmt.Printf("File name must be specified\n")
		return nil, errors.New("file name needs to be specified")
	}

	fullFileName, err := filepath.Abs(fName)
	if err != nil {
		fmt.Printf("couldn't get absolute filename for %s\n", fName)
		return nil, err
	}

	d := &Dfile{
		fileName: fullFileName,
		fileSize: fSize,
	}

	if err = d.hashFileSHA256(); err != nil {
		return d, errors.New("failed to hash file")
	}

	return d, nil
}

// FileName will return the name of the file currently described by the dfile
func (d *Dfile) FileName() string { return d.fileName }

// BaseName returns the base filename only instead of the full pathname.
func (d *Dfile) BaseName() string { return filepath.Base(d.fileName) }

// FileSize will return the size of the file described by dfile object.
func (d *Dfile) FileSize() int64 { return d.fileSize }

// GetHash will return SHA256 Hash as byte array.
func (d *Dfile) Hash() [32]byte { return d.fileSHA256Hash }

// GetHashString will return SHA256 Hash as hex string for display purposes.
func (d *Dfile) HashString() string { return fmt.Sprintf("%x", d.fileSHA256Hash) }

// GetPerms will give us UNIX permissions we need to ensure we can access
// a file.
func (d *Dfile) GetPerms() string {
	return ""
}

var sema = make(chan struct{}, OpenFileDescLimMax)

// Compute SHA256 hash. This will be the primary hash used, MD5 will be removed.
func (d *Dfile) hashFileSHA256() error {

	// Acquire semaphore to prevent exhausting all our file descriptors.
	sema <- struct{}{}
	defer func() { <-sema }()

	f, err := os.Open(d.fileName)
	if err != nil {
		err = fmt.Errorf("failed to read file %s", d.fileName)
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		// TODO: Handle this more approriately
		return nil
	}

	// Copy the hash bytes directly into our fixed-size array
	copy(d.fileSHA256Hash[:], h.Sum(nil))
	return nil

}
