package dupview

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
)

type ApplyOptions struct {
	BackupPath    string
	HashAlgorithm dfs.HashAlgorithm
}

type plannedMutation struct {
	action     Action
	targetPath string
	affected   []*FileEntry
}

func ApplyMarked(groups []*Group, action Action, opts ApplyOptions) (string, error) {
	if opts.BackupPath == "" {
		switch action {
		case ActionLink:
			return LinkMarked(groups), nil
		default:
			return DeleteMarked(groups), nil
		}
	}
	if opts.HashAlgorithm == "" {
		return "", fmt.Errorf("backup manifest requires a hash algorithm")
	}

	plan, entries, err := buildMutationPlan(groups, action, opts.HashAlgorithm)
	if err != nil {
		return "", err
	}
	if err := manifest.AppendUnique(opts.BackupPath, entries); err != nil {
		return "", fmt.Errorf("write restore manifest %s: %w", opts.BackupPath, err)
	}

	return executeMutationPlan(plan), nil
}

func buildMutationPlan(groups []*Group, action Action, algo dfs.HashAlgorithm) ([]plannedMutation, []manifest.Entry, error) {
	plan := make([]plannedMutation, 0, len(groups))
	entries := make([]manifest.Entry, 0)
	var groupID uint64 = 1

	for _, group := range groups {
		marked := markedActionEntries(group)
		if len(marked) == 0 {
			continue
		}

		target := survivingTarget(group)
		if target == nil {
			return nil, nil, fmt.Errorf("group %q has no surviving canonical file for backup", group.Title)
		}

		hash := fmt.Sprintf("%x", group.Hash)
		plan = append(plan, plannedMutation{
			action:     action,
			targetPath: target.Path,
			affected:   marked,
		})

		for _, entry := range marked {
			manifestEntry, err := manifest.NewEntry(groupID, algo, hash, target.Path, entry.Path)
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, manifestEntry)
		}
		groupID++
	}

	return plan, entries, nil
}

func markedActionEntries(group *Group) []*FileEntry {
	if group == nil {
		return nil
	}

	entries := make([]*FileEntry, 0, len(group.Files))
	for _, entry := range group.Files {
		if entry == nil || !entry.Marked || entry.Status == FileStatusDeleted {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func survivingTarget(group *Group) *FileEntry {
	if group == nil {
		return nil
	}

	for _, entry := range group.Files {
		if entry == nil || entry.Status == FileStatusDeleted || entry.Marked {
			continue
		}
		return entry
	}
	return nil
}

func executeMutationPlan(plan []plannedMutation) string {
	if len(plan) == 0 {
		return ""
	}

	switch plan[0].action {
	case ActionLink:
		return executeLinkPlan(plan)
	default:
		return executeDeletePlan(plan)
	}
}

func executeDeletePlan(plan []plannedMutation) string {
	var deleted, failures int
	for _, step := range plan {
		for _, entry := range step.affected {
			if err := os.Remove(entry.Path); err != nil {
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

func executeLinkPlan(plan []plannedMutation) string {
	var linked, failures int
	for _, step := range plan {
		for _, entry := range step.affected {
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
			if err := os.Symlink(step.targetPath, entry.Path); err != nil {
				entry.Status = FileStatusError
				entry.Message = err.Error()
				if dsklog.Dlogger != nil {
					dsklog.Dlogger.Errorf("Failed to create symlink %s -> %s: %v", entry.Path, step.targetPath, err)
				}
				failures++
				entry.Marked = false
				continue
			}
			entry.Status = FileStatusLinked
			entry.Message = fmt.Sprintf("linked -> %s", filepath.Base(step.targetPath))
			if dsklog.Dlogger != nil {
				dsklog.Dlogger.Infof("Converted duplicate to symlink: %s -> %s", entry.Path, step.targetPath)
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
