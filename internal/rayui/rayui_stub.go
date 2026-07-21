//go:build nogui

package rayui

import (
	"fmt"
	"os"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
)

// Launch is a stub for builds without the raylib GUI (-tags nogui).
func Launch(_ *dmap.Dmap, _ dupview.ApplyOptions) {
	fmt.Fprintln(os.Stderr, "dskDitto was built without GUI support")
	os.Exit(1)
}
