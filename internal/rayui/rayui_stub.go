//go:build !gui

package rayui

import (
	"fmt"
	"os"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
)

// Launch is a stub used when dskDitto is built without the "gui" build tag.
// The raylib-based GUI pulls in cgo/GLFW/Wayland dependencies that most
// users (TUI, --text, --csv, --json, etc.) never need, so it is excluded
// from the default build. Rebuild with -tags gui to enable --gui.
func Launch(dMap *dmap.Dmap, applyOptions dupview.ApplyOptions) {
	fmt.Fprintln(os.Stderr, "dskDitto: --gui support was not built into this binary.")
	fmt.Fprintln(os.Stderr, "Rebuild with: go install -tags gui github.com/jdefrancesco/dskDitto/cmd/dskDitto@latest")
	fmt.Fprintln(os.Stderr, "or: make build-gui")
	os.Exit(1)
}
