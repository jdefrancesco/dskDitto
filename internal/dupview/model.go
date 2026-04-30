package dupview

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/pkg/utils"
)

type FileStatus int

const (
	FileStatusPending FileStatus = iota
	FileStatusDeleted
	FileStatusLinked
	FileStatusError
)

type Action int

const (
	ActionDelete Action = iota
	ActionLink
)

type SortMode int

const (
	SortByTotalSize SortMode = iota
	SortByCount
)

const SortModeCount = 2

type FileEntry struct {
	Path    string
	Marked  bool
	Status  FileStatus
	Message string
}

type Group struct {
	Hash     dmap.Digest
	Title    string
	Files    []*FileEntry
	Expanded bool
	TotalSz  uint64
}

type Model struct {
	Groups        []*Group
	SortMode      SortMode
	MinDuplicates uint
	Result        string
}

func New(dMap *dmap.Dmap) *Model {
	m := &Model{
		SortMode: SortByTotalSize,
	}

	if dMap == nil {
		m.MinDuplicates = 2
		return m
	}

	m.MinDuplicates = dMap.MinDuplicates()
	for hash, files := range dMap.GetMap() {
		if uint(len(files)) < m.MinDuplicates {
			continue
		}

		totalSize := EstimateGroupTotalSize(files)
		group := &Group{
			Hash:     hash,
			Title:    FormatGroupTitle(hash, len(files), totalSize),
			Expanded: true,
			TotalSz:  totalSize,
		}

		for _, file := range files {
			group.Files = append(group.Files, &FileEntry{Path: file})
		}

		AutoMarkGroup(group)
		m.Groups = append(m.Groups, group)
	}

	m.SortGroups()
	return m
}

func (m *Model) SetSortMode(mode SortMode) {
	if m == nil {
		return
	}
	if mode < 0 || int(mode) >= SortModeCount {
		return
	}
	if m.SortMode == mode {
		return
	}
	m.SortMode = mode
	m.SortGroups()
}

func (m *Model) CycleSortMode() {
	if m == nil {
		return
	}
	next := SortMode((int(m.SortMode) + 1) % SortModeCount)
	m.SetSortMode(next)
}

func (m *Model) SortGroups() {
	if m == nil {
		return
	}
	SortGroups(m.Groups, m.SortMode)
}

func SortGroups(groups []*Group, mode SortMode) {
	if len(groups) == 0 {
		return
	}
	switch mode {
	case SortByTotalSize:
		sort.SliceStable(groups, func(i, j int) bool {
			if groups[i].TotalSz == groups[j].TotalSz {
				return len(groups[i].Files) > len(groups[j].Files)
			}
			return groups[i].TotalSz > groups[j].TotalSz
		})
	case SortByCount:
		sort.SliceStable(groups, func(i, j int) bool {
			si := len(groups[i].Files)
			sj := len(groups[j].Files)
			if si == sj {
				return groups[i].TotalSz > groups[j].TotalSz
			}
			return si > sj
		})
	default:
		SortGroups(groups, SortByTotalSize)
	}
}

func MarkAll(groups []*Group) {
	for _, group := range groups {
		for _, entry := range group.Files {
			if entry.Status == FileStatusDeleted {
				continue
			}
			entry.Marked = true
		}
	}
}

func UnmarkAll(groups []*Group) {
	for _, group := range groups {
		for _, entry := range group.Files {
			entry.Marked = false
		}
	}
}

