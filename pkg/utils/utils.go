package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Our size constants. Powers of two!
const (
	_   = iota
	KiB = 1 << (10 * iota)
	MiB
	GiB
	TiB
	PiB
	EiB
)

const (
	KB uint64 = 1000
	MB        = KB * 1000
	GB        = MB * 1000
	TB        = GB * 1000
	PB        = TB * 1000
	EB        = PB * 1000
)

var sizeSuffixMultipliers = map[string]uint64{
	"":          1,
	"b":         1,
	"byte":      1,
	"bytes":     1,
	"k":         KB,
	"kb":        KB,
	"kbyte":     KB,
	"kbytes":    KB,
	"kilobyte":  KB,
	"kilobytes": KB,
	"m":         MB,
	"mb":        MB,
	"mbyte":     MB,
	"mbytes":    MB,
	"megabyte":  MB,
	"megabytes": MB,
	"g":         GB,
	"gb":        GB,
	"gbyte":     GB,
	"gbytes":    GB,
	"gigabyte":  GB,
	"gigabytes": GB,
	"t":         TB,
	"tb":        TB,
	"terabyte":  TB,
	"terabytes": TB,
	"p":         PB,
	"pb":        PB,
	"petabyte":  PB,
	"petabytes": PB,
	"e":         EB,
	"eb":        EB,
	"exabyte":   EB,
	"exabytes":  EB,
	"ki":        uint64(KiB),
	"kib":       uint64(KiB),
	"kibi":      uint64(KiB),
	"kibibyte":  uint64(KiB),
	"kibibytes": uint64(KiB),
	"mi":        uint64(MiB),
	"mib":       uint64(MiB),
	"mibi":      uint64(MiB),
	"mibibyte":  uint64(MiB),
	"mibibytes": uint64(MiB),
	"gi":        uint64(GiB),
	"gib":       uint64(GiB),
	"gibi":      uint64(GiB),
	"gibibyte":  uint64(GiB),
	"gibibytes": uint64(GiB),
	"ti":        uint64(TiB),
	"tib":       uint64(TiB),
	"tibi":      uint64(TiB),
	"tibibyte":  uint64(TiB),
	"tibibytes": uint64(TiB),
	"pi":        uint64(PiB),
	"pib":       uint64(PiB),
	"pibi":      uint64(PiB),
	"pibibyte":  uint64(PiB),
	"pibibytes": uint64(PiB),
	"ei":        uint64(EiB),
	"eib":       uint64(EiB),
	"eibi":      uint64(EiB),
	"eibibyte":  uint64(EiB),
	"eibibytes": uint64(EiB),
}

// ParseSize converts human-readable size strings (e.g. "10M", "4GiB", "1.5T") to bytes.
// Supported suffixes default to binary multiples (KiB, MiB, GiB, etc.).
func ParseSize(input string) (uint64, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, fmt.Errorf("size string is empty")
	}

	normalized := strings.ToLower(trimmed)
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, ",", "")

	if f, err := strconv.ParseFloat(normalized, 64); err == nil {
		if f < 0 {
			return 0, fmt.Errorf("size must be non-negative: %s", input)
		}
		if f > float64(math.MaxUint64) {
			return 0, fmt.Errorf("size %s overflows uint64", input)
		}
		return uint64(f), nil
	}

	if strings.HasPrefix(normalized, "-") {
		return 0, fmt.Errorf("size must be non-negative: %s", input)
	}

	normalized = strings.TrimPrefix(normalized, "+")

	if normalized == "" {
		return 0, fmt.Errorf("size string is empty")
	}

	idx := 0
	for idx < len(normalized) {
		r := normalized[idx]
		if (r >= '0' && r <= '9') || r == '.' {
			idx++
			continue
		}
		break
	}

	numPart := normalized[:idx]
	suffix := normalized[idx:]
	if numPart == "" {
		return 0, fmt.Errorf("invalid size %q", input)
	}

	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", input, err)
	}

	multiplier, err := lookupMultiplier(suffix)
	if err != nil {
		return 0, err
	}

	product := value * float64(multiplier)
	if product < 0 || product > float64(math.MaxUint64) {
		return 0, fmt.Errorf("size %s overflows uint64", input)
	}
	if math.IsInf(product, 1) || math.IsNaN(product) {
		return 0, fmt.Errorf("size %s is not a finite number", input)
	}

	return uint64(product), nil
}

func lookupMultiplier(suffix string) (uint64, error) {
	candidates := []string{
		suffix,
		strings.TrimSuffix(suffix, "s"),
		strings.TrimSuffix(suffix, "byte"),
		strings.TrimSuffix(suffix, "bytes"),
		strings.TrimSuffix(suffix, "b"),
	}

	for _, candidate := range candidates {
		if multiplier, ok := sizeSuffixMultipliers[candidate]; ok {
			return multiplier, nil
		}
	}

	if suffix == "" {
		return 1, nil
	}

	return 0, fmt.Errorf("unknown size suffix %q", suffix)
}

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

// IsAlphanumeric checks if a rune is alphanumeric (letter or digit)
func IsAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
