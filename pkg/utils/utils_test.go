package utils

import (
	"fmt"
	"testing"
)

// Test the DisplaySize function
func TestDisplaySize(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1024, "1.00 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.00 TiB"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.00 PiB"},
		{1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1.00 EiB"},
		{234, "234 B"},
		{200034, "195.35 KiB"},
	}

	for _, test := range tests {
		got := DisplaySize(test.bytes)
		if got != test.want {
			t.Errorf("DisplaySize(%d) = %q; want %q", test.bytes, got, test.want)
		}
		fmt.Printf("Success. DisplaySize(%d) = %s\n", test.bytes, got)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
	}{
		{"0", 0},
		{"1024", 1024},
		{"1K", KB},
		{"1KB", KB},
		{"1KiB", uint64(KiB)},
		{"1.5G", GB + GB/2},
		{"2Gi", 2 * uint64(GiB)},
		{"2GB", 2 * GB},
		{"750MiB", 750 * uint64(MiB)},
		{"1e3", 1000},
		{"512b", 512},
		{"2 GiB", 2 * uint64(GiB)},
	}

	for _, tc := range tests {
		got, err := ParseSize(tc.input)
		if err != nil {
			t.Fatalf("ParseSize(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("ParseSize(%q) = %d; want %d", tc.input, got, tc.want)
		}
	}

	invalid := []string{"", "abc", "-1", "1XB"}
	for _, input := range invalid {
		if _, err := ParseSize(input); err == nil {
			t.Errorf("ParseSize(%q) expected error, got nil", input)
		}
	}
}
