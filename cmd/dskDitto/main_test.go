package main

import "testing"

func TestNewDupView(t *testing.T) {

}

func TestNormalizeInteractiveUI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default", in: "", want: "tui"},
		{name: "tui", in: "tui", want: "tui"},
		{name: "bubble tea alias", in: "bubble-tea", want: "tui"},
		{name: "raylib", in: "raylib", want: "raylib"},
		{name: "gui alias", in: "gui", want: "raylib"},
		{name: "trim and case", in: " RayLib ", want: "raylib"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeInteractiveUI(tt.in)
			if err != nil {
				t.Fatalf("normalizeInteractiveUI(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeInteractiveUI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeInteractiveUIRejectsUnknownMode(t *testing.T) {
	if _, err := normalizeInteractiveUI("web"); err == nil {
		t.Fatalf("expected error for unknown UI mode")
	}
}
