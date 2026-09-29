package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dwalk"
	"github.com/jdefrancesco/dskDitto/pkg/utils"
)

func addNameOnlyGroups(dMap *dmap.Dmap, nameGroups map[string][]dwalk.FileCandidate, minDups uint) (uint, uint) {
	if dMap == nil {
		return 0, 0
	}
	if minDups < 2 {
		minDups = 2
	}

	names := make([]string, 0, len(nameGroups))
	for name := range nameGroups {
		names = append(names, name)
	}
	sort.Strings(names)

	var addedGroups uint
	var skipped uint
	for _, name := range names {
		files := nameGroups[name]
		if uint(len(files)) < minDups {
			skipped += uint(len(files))
			continue
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
		for _, file := range files {
			dMap.AddNamePathSized(name, file.Path, file.Size)
		}
		addedGroups++
	}
	return addedGroups, skipped
}

func resolveSkipHidden(includeHidden bool, shallowTargetName string, singleFilePath string) bool {
	if includeHidden || shallowTargetIsHidden(shallowTargetName) || singleFileTargetIsHidden(singleFilePath) {
		return false
	}
	return true
}

func shallowTargetIsHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func singleFileTargetIsHidden(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(path), ".")
}

func validateShallowMode(nameOnly bool, fileShallow, singleFile, backupFile string) (string, error) {
	shallowMode := nameOnly || fileShallow != ""
	if !shallowMode {
		return "", nil
	}
	if singleFile != "" && fileShallow != "" {
		return "", fmt.Errorf("--file cannot be combined with --file-shallow")
	}
	if backupFile != "" {
		return "", fmt.Errorf("restore backups are not supported for shallow/name-only finds; rerun without --backup")
	}
	if singleFile != "" {
		return shallowFileName(singleFile, "--file")
	}
	if fileShallow == "" {
		return "", nil
	}
	return shallowFileName(fileShallow, "--file-shallow")
}

func shallowFileName(path, flagName string) (string, error) {
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(os.PathSeparator) {
		return "", fmt.Errorf("%s must name a file path", flagName)
	}
	return name, nil
}

func validateRestoreMode(restoreManifest, backupFile string, args []string, gui, textOutput, bulletOutput bool, csvOut, jsonOut, singleFile, fileShallow string, nameOnly bool, keep uint, linkMode bool) error {
	if restoreManifest == "" {
		return fmt.Errorf("--restore path must not be empty")
	}
	if backupFile != "" {
		return fmt.Errorf("--restore cannot be combined with --backup")
	}
	if len(args) > 0 {
		return fmt.Errorf("path arguments are not allowed with --restore")
	}
	if gui || textOutput || bulletOutput || csvOut != "" || jsonOut != "" || keep > 0 || linkMode || singleFile != "" || fileShallow != "" || nameOnly {
		return fmt.Errorf("--restore cannot be combined with scan/output/mutation flags")
	}
	return nil
}

func resolveMaxFileSize(allSizes bool, maxSizeValue string) (int64, error) {
	if allSizes && maxSizeValue != "" {
		return 0, fmt.Errorf("--all-sizes cannot be combined with --max-size")
	}
	if allSizes {
		return 0, nil
	}
	if maxSizeValue == "" {
		return dwalk.MAX_FILE_SIZE, nil
	}

	value, err := utils.ParseSize(maxSizeValue)
	if err != nil {
		return 0, fmt.Errorf("invalid value for --max-size: %v", err)
	}
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("--max-size %s exceeds supported file size limit (%d bytes)", maxSizeValue, int64(math.MaxInt64))
	}
	return int64(value), nil // #nosec G115 -- bounds checked above
}
