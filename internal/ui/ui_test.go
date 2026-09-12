package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
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

func TestHandleTreeKeysReflinkStartsConfirmation(t *testing.T) {
	m := &model{
		mode: modeTree,
		groups: []*dupview.Group{
			{Files: []*dupview.FileEntry{{Path: "/tmp/marked", Marked: true}}},
		},
	}

	m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})

	if m.mode != modeConfirm {
		t.Fatalf("expected confirmation mode")
	}
	if m.action != confirmReflink {
		t.Fatalf("expected reflink action, got %v", m.action)
	}
}

func TestHandleTreeKeysSortsAlphabeticallyWithThree(t *testing.T) {
	m := &model{
		mode:     modeTree,
		sortMode: sortByTotalSize,
		groups: []*dupview.Group{
			{Title: "zeta", TotalSz: 300, Files: []*dupview.FileEntry{{Path: "/var/zeta.bin"}}},
			{Title: "alpha", TotalSz: 100, Files: []*dupview.FileEntry{{Path: "/Users/example/alpha.bin"}}},
		},
	}

	m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})

	if m.sortMode != sortByPath {
		t.Fatalf("expected path sort mode, got %v", m.sortMode)
	}
	if got := m.groups[0].Title; got != "alpha" {
		t.Fatalf("expected alphabetically first group first, got %q", got)
	}
}

func TestHandleTreeKeysCollapseAndOpenAllGroups(t *testing.T) {
	m := &model{
		mode: modeTree,
		groups: []*dupview.Group{
			{
				Expanded: true,
				Files: []*dupview.FileEntry{
					{Path: "/tmp/a"},
					{Path: "/tmp/b"},
				},
			},
			{
				Expanded: false,
				Files:    []*dupview.FileEntry{{Path: "/tmp/c"}},
			},
		},
	}
	m.rebuildVisibleNodes()

	m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if m.groups[0].Expanded || m.groups[1].Expanded {
		t.Fatalf("expected all groups to collapse")
	}
	if got, want := len(m.visible), 2; got != want {
		t.Fatalf("expected only group rows after collapse, got %d want %d", got, want)
	}

	m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !m.groups[0].Expanded || !m.groups[1].Expanded {
		t.Fatalf("expected all groups to open")
	}
	if got, want := len(m.visible), 5; got != want {
		t.Fatalf("expected group and file rows after open, got %d want %d", got, want)
	}
}

func TestFormatFileStatusReflinked(t *testing.T) {
	entry := &fileEntry{Path: "/tmp/dup.bin", Status: fileStatusReflinked, Message: "reflinked -> /tmp/target.bin"}

	got := formatFileStatus(entry, 40)
	if !strings.Contains(got, "REFLINK") {
		t.Fatalf("expected status text to contain REFLINK, got %q", got)
	}
}

func TestToggleCurrentFileMarkAllowsFuzzyGroup(t *testing.T) {
	m := &model{
		groups: []*dupview.Group{
			{
				MatchInfo: dmap.MatchInfo{Type: dmap.MatchFuzzy, Key: "near-content"},
				Expanded:  true,
				Files:     []*dupview.FileEntry{{Path: "/tmp/fuzzy.bin"}},
			},
		},
		visible: []nodeRef{{typ: nodeGroup, group: 0}, {typ: nodeFile, group: 0, file: 0}},
		cursor:  1,
	}

	m.toggleCurrentFileMark()
	if !m.groups[0].Files[0].Marked {
		t.Fatalf("expected fuzzy file to become marked")
	}
}

func TestEffectiveWidthUsesWideTerminalWidth(t *testing.T) {
	m := &model{width: 180}

	if got := m.effectiveWidth(); got != 180 {
		t.Fatalf("expected effective width to use terminal width, got %d", got)
	}
}

func TestFooterMentionsAlphabeticalSortAndGroupControls(t *testing.T) {
	m := &model{}

	sortText := m.sortHotkeysText()
	for _, want := range []string{"1 sort by total size", "2 sort by dup count", "3 sort alphabetically", "s cycle"} {
		if !strings.Contains(sortText, want) {
			t.Fatalf("expected sort footer %q to contain %q", sortText, want)
		}
	}

	navText := m.instructionsText()
	for _, want := range []string{"c collapse all", "o open all"} {
		if !strings.Contains(navText, want) {
			t.Fatalf("expected navigation footer %q to contain %q", navText, want)
		}
	}
}
