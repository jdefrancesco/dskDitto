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
	"crypto/md5"
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
	fileName    string
	fileSize    int64
	fileMd5Hash string
}

// New creates a new Dfile.
func NewDfile(fName string, fSize int64) (*Dfile, error) {

	if fName == "" {
		fmt.Errorf("File name must be specified\n")
		return nil, errors.New("File name needs to be specified\n")
	}

	fullFileName, err := filepath.Abs(fName)
	if err != nil {
		fmt.Errorf("couldn't get absolute filename for %s\n", fName)
		return nil, err
	}

	d := &Dfile{
		fileName: fullFileName,
		fileSize: fSize,
	}

	if err = d.hashFile(); err != nil {
		fileLogger.Error().Err(err).Msgf("Failed to hash %s: error: %s\n", d.fileName, err)
		return d, errors.New("Failed to hash file")
	}

	return d, nil
}

// FileName will return the name of the file currently described by the dfile
func (d *Dfile) FileName() string { return d.fileName }

// BaseName returns the base filename only instead of the full pathname.
func (d *Dfile) BaseName() string { return filepath.Base(d.fileName) }

// FileSize will return the size of the file described by dfile object.
func (d *Dfile) FileSize() int64 { return d.fileSize }

// GetHash will return MD5 Hash string of file.
func (d *Dfile) Hash() string { return d.fileMd5Hash }

// concurrency-limiting semaphore to ensure MD5 hashing doesn't exhaust
// all the available file descriptors. macOS open file limit is ridiculously
// low; by default 256.
var sema = make(chan struct{}, OpenFileDescLimit)

// HashFile will MD5 hash our Dfile.
// Profiling has revealed this to be one of the bottlenecks,
// in the future seek to improve efficiency of this operation.
func (d *Dfile) hashFile() error {

	// Acquire token
	sema <- struct{}{}
	defer func() { <-sema }()

	f, err := os.Open(d.fileName)
	if err != nil {
		fmt.Errorf("Failed to read file %s", d.fileName)
		return err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		e := "error"
		fileLogger.Fatal().Str("error", e)
	}

	d.fileMd5Hash = fmt.Sprintf("%x", h.Sum(nil))
	return nil
}

// Simple debug function to show current Dfile fields.
func (d *Dfile) PrintDfile() {
	fileLogger.Info().Msgf("d.fileSize %d", d.fileSize)
	fileLogger.Info().Msgf("d.fileMd5Hash %s", d.fileMd5Hash)
	fileLogger.Info().Msgf("d.fileName %s", d.fileName)
}
