package main

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rivo/tview"
)

func hashFile(filePath string) (string, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

func findDuplicates(rootDir string) map[string][]string {
	fileHashes := make(map[string][]string)

	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			hash, err := hashFile(path)
			if err != nil {
				return err
			}
			fileHashes[hash] = append(fileHashes[hash], path)
		}
		return nil
	})

	return fileHashes
}

func main() {
	app := tview.NewApplication()

	list := tview.NewList()
	duplicateFiles := findDuplicates("/Users/jo31816/code/dskDitto/dmap/test_files/")

	if len(duplicateFiles) == 0 {
		list.AddItem("No duplicates found.", "", 0, nil)
	}

	for hash, files := range duplicateFiles {
		if len(files) > 1 {
			for _, file := range files {
				list.AddItem(file, hash, rune(hash[0]), func() {
					// Here you could delete the file or perform other actions.
				})
			}
		}
	}

	list.SetSelectedFunc(func(i int, s string, r string, ru rune) {
		// Delete the file when selected.
		if err := os.Remove(s); err != nil {
			panic(err)
		}
		list.RemoveItem(i)
	})

	if err := app.SetRoot(list, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
