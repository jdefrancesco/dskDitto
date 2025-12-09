package dfs

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jdefrancesco/dskDitto/internal/dsklog"

	"lukechampine.com/blake3"
)

// For now this will be our max open-file descriptor limit. This value
// is used by hashing function semaphore logic.
// TODO: Make tuneable.
const OpenFileDescLimMax = 2048

// HashAlgorithm identifies which digest to use when hashing files.
type HashAlgorithm string

// XXX: Removal of Blake3 until I find issue with implementation or my usage.
const (
	HashSHA256 HashAlgorithm = "sha256"
	HashBLAKE3 HashAlgorithm = "blake3"
)

// Dfile structure will describe a given file. We
// only care about the few file properties that will
// allow us to detect a duplicate.
type Dfile struct {
	fileName string
	fileSize int64
	algo     HashAlgorithm
	fileHash [32]byte
}

// New creates a new Dfile.
func NewDfile(fName string, fSize int64, algo HashAlgorithm) (*Dfile, error) {

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
		algo:     algo,
	}

	if err = d.hashFile(); err != nil {
		return d, errors.New("failed to hash file")
	}

	dsklog.Dlogger.Debugf("Hash algorithm chosen is %s", d.algo)
	return d, nil
}

// FileName will return the name of the file currently described by the dfile
func (d *Dfile) FileName() string { return d.fileName }

// BaseName returns the base filename only instead of the full pathname.
func (d *Dfile) BaseName() string { return filepath.Base(d.fileName) }

// FileSize will return the size of the file described by dfile object.
func (d *Dfile) FileSize() int64 { return d.fileSize }

// Algorithm returns the hashing algorithm used to create this Dfile.
func (d *Dfile) Algorithm() HashAlgorithm { return d.algo }

// GetHash will return hash bytes as fixed-size array.
func (d *Dfile) Hash() [32]byte { return d.fileHash }

// GetHashString will return SHA256 Hash as hex string for display purposes.
func (d *Dfile) HashString() string { return fmt.Sprintf("%x", d.fileHash) }

// GetPerms will give us UNIX permissions we need to ensure we can access
// a file.
func (d *Dfile) GetPerms() string {
	return ""
}

// Semaphore that controls how many open file descriptors we can have at once..
var sema = make(chan struct{}, OpenFileDescLimMax)

// We want to re-use a pool of buffers to make things easier on GC. We hash files
// in quick succession. Instead of creating a new buffer for each file we can re-use what we have
// available.
var bufPool = sync.Pool{
	New: func() any {
		var arr [1 << 20]byte
		return &arr
	},
}

func (d *Dfile) hashFile() error {
	sema <- struct{}{}
	defer func() { <-sema }()

	bufPtr := bufPool.Get().(*[1 << 20]byte)
	defer bufPool.Put(bufPtr)

	f, err := os.Open(d.fileName)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", d.fileName, err)
	}
	defer f.Close()

	h, err := newHash(d.algo)
	if err != nil {
		return err
	}

	if _, err := io.CopyBuffer(h, f, bufPtr[:]); err != nil {
		return fmt.Errorf("failed to copy file %s into hash buffer for processing: %w", d.fileName, err)
	}

	sum := h.Sum(nil)
	copy(d.fileHash[:], sum)
	return nil
}

func newHash(algo HashAlgorithm) (hash.Hash, error) {
	switch algo {
	case HashBLAKE3:
		return blake3.New(32, nil), nil
	case HashSHA256, "":
		return sha256.New(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algo)
	}
}

// ParseHashAlgorithm returns the supported hash algorithm constant for the supplied string.
func ParseHashAlgorithm(name string) (HashAlgorithm, error) {
	switch HashAlgorithm(strings.ToLower(name)) {
	case HashSHA256, "":
		return HashSHA256, nil
	case HashBLAKE3:
		return HashBLAKE3, nil
	default:
		return "", fmt.Errorf("unsupported hash algorithm %q", name)
	}
}

// DetectFilesystem returns the filesystem type for the path.
func DetectFilesystem(path string) (string, error) {
	return detectFilesystem(path)
}
