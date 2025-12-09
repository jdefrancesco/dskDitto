//go:build !unix

package dmap

import (
	"fmt"
	"os"
)

func openFileSecure(absPath, dirPath, fileName string) (*os.File, error) {
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open output file %s: %w", absPath, err)
	}
	return file, nil
}
