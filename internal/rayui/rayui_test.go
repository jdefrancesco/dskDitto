package rayui

import (
	"strings"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func TestFooterHelpWidthBuckets(t *testing.T) {
	if got := footerHelp(800); got != "arrows navigate | space marks | d delete | shift+L link | q exits" {
		t.Fatalf("unexpected narrow footer help: %q", got)
	}

	if got := footerHelp(1000); got != "jk arrows navigate | enter folds | space marks | a mark all | u clear | d delete | q exits" {
		t.Fatalf("unexpected medium footer help: %q", got)
	}

	if got := footerHelp(1400); got != "jk arrows navigate | enter folds | space/m mark | a mark all | u clear | d delete | shift+L link | q exits" {
		t.Fatalf("unexpected wide footer help: %q", got)
	}
}

func TestTruncateMeasuredText(t *testing.T) {
	measure := func(text string, _ int32) float32 { return float32(len([]rune(text))) }

	if got := truncateMeasuredText("abcdef", 12, 6, measure); got != "abcdef" {
		t.Fatalf("expected full text when it fits, got %q", got)
	}

	if got := truncateMeasuredText("abcdef", 12, 3, measure); got != "..." {
		t.Fatalf("expected suffix-only truncation, got %q", got)
	}

	if got := truncateMeasuredText("abcdef", 12, 2, measure); got != "" {
		t.Fatalf("expected empty text when suffix cannot fit, got %q", got)
	}

	got := truncateMeasuredText("abc   def", 12, 7, measure)
	if strings.Contains(got, " ...") {
		t.Fatalf("expected trailing spaces to be trimmed before suffix, got %q", got)
	}
}

func TestFormatCompactGroupTitleAndHashPrefix(t *testing.T) {
	group := &dupview.Group{
		Hash: dmap.Digest{
			0x00, 0x11, 0x22, 0x33,
			0x44, 0x55, 0x66, 0x77,
			0x88, 0x99, 0xaa, 0xbb,
			0xcc, 0xdd, 0xee, 0xff,
		},
		Files: []*dupview.FileEntry{
			{Path: "/tmp/a"},
			{Path: "/tmp/b"},
			{Path: "/tmp/c"},
		},
		TotalSz: 3 * 1024,
	}

	prefix := hashPrefix(group)
	if len(prefix) != 16 {
		t.Fatalf("expected 16-char hash prefix, got %q", prefix)
	}

	title := formatCompactGroupTitle(group)
	if !strings.Contains(title, prefix) {
		t.Fatalf("expected title to contain hash prefix %q, got %q", prefix, title)
	}
	if !strings.Contains(title, "3 files") {
		t.Fatalf("expected title to include file count, got %q", title)
	}
	if !strings.Contains(title, "3.00 KiB") {
		t.Fatalf("expected title to include human size, got %q", title)
	}
}

func TestRebuildVisibleNodesAndToggleMark(t *testing.T) {
	groups := []*dupview.Group{
		{
			Expanded: true,
			Files: []*dupview.FileEntry{
				{Path: "/tmp/a"},
				{Path: "/tmp/b"},
			},
		},
		{
			Expanded: false,
			Files: []*dupview.FileEntry{
				{Path: "/tmp/c"},
			},
		},
	}

	a := &app{
		results: &dupview.Model{Groups: groups},
		layout: layout{
			rowHeight: 20,
			list:      rl.NewRectangle(0, 0, 120, 90),
		},
		lastClickIdx: -1,
		lastGroupIdx: -1,
	}

	a.rebuildVisibleNodes()

	if got, want := len(a.visible), 4; got != want {
		t.Fatalf("unexpected visible node count: got %d want %d", got, want)
	}
	if a.visible[0].typ != nodeGroup || a.visible[1].typ != nodeFile || a.visible[3].typ != nodeGroup {
		t.Fatalf("unexpected node order: %#v", a.visible)
	}

	a.cursor = 1
	a.toggleCurrentFileMark()
	if !groups[0].Files[0].Marked {
		t.Fatalf("expected first file to be marked")
	}

	a.cursor = 2
	groups[0].Files[1].Status = dupview.FileStatusDeleted
	a.toggleCurrentFileMark()
	if groups[0].Files[1].Marked {
		t.Fatalf("expected deleted file to remain unmarked")
	}
}

func TestAdjustScrollKeepsCursorVisible(t *testing.T) {
	groups := []*dupview.Group{
		{Expanded: true, Files: []*dupview.FileEntry{{Path: "/tmp/a"}}},
	}

	a := &app{
		results: &dupview.Model{Groups: groups},
		layout: layout{
			rowHeight: 20,
			// insetRect(list, 1).Height => 58, so visibleRows() == 2
			list: rl.NewRectangle(0, 0, 120, 60),
		},
		visible: []nodeRef{
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
		},
		lastClickIdx: -1,
		lastGroupIdx: -1,
	}

	a.cursor = 4
	a.scroll = 0
	a.adjustScroll()
	if got, want := a.scroll, 3; got != want {
		t.Fatalf("expected scroll to follow cursor, got %d want %d", got, want)
	}

	a.scroll = 99
	a.adjustScroll()
	if got, want := a.scroll, 3; got != want {
		t.Fatalf("expected scroll clamped to max, got %d want %d", got, want)
	}
}

func TestApplyWheelScrollMovesCursorWithViewport(t *testing.T) {
	a := &app{
		layout: layout{
			rowHeight: 20,
			// insetRect(list, 1).Height => 58, so visibleRows() == 2
			list: rl.NewRectangle(0, 0, 120, 60),
		},
		visible: []nodeRef{
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
		},
		cursor: 1,
		scroll: 0,
	}

	a.applyWheelScroll(-1)

	if got, want := a.scroll, 3; got != want {
		t.Fatalf("expected viewport to scroll, got %d want %d", got, want)
	}
	if got, want := a.cursor, 4; got != want {
		t.Fatalf("expected cursor to move with viewport, got %d want %d", got, want)
	}
	if got := a.wheelRemainder; got != 0 {
		t.Fatalf("expected wheel remainder to be cleared after whole-step scroll, got %v", got)
	}
}

func TestApplyWheelScrollKeepsCursorStableAtClamp(t *testing.T) {
	a := &app{
		layout: layout{
			rowHeight: 20,
			// insetRect(list, 1).Height => 58, so visibleRows() == 2
			list: rl.NewRectangle(0, 0, 120, 60),
		},
		visible: []nodeRef{
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
			{typ: nodeGroup, group: 0},
		},
		cursor: 6,
		scroll: 6,
	}

	a.applyWheelScroll(-1)

	if got, want := a.scroll, 6; got != want {
		t.Fatalf("expected viewport to stay clamped at bottom, got %d want %d", got, want)
	}
	if got, want := a.cursor, 6; got != want {
		t.Fatalf("expected cursor to stay put when viewport cannot move, got %d want %d", got, want)
	}
}
