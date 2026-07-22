//go:build !gui || !cgo

package main

import (
	"errors"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
)

func validateGUIBuild() error {
	return guiUnavailableError()
}

func launchGUI(_ *dmap.Dmap, _ dupview.ApplyOptions) error {
	return guiUnavailableError()
}

func guiUnavailableError() error {
	return errors.New("GUI support was not built into this binary. Reinstall with: go install -tags gui github.com/jdefrancesco/dskDitto/cmd/dskDitto@latest")
}
