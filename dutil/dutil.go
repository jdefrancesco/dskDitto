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
package dutil

import (
	"crypto/md5"
	"errors"
	"io"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.LevelDebug)
}

type Dfile struct {
	fileName    string
	fileSize    uint64
	fileMd5Hash string
}

// New creates a new Dfile.
func New(fName string) (*Dfile, error) {

	if fName == "" {
		fmt.Errorf("File name must be specified\n")
		return nil, errors.New("File name needs to be specified\n")
	}

	d := &Dfile{
		fileName: fName,
	}

	// First get file size without opening the file
	f, err := os.Stat(fName)
	if err != nil {
		fmt.Errorf("Failed to call Stat on file %s\n", fName)
		d.fileSize = 0
		return d
	}

	// If file size isn't zero, grab hash of file.
	d.fileSize = f.Size()
	if d.fileSize {
		err := d.hashFile()
		if err != nil {
			fmt.Errorf("Failed to hash file %s\n", fName)
			d.fileMd5Hash = "FAILED"
		}
	}

	return d
}

// FileSize will return the size of the file described by dfile object.
func (d *Dfile) GetFileSize() uint64 { return d.fileSize }

// GetHash will return MD5 Hash string of file.
func (d *Dfile) GetHash() string { return d.fileMd5Hash }

// HashFile will MD5 hash our Dfile.
func (d *Dfile) hashFile() error {

	if d.fileSize == 0 {
		return error.New("File size is zero")
	}

	data, err := ioutil.ReadFile(d.fileName)
	if err != nil {
		fmt.Errorf("Failed to read file %s", d.fileName)
		return err
	}

	d.fileMd5Hash = md5.Sum(data)
	log.WithFields(log.Fields{
		"hash": d.fileMd5Hash,
	}).Debug("Hash of file")

	return nil
}

// Simple debug function to show current Dfile fields.
func (d *Dfile) PrintDfile() {
	log.WithFields(log.Fields{
		"fileName": d.fileName,
		"fileSize", d.fileSize,
		"fileMd5Hash": d.fileMd5Hash,
	}).Debug("Dfile information")
}
