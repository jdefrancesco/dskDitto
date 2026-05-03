// manifest helps support file restore funnctionality. If user clobbers files
// by accident. They can simply restore them by providing the generated file
// that holds restore information.
//
// The restore file is dumped as JSONL for simplicity. The primary logic for
// creating the file or reading for restoration are in: writer.go, reader.go
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
)

const ManifestVersion = 1

// Try to keep as much metadata for file as we can
// for robustness. Not all attributes exist for every
// potential file system. These should be omitted.
type Entry struct {
	Version     int    `json:"version"`
	GroupID     uint64 `json:"group_id"`
	HashAlgo    string `json:"hash_algo"`
	Hash        string `json:"hash"`
	Size        int64  `json:"size"`
	Canonical   string `json:"canonical"`
	RestorePath string `json:"restore_path"`
	Mode        uint32 `json:"mode,omitempty"`
	ModTimeUnix int64  `json:"mtime_unix,omitempty"`
	ModTimeNsec int64  `json:"mtime_nsec,omitempty"`
	Dev         uint64 `json:"dev,omitempty"`
	Ino         uint64 `json:"ino,omitempty"`
}

// RestoreOptions config info
type RestoreOptions struct {
	DryRun       bool
	Overwrite    bool
	VerifyHash   bool
	RestoreMode  bool
	RestoreMTime bool
}

type group struct {
	hash   string
	digest dmap.Digest
	paths  []string
}

func EntriesFromDmap(dm *dmap.Dmap, algo dfs.HashAlgorithm) ([]Entry, error) {
	if dm == nil {
		return nil, nil
	}

	minDups := dm.MinDuplicates()
	if minDups < 2 {
		minDups = 2
	}

	groups := make([]group, 0, dm.MapSize())
	for digest, files := range dm.GetMap() {
		if uint(len(files)) < minDups {
			continue
		}
		absPaths, err := normalizeAndSortPaths(files)
		if err != nil {
			return nil, err
		}
		if len(absPaths) < 2 {
			continue
		}
		groups = append(groups, group{
			hash:   fmt.Sprintf("%x", digest),
			digest: digest,
			paths:  absPaths,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].hash < groups[j].hash
	})

	entries := make([]Entry, 0)
	for i, g := range groups {
		groupID := uint64(i + 1)
		canonical := g.paths[0]
		canonicalInfo, err := os.Stat(canonical)
		if err != nil {
			return nil, fmt.Errorf("stat canonical %s: %w", canonical, err)
		}

		for _, dupPath := range g.paths[1:] {
			dupInfo, err := os.Stat(dupPath)
			if err != nil {
				return nil, fmt.Errorf("stat duplicate %s: %w", dupPath, err)
			}

			dev, ino := fileIdentity(dupInfo)
			mt := dupInfo.ModTime()
			entries = append(entries, Entry{
				Version:     ManifestVersion,
				GroupID:     groupID,
				HashAlgo:    string(algo),
				Hash:        g.hash,
				Size:        canonicalInfo.Size(),
				Canonical:   canonical,
				RestorePath: dupPath,
				Mode:        uint32(dupInfo.Mode().Perm()),
				ModTimeUnix: mt.Unix(),
				ModTimeNsec: int64(mt.Nanosecond()),
				Dev:         dev,
				Ino:         ino,
			})
		}
	}

	return entries, nil
}

func Write(path string, entries []Entry) error {
	writer, err := NewWriter(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := writer.WriteEntry(entry); err != nil {
			_ = writer.Abort()
			return err
		}
	}
	return writer.Close()
}

func CanonicalizeDmapGroups(dm *dmap.Dmap) error {
	if dm == nil {
		return nil
	}
	for digest, files := range dm.GetMap() {
		normalized, err := normalizeAndSortPaths(files)
		if err != nil {
			return err
		}
		dm.GetMap()[digest] = normalized
	}
	return nil
}

func normalizeAndSortPaths(paths []string) ([]string, error) {
	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve path %s: %w", path, err)
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		normalized = append(normalized, abs)
	}
	sort.Strings(normalized)
	return normalized, nil
}
