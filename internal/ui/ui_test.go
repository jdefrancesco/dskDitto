package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dupview"
)

// TestGenerateConfirmationCodes tests the GenConfirmationCode function
func TestGenerateConfirmationCodes(t *testing.T) {

	for i := range 100 {
		code := GenConfirmationCode()

		if len(code) < 5 || len(code) > 8 {
			t.Errorf("Generated code length out of bounds: got %d, want between 5 and 8", len(code))
		}
		for _, c := range code {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("Generated code contains invalid character: %q", c)
			}
		}

		if i%10 == 0 {
			t.Logf("Sample generated code: %s", code)
		}
	}
}

func TestStartConfirmationPromptUsesSafePromptByDefault(t *testing.T) {
	m := &model{
		mode: modeTree,
		groups: []*dupview.Group{
			{Files: []*dupview.FileEntry{{Path: "/tmp/marked", Marked: true}}},
		},
	}

	m.startConfirmationPrompt(confirmDelete)

	if m.mode != modeConfirm {
		t.Fatalf("expected confirmation mode")
	}
	if m.confirmCode == "" {
		t.Fatalf("expected confirmation code")
	}
}

func TestStartConfirmationPromptSkipsPromptWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marked.txt")
	if err := os.WriteFile(path, []byte("delete me"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	m := &model{
		mode: modeTree,
		groups: []*dupview.Group{
			{Files: []*dupview.FileEntry{{Path: path, Marked: true}}},
		},
		applyOptions: dupview.ApplyOptions{SkipConfirm: true},
	}

	m.startConfirmationPrompt(confirmDelete)

	if m.mode != modeTree {
		t.Fatalf("expected tree mode after direct delete")
	}
	if m.confirmCode != "" {
		t.Fatalf("did not expect confirmation code")
	}
	if !strings.Contains(m.deleteResult, "Deleted 1 file") {
		t.Fatalf("expected delete result, got %q", m.deleteResult)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected marked file to be deleted, stat err: %v", err)
	}
}
