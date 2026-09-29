package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
)

func TestProcessResultsStopsWhenBackupFails(t *testing.T) {
	dir := t.TempDir()
	dsklog.InitializeDlogger(filepath.Join(dir, "test.log"))
	dm, err := dmap.NewDmap(2)
	if err != nil {
		t.Fatal(err)
	}
	algo, err := dfs.ParseHashAlgorithm("sha256")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "results.csv")
	err = processResults(dm, commandOptions{backupFile: dir, csvOut: output}, algo)
	if err == nil || !strings.Contains(err.Error(), "failed to write restore manifest") {
		t.Fatalf("expected backup failure, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output should not be written after backup failure; stat error: %v", err)
	}
}
