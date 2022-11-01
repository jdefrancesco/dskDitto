/* Package dutil implements simple file operation functions.
 *
 * File operations dutil aims to fulfill may include things such as:
 * 	a. Getting size of file
 *  b. Getting type of file
 *  c. Reading a files contents
 *  d. etc...
 *
 * The file type Dfile is also defined. This serves as a file descriptor storing
 * important information about files.
 */
package dfs

import (
	"crypto/sha256"
	"errors"
	"io"
	"io/ioutil"
	"path/filepath"

	"fmt"
	"os"

	"github.com/rs/zerolog"
)

const OpenFileDescLimit = 100

var fileLogger zerolog.Logger

func init() {
	// zerofileLogger.SetGlobalLevel(zerofileLogger.InfoLevel)
	// fileLogger.Logger = fileLogger.Output(zerofileLogger.ConsoleWriter{Out: os.Stderr})
	tmpFile, err := ioutil.TempFile(os.TempDir(), "dskditto-dfs")
	if err != nil {
		fmt.Printf("Error creating log file\n")
	}
	fileLogger = zerolog.New(tmpFile).With().Logger()
	fileLogger.Info().Msg("DskDitto Log:")

}

// Dfile structure will describe a given file. We
// only care about the few file properties that will
// allow us to detect a duplicate.
type Dfile struct {
	fileName string
	fileSize int64
	// SHA256 of file. Removing MD5 code.
	fileSHA256Hash string
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
		fileLogger.Error().Err(err).Msgf("Failed to hash %s: error: %s\n", d.fileName, err)
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

// GetHash will return SHA256 Hash string of file.
func (d *Dfile) Hash() string { return d.fileSHA256Hash }

// GetPerms will give us UNIX permissions we need to ensure we can access
// a file.
func (d *Dfile) GetPerms() string {
	return ""
}

// concurrency-limiting semaphore to ensure MD5 hashing doesn't exhaust
// all the available file descriptors. macOS open file limit is ridiculously
// low; by default 256.
var sema = make(chan struct{}, OpenFileDescLimit)

// Computer SHA256 hash. This will be the primary hash used, MD5 will be removed.
func (d *Dfile) hashFileSHA256() error {

	// Acquire semaphore to prevent exhausting all our file descriptors.
	sema <- struct{}{}
	defer func() { <-sema }()

	f, err := os.Open(d.fileName)
	if err != nil {
		err = fmt.Errorf("Failed to read file %s", d.fileName)
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		e := "error"
		fileLogger.Fatal().Str("error", e)
	}

	d.fileSHA256Hash = fmt.Sprintf("%x", h.Sum(nil))
	return nil

}

// Simple debug function to show current Dfile fields.
func (d *Dfile) PrintDfile() {
	fileLogger.Info().Msgf("d.fileSize %d", d.fileSize)
	fileLogger.Info().Msgf("d.fileSHA256Hash %s", d.fileSHA256Hash)
	fileLogger.Info().Msgf("d.fileName %s", d.fileName)
}
