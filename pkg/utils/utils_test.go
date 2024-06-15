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
