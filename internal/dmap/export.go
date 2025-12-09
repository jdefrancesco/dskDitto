package dmap

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
)

type exportFile struct {
	Path string `json:"path"`
	Size uint64 `json:"size"`
}

type exportGroup struct {
	Hash           string       `json:"hash"`
	DuplicateCount int          `json:"duplicate_count"`
	Files          []exportFile `json:"files"`
}

type exportSummary struct {
	GroupCount int           `json:"group_count"`
	Groups     []exportGroup `json:"groups"`
}

// WriteJSON writes duplicate groups that satisfy the minimum duplicate threshold to a JSON file.
func (d *Dmap) WriteJSON(path string) error {
	summary := d.collectExportSummary()
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	file, err := secureOutputFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write JSON file %s: %w", path, err)
	}
	return nil
}

// WriteCSV writes duplicate groups that satisfy the minimum duplicate threshold to a CSV file.
func (d *Dmap) WriteCSV(path string) error {
	summary := d.collectExportSummary()
	file, err := secureOutputFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"hash", "duplicate_count", "path", "size_bytes"}); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	for _, group := range summary.Groups {
		count := strconv.Itoa(group.DuplicateCount)
		for _, f := range group.Files {
			if err := writer.Write([]string{group.Hash, count, f.Path, strconv.FormatUint(f.Size, 10)}); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush CSV writer: %w", err)
	}
	return nil
}

// collectExportSummary builds an exportSummary containing deduplicated file groups,
// filtered by the minimum duplicate threshold, sorted by hash, and enriched with file sizes.
func (d *Dmap) collectExportSummary() exportSummary {
	if d == nil {
		return exportSummary{}
	}

	type groupData struct {
		hash  string
		files []string
	}

	groups := make([]groupData, 0, len(d.filesMap))
	for digest, files := range d.filesMap {
		if uint(len(files)) < d.minDuplicates {
			continue
		}
		groups = append(groups, groupData{
			hash:  fmt.Sprintf("%x", digest),
			files: append([]string(nil), files...),
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].hash < groups[j].hash
	})

	exportGroups := make([]exportGroup, 0, len(groups))
	for _, g := range groups {
		item := exportGroup{
			Hash:           g.hash,
			DuplicateCount: len(g.files),
			Files:          make([]exportFile, 0, len(g.files)),
		}
		for _, path := range g.files {
			item.Files = append(item.Files, exportFile{
				Path: path,
				Size: dfs.GetFileSize(path),
			})
		}
		exportGroups = append(exportGroups, item)
	}

	return exportSummary{
		GroupCount: len(exportGroups),
		Groups:     exportGroups,
	}
}

func secureOutputFile(path string) (*os.File, error) {
	if path == "" {
		return nil, errors.New("output path is empty")
	}

	clean := filepath.Clean(path)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return nil, fmt.Errorf("resolve output path %s: %w", path, err)
	}

	info, err := os.Stat(abs)
	if err == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("output path %s is a directory", abs)
		}
	}

	dirPath := filepath.Dir(abs)
	base := filepath.Base(abs)

	return openFileSecure(abs, dirPath, base)
}
