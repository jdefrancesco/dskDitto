package utils

import "fmt"

// Our size constants
const (
	_   = iota
	KiB = 1 << (10 * iota)
	MiB
	GiB
	TiB
	PiB
	EiB
)

// DisplaySize takes a number of bytes and returns a human-readable string
func DisplaySize(bytes uint64) string {

	switch {
	case bytes < KiB:
		return fmt.Sprintf("%d B", bytes)
	case bytes < MiB:
		return fmt.Sprintf("%.2f KiB", float64(bytes)/float64(KiB))
	case bytes < GiB:
		return fmt.Sprintf("%.2f MiB", float64(bytes)/float64(MiB))
	case bytes < TiB:
		return fmt.Sprintf("%.2f GiB", float64(bytes)/float64(GiB))
	case bytes < PiB:
		return fmt.Sprintf("%.2f TiB", float64(bytes)/float64(TiB))
	case bytes < EiB:
		return fmt.Sprintf("%.2f PiB", float64(bytes)/float64(PiB))
	default:
		return fmt.Sprintf("%.2f EiB", float64(bytes)/float64(EiB))
	}
}
