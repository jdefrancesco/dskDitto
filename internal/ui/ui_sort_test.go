package ui

import "testing"

func TestSortGroupsByTotalSize(t *testing.T) {
	m := &model{
		sortMode: sortByTotalSize,
		groups: []*duplicateGroup{
			{Title: "small", TotalSz: 100, Files: []*fileEntry{{Path: "a"}, {Path: "b"}}},
			{Title: "big", TotalSz: 500, Files: []*fileEntry{{Path: "c"}}},
			{Title: "medium", TotalSz: 250, Files: []*fileEntry{{Path: "d"}}},
		},
	}

	m.sortGroups()

	if got := m.groups[0].Title; got != "big" {
		t.Fatalf("expected largest group first, got %q", got)
	}
	if got := m.groups[2].Title; got != "small" {
		t.Fatalf("expected smallest group last, got %q", got)
	}
}

func TestSortGroupsByCount(t *testing.T) {
	m := &model{
		sortMode: sortByCount,
		groups: []*duplicateGroup{
			{Title: "two", TotalSz: 300, Files: []*fileEntry{{Path: "a"}, {Path: "b"}}},
			{Title: "three", TotalSz: 200, Files: []*fileEntry{{Path: "c"}, {Path: "d"}, {Path: "e"}}},
			{Title: "one", TotalSz: 400, Files: []*fileEntry{{Path: "f"}}},
		},
	}

	m.sortGroups()

	if got := m.groups[0].Title; got != "three" {
		t.Fatalf("expected group with most files first, got %q", got)
	}
	if got := m.groups[2].Title; got != "one" {
		t.Fatalf("expected group with fewest files last, got %q", got)
	}
}

func TestSortGroupsByPath(t *testing.T) {
	m := &model{
		sortMode: sortByPath,
		groups: []*duplicateGroup{
			{Title: "zeta", TotalSz: 300, Files: []*fileEntry{{Path: "zeta.bin"}, {Path: "omega.bin"}}},
			{Title: "alpha", TotalSz: 100, Files: []*fileEntry{{Path: "alpha.bin"}}},
			{Title: "middle", TotalSz: 200, Files: []*fileEntry{{Path: "middle.bin"}}},
		},
	}

	m.sortGroups()

	want := []string{"alpha", "middle", "zeta"}
	for i, title := range want {
		if got := m.groups[i].Title; got != title {
			t.Fatalf("unexpected group at index %d: got %q want %q", i, got, title)
		}
	}
}

func TestCycleSortModeIncludesPathSort(t *testing.T) {
	m := &model{sortMode: sortByTotalSize}

	m.cycleSortMode()
	if m.sortMode != sortByCount {
		t.Fatalf("expected second sort mode to be dup count, got %v", m.sortMode)
	}

	m.cycleSortMode()
	if m.sortMode != sortByPath {
		t.Fatalf("expected third sort mode to be path, got %v", m.sortMode)
	}

	m.cycleSortMode()
	if m.sortMode != sortByTotalSize {
		t.Fatalf("expected sort cycle to wrap to total size, got %v", m.sortMode)
	}
}
