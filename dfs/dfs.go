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
	"path/filepath"

	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
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
func NewDfile(fName string) (*Dfile, error) {

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
	}

	// First get file size without opening the file
	f, err := os.Stat(fName)
	if err != nil {
		fmt.Errorf("Failed to call Stat on file %s\n", fName)
		d.fileSize = 0
		return d, errors.New("Failed to call Stat on file")
	}

	// If file size isn't zero, grab hash of file.
	d.fileSize = f.Size()
	err = d.hashFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash %s", d.fileName)
		return d, errors.New("Failed to hash file")
	}

	return d, nil
}

// FileName will return the name of the file currently described by the dfile
func (d *Dfile) FileName() string { return d.fileName }

// FileSize will return the size of the file described by dfile object.
func (d *Dfile) FileSize() int64 { return d.fileSize }

// GetHash will return MD5 Hash string of file.
func (d *Dfile) Hash() string { return d.fileMd5Hash }

// HashFile will MD5 hash our Dfile.
func (d *Dfile) hashFile() error {

	f, err := os.Open(d.fileName)
	if err != nil {
		fmt.Errorf("Failed to read file %s", d.fileName)
		return err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		e := "error"
		log.Fatal().Str("error", e)
	}

	d.fileMd5Hash = fmt.Sprintf("%x", h.Sum(nil))
	return nil
}

// Simple debug function to show current Dfile fields.
func (d *Dfile) PrintDfile() {
	log.Info().Msgf("d.fileSize %d", d.fileSize)
	log.Info().Msgf("d.fileMd5Hash %s", d.fileMd5Hash)
	log.Info().Msgf("d.fileName %s", d.fileName)
}
