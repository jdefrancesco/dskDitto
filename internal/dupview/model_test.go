package dupview

import (
	"strings"
	"testing"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
)

func TestFormatGroupTitleFuzzy(t *testing.T) {
	title := FormatGroupTitle(dmap.Digest{}, dmap.MatchInfo{Type: dmap.MatchFuzzy, Key: "near-content >=85%"}, 3, 4096)
	if !strings.Contains(title, "Similar: near-content >=85%") {
		t.Fatalf("expected fuzzy title prefix, got %q", title)
	}
}

func TestAutoMarkGroupSkipsFuzzy(t *testing.T) {
	group := &Group{
		MatchInfo: dmap.MatchInfo{Type: dmap.MatchFuzzy, Key: "near-content"},
		Files: []*FileEntry{
			{Path: "/tmp/a"},
			{Path: "/tmp/b"},
		},
	}

	AutoMarkGroup(group)
	if group.Files[0].Marked || group.Files[1].Marked {
		t.Fatalf("expected fuzzy group entries to remain unmarked")
	}
}
