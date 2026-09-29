package main

import (
	"fmt"

	"github.com/jdefrancesco/dskDitto/internal/buildinfo"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

// showHeader prints dskDitto banner.
// When safe is true, it avoids explicit colors to maximize contrast across themes.
func showHeader(safe bool) {

	fmt.Println("")

	leftStyle := pterm.NewStyle(pterm.FgLightGreen)
	rightStyle := pterm.NewStyle(pterm.FgLightWhite)
	if safe {
		leftStyle = pterm.NewStyle()
		rightStyle = pterm.NewStyle()
	}

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", leftStyle),
		putils.LettersFromStringWithStyle("Ditto", rightStyle),
	).Render()
}

func showVersion() {
	fmt.Printf("Version: %s\n", buildinfo.Version)
	fmt.Printf("Github: https://github.com/jdefrancesco/dskDitto\n")
	// Get rid of pesky percent sign some shells show if new line isn't printed correctly.
	fmt.Println("")
}

func showScanSummary(stats scanStats, fuzzyMode, shallowMode bool) {
	// Status bar update
	finalInfo := "Scanned " + pterm.LightWhite(stats.scannedFiles) + " files, sampled " +
		pterm.LightWhite(stats.sampledFiles) + " candidates, fully hashed " +
		pterm.LightWhite(stats.fullHashedFiles) + " in " + pterm.LightWhite(stats.duration)
	if fuzzyMode {
		finalInfo = "Scanned " + pterm.LightWhite(stats.scannedFiles) + " files, fuzzy-signatured " + pterm.LightWhite(stats.fuzzyProcessed) +
			" (skipped " + pterm.LightWhite(stats.fuzzySkipped) + ") in " + pterm.LightWhite(stats.duration)
	} else if shallowMode {
		finalInfo = "Scanned " + pterm.LightWhite(stats.scannedFiles) + " files by name in " + pterm.LightWhite(stats.duration)
	}
	pterm.Success.Println(finalInfo)

}
