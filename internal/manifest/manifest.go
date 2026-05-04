// manifest helps support file restore funnctionality. If user clobbers files
// by accident. They can simply restore them by providing the generated file
// that holds restore information.
//
// The restore file is dumped as JSONL for simplicity. The primary logic for
// creating the file or reading for restoration are in: writer.go, reader.go
package manifest

import (
	"errors"
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

func NewEntry(groupID uint64, algo dfs.HashAlgorithm, hash, canonicalPath, restorePath string) (Entry, error) {
	if hash == "" {
		return Entry{}, errors.New("manifest hash is empty")
	}
	if canonicalPath == "" {
		return Entry{}, errors.New("canonical path is empty")
	}
	if restorePath == "" {
		return Entry{}, errors.New("restore path is empty")
	}

	canonicalAbs, err := filepath.Abs(filepath.Clean(canonicalPath))
	if err != nil {
		return Entry{}, fmt.Errorf("resolve canonical path %s: %w", canonicalPath, err)
	}
	restoreAbs, err := filepath.Abs(filepath.Clean(restorePath))
	if err != nil {
		return Entry{}, fmt.Errorf("resolve restore path %s: %w", restorePath, err)
	}

	canonicalInfo, err := os.Stat(canonicalAbs)
	if err != nil {
		return Entry{}, fmt.Errorf("stat canonical %s: %w", canonicalAbs, err)
	}
	if !canonicalInfo.Mode().IsRegular() {
		return Entry{}, fmt.Errorf("canonical path is not a regular file: %s", canonicalAbs)
	}

	restoreInfo, err := os.Lstat(restoreAbs)
	if err != nil {
		return Entry{}, fmt.Errorf("stat restore path %s: %w", restoreAbs, err)
	}
	if !restoreInfo.Mode().IsRegular() && restoreInfo.Mode()&os.ModeSymlink == 0 {
		return Entry{}, fmt.Errorf("restore path is not a file or symlink: %s", restoreAbs)
	}

	dev, ino := fileIdentity(restoreInfo)
	mt := restoreInfo.ModTime()
	return Entry{
		Version:     ManifestVersion,
		GroupID:     groupID,
		HashAlgo:    string(algo),
		Hash:        hash,
		Size:        canonicalInfo.Size(),
		Canonical:   canonicalAbs,
		RestorePath: restoreAbs,
		Mode:        uint32(restoreInfo.Mode().Perm()),
		ModTimeUnix: mt.Unix(),
		ModTimeNsec: int64(mt.Nanosecond()),
		Dev:         dev,
		Ino:         ino,
	}, nil
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
		for _, dupPath := range g.paths[1:] {
			entry, err := NewEntry(groupID, algo, g.hash, canonical, dupPath)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
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

func AppendUnique(path string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}

	existing, err := Read(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		existing = nil
	}

	return Write(path, MergeEntriesByRestorePath(existing, entries))
}

func MergeEntriesByRestorePath(existing, incoming []Entry) []Entry {
	merged := make([]Entry, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))

	appendUnique := func(entries []Entry) {
		for _, entry := range entries {
			if entry.RestorePath == "" {
				continue
			}
			if _, ok := seen[entry.RestorePath]; ok {
				continue
			}
			seen[entry.RestorePath] = struct{}{}
			merged = append(merged, entry)
		}
	}

	appendUnique(existing)
	appendUnique(incoming)
	return merged
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
