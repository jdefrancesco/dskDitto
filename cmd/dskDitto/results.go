package main

import (
	"fmt"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
	"github.com/jdefrancesco/dskDitto/internal/manifest"
	"github.com/jdefrancesco/dskDitto/internal/ui"

	"github.com/pterm/pterm"
)

func processResults(dMap *dmap.Dmap, opts commandOptions, hashAlgo dfs.HashAlgorithm) error {
	keepCount := opts.keep
	// Write backup manifest before any batch-mode action. In interactive mode
	// the manifest is written lazily by the TUI/GUI via applyOptions.BackupPath.
	if opts.backupFile != "" && (keepCount > 0 || opts.timeOnly || opts.textOutput || opts.showBullets || opts.csvOut != "" || opts.jsonOut != "") {
		if err := writeBackupManifest(dMap, hashAlgo, opts.backupFile); err != nil {
			return err
		}
	}

	applyOptions := dupview.ApplyOptions{
		BackupPath:    opts.backupFile,
		HashAlgorithm: hashAlgo,
		SkipConfirm:   opts.noConfirm,
	}

	switch {
	case keepCount > 0 && opts.linkMode:
		linkedPaths, linkErr := dMap.LinkDuplicates(keepCount)
		fmt.Printf("Converted %d duplicate files to symlinks, kept %d real file(s) per group.\n", len(linkedPaths), keepCount)
		if linkErr != nil {
			return fmt.Errorf("Linking completed with errors: %w", linkErr)
		}
	case keepCount > 0 && opts.reflinkMode:
		reflinkedPaths, reflinkErr := dMap.ReflinkDuplicates(keepCount)
		fmt.Printf("Converted %d duplicate files to reflinks, kept %d real file(s) per group.\n", len(reflinkedPaths), keepCount)
		if reflinkErr != nil {
			return fmt.Errorf("Reflinking completed with errors: %w", reflinkErr)
		}
	case keepCount > 0:
		removedPaths, removeErr := dMap.RemoveDuplicates(keepCount)
		fmt.Printf("Removed %d duplicate files, kept %d per group.\n", len(removedPaths), keepCount)
		if removeErr != nil {
			return fmt.Errorf("Removal completed with errors: %w", removeErr)
		}
	case opts.csvOut != "":
		pterm.Info.Printf("Writing CSV to %s...\n", opts.csvOut)
		if err := dMap.WriteCSV(opts.csvOut); err != nil {
			return fmt.Errorf("failed to write CSV output: %w", err)
		}
		pterm.Success.Printf("CSV file %s written to disk.\n", opts.csvOut)
	case opts.jsonOut != "":
		pterm.Info.Printf("Writing JSON to %s...\n", opts.jsonOut)
		if err := dMap.WriteJSON(opts.jsonOut); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		pterm.Success.Printf("JSON file %s written to disk.\n", opts.jsonOut)
	case opts.timeOnly:
		// scan-only benchmark mode; elapsed time already printed above
	case opts.textOutput:
		dMap.PrintDmap()
	case opts.showBullets:
		dMap.ShowResultsBullet()
	case opts.gui:
		if err := launchGUI(dMap, applyOptions); err != nil {
			return err
		}
	default:
		ui.LaunchTUI(dMap, applyOptions)
	}
	return nil
}

func writeBackupManifest(dMap *dmap.Dmap, algo dfs.HashAlgorithm, path string) error {
	entries, err := manifest.EntriesFromDmap(dMap, algo)
	if err != nil {
		return fmt.Errorf("failed to build restore manifest: %w", err)
	}
	if err := manifest.Write(path, entries); err != nil {
		return fmt.Errorf("failed to write restore manifest: %w", err)
	}
	pterm.Success.Printf("Restore backup file with %d entries written to %s.\n", len(entries), path)
	return nil
}