func MarkedEntries(groups []*Group) []*FileEntry {
	var entries []*FileEntry
	for _, group := range groups {
		for _, entry := range group.Files {
			if entry.Marked {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func CountMarked(groups []*Group) int {
	count := 0
	for _, group := range groups {
		for _, entry := range group.Files {
			if entry.Marked {
				count++
			}
		}
	}
	return count
}

func DeleteMarked(groups []*Group) string {
	if len(groups) == 0 {
		return ""
	}

	var deleted, failures int
	for _, entry := range MarkedEntries(groups) {
		err := os.Remove(entry.Path)
		if err != nil {
			entry.Status = FileStatusError
			entry.Message = err.Error()
			if dsklog.Dlogger != nil {
				dsklog.Dlogger.Errorf("Failed to delete file %s: %v", entry.Path, err)
			}
			failures++
		} else {
			entry.Status = FileStatusDeleted
			entry.Message = fmt.Sprintf("deleted (%s)", filepath.Base(entry.Path))
			if dsklog.Dlogger != nil {
				dsklog.Dlogger.Infof("Successfully deleted file: %s", entry.Path)
			}
			deleted++
		}
		entry.Marked = false
	}

	switch {
	case deleted == 0 && failures == 0:
		return "No files were deleted."
	case failures == 0:
		return fmt.Sprintf("Deleted %d file(s).", deleted)
	case deleted == 0:
		return fmt.Sprintf("Failed to delete %d file(s).", failures)
	default:
		return fmt.Sprintf("Deleted %d file(s); %d error(s) occurred.", deleted, failures)
	}
}

func LinkMarked(groups []*Group) string {
	if len(groups) == 0 {
		return ""
	}

	var linked, failures int
	for _, group := range groups {
		var target *FileEntry
		for _, entry := range group.Files {
			if entry.Status == FileStatusDeleted {
				continue
			}
			if !entry.Marked {
				target = entry
				break
			}
		}
		if target == nil {
			for _, entry := range group.Files {
				if !entry.Marked {
					continue
				}
				entry.Status = FileStatusError
				entry.Message = "no unmarked file to link to"
				entry.Marked = false
				failures++
			}
			continue
		}

		for _, entry := range group.Files {
			if !entry.Marked {
				continue
			}
			if entry.Status == FileStatusDeleted {
				entry.Marked = false
				continue
			}

			if err := os.Remove(entry.Path); err != nil {
				entry.Status = FileStatusError
				entry.Message = err.Error()
				if dsklog.Dlogger != nil {
					dsklog.Dlogger.Errorf("Failed to remove file for relink %s: %v", entry.Path, err)
				}
				failures++
				entry.Marked = false
				continue
			}
			if err := os.Symlink(target.Path, entry.Path); err != nil {
				entry.Status = FileStatusError
				entry.Message = err.Error()
				if dsklog.Dlogger != nil {
					dsklog.Dlogger.Errorf("Failed to create symlink %s -> %s: %v", entry.Path, target.Path, err)
				}
				failures++
				entry.Marked = false
				continue
			}
			entry.Status = FileStatusLinked
			entry.Message = fmt.Sprintf("linked -> %s", filepath.Base(target.Path))
			if dsklog.Dlogger != nil {
				dsklog.Dlogger.Infof("Converted duplicate to symlink: %s -> %s", entry.Path, target.Path)
			}
			linked++
			entry.Marked = false
		}
	}

	switch {
	case linked == 0 && failures == 0:
		return "No files were converted."
	case failures == 0:
		return fmt.Sprintf("Converted %d file(s) to symlinks.", linked)
	case linked == 0:
		return fmt.Sprintf("Failed to convert %d file(s) to symlinks.", failures)
	default:
		return fmt.Sprintf("Converted %d file(s); %d error(s) occurred.", linked, failures)
	}
}

func EstimateGroupTotalSize(files []string) uint64 {
	if len(files) == 0 {
		return 0
	}
	perFile := dfs.GetFileSize(files[0])
	return perFile * uint64(len(files))
}

func FormatGroupTitle(hash dmap.Digest, count int, totalSize uint64) string {
	if count == 0 {
		return "Empty group"
	}

	const tmpl = "%s - %d files - (approx. size %s)"
	hashHex := fmt.Sprintf("%x", hash[:16])
	return fmt.Sprintf(tmpl, hashHex, count, utils.DisplaySize(totalSize))
}

func AutoMarkGroup(group *Group) {
	if group == nil {
		return
	}
	for i, entry := range group.Files {
		if i == 0 {
			continue
		}
		entry.Marked = true
		if dsklog.Dlogger != nil {
			dsklog.Dlogger.Debugf("Auto-marked file for deletion: %s", entry.Path)
		}
	}
}

func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func IsAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func GenConfirmationCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	// #nosec G404
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	codeLen := r.Intn(4) + 5
	code := make([]byte, codeLen)
	for i := range code {
		code[i] = charset[r.Intn(len(charset))]
	}
	return string(code)
}
