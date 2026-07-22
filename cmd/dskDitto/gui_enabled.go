//go:build gui && cgo

package main

import (
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
	"github.com/jdefrancesco/dskDitto/internal/rayui"
)

func launchGUI(dMap *dmap.Dmap, applyOptions dupview.ApplyOptions) error {
	rayui.Launch(dMap, applyOptions)
	return nil
}

func validateGUIBuild() error {
	return nil
}
