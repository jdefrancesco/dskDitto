package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"ditto/dfs"
	"ditto/dmap"
	"ditto/dwalk"

	"github.com/gdamore/tcell/v2"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/rivo/tview"
	"github.com/rs/zerolog"
)

var fileLogger zerolog.Logger

func init() {

	// Custom help message
	flag.Usage = func() {
		fmt.Printf("Usage: dskDitto [options] PATHS\n\n")
		flag.PrintDefaults()
	}
}

// signalHandler will handle SIGINT and others in order to
// gracefully shutdown.
func signalHandler(ctx context.Context, sig os.Signal) {
	switch sig {
	case syscall.SIGINT:
		fmt.Printf("\r[!] Signal %s (SIGINT). Quitting....\n", sig.String())
		ctx.Done()
		os.Exit(0)
	default:
		fmt.Printf("\r[!] Unhandled/Unknown signal\n")
		ctx.Done()
		os.Exit(0)
	}
}

func main() {

	// Setup channel for signal handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	// Watch for signal events, call handler to shutdown
	go func() {
		for {
			sig := <-sigChan
			signalHandler(ctx, sig)
		}
	}()

	// Parse command flags.
	var (
		flNoBanner    = flag.Bool("no-banner", false, "Do not show the dskDitto banner")
		flCpuProfile  = flag.String("cpuprofile", "", "Write CPU profile to disk for analysis")
		flNoResults   = flag.Bool("time-only", false, "Use to show only the time taken to scan directory")
		flMaxFileSize = flag.Int64("max-file-size", 1, "Max file size is 1 GiB by default")
	)
	flag.Parse()

	_ = flMaxFileSize

	// Enable CPU profiling
	if *flCpuProfile != "" {
		// Output to file
		f, err := os.Create(*flCpuProfile)
		if err != nil {
			fileLogger.Fatal().Err(err).Msgf("cpuprofile failed")
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Toggle banner.
	if !*flNoBanner {
		showHeader()
	}

	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Allow user to quit dskDitto.
	go func() {
		os.Stdin.Read(make([]byte, 1))
		cancel()
	}()

	// Dmap stores duplicate file information.
	dMap, err := dmap.NewDmap()
	if err != nil {
		fileLogger.Fatal().Msgf("could not create new dmap: %s\n", err)
	}

	// dFiles will be the channel we receive files to be added to the DMap.
	dFiles := make(chan *dfs.Dfile)
	walker := dwalk.NewDWalker(rootDirs, dFiles)

	walker.Run(ctx)
	start := time.Now()

	// Number of files we been sent for processing.
	var nfiles int64
	// Show progress to user at intervals specified by tick.
	tick := time.Tick(time.Duration(500) * time.Millisecond)

	infoSpinner, _ := pterm.DefaultSpinner.Start()

MainLoop:
	for {
		select {
		case <-ctx.Done():
			// Drain dFiles.
			for range dFiles {
			}
			break MainLoop
		case dFile, ok := <-dFiles:
			if !ok {
				break MainLoop
			}
			dMap.Add(dFile)
			nfiles++
		case <-tick:
			// Display progress information.
			progressMsg := fmt.Sprintf("Processed %d files...", nfiles)
			infoSpinner.UpdateText(progressMsg)
		}
	}

	infoSpinner.Stop()
	// Get elapsed time of scan.
	duration := time.Since(start)

	// Scan success message
	finalInfo := "Total of " + pterm.LightWhite(nfiles) + " files processed in " +
		pterm.LightWhite(duration) + ". \nDuplicates: "
	pterm.Success.Println(finalInfo)

	//  FOR DEBUGGING TO TEST SPEED
	if *flNoResults {
		os.Exit(0)
	}

	if dMap.MapSize() < 50 {
		dMap.ShowAllResults()
		os.Exit(0)
	} else {
		var prompt string
		pterm.Success.Print("There are too many results to show. Launch TUI? (y/n) ")
		fmt.Scanf("%s", &prompt)
		// We exit without showing user anything...
		if prompt == "n" {
			os.Exit(0)
		}
	}

	// Launch TUI!
	app := tview.NewApplication()
	// Show tree banner
	tree := tview.NewTreeView().
		SetRoot(tview.NewTreeNode("dskDitto Results").SetSelectable(false))

	addTreeData(tree, dMap)

	// // Set the navigation key bindings.
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		}
		return event
	})
	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		// Expand or collapse the node.
		if node.IsExpanded() {
			node.Collapse()
		} else {
			node.Expand()
		}
	})

	if err := app.SetRoot(tree, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

}

func addTreeData(tree *tview.TreeView, dMap *dmap.Dmap) {
	// Add the header.
	tree.SetRoot(tview.NewTreeNode("dskDitto Results").SetSelectable(false))

	// Add the hash as root node and the files as children.
	for hash, files := range dMap.GetMap() {
		if len(files) > 1 {
			hashNode := tview.NewTreeNode(string(hash)).SetSelectable(true)
			for _, file := range files {
				hashNode.AddChild(tview.NewTreeNode(file)).SetSelectable(true)
			}
			tree.GetRoot().AddChild(hashNode)
		}
	}

}

func addTableData(table *tview.Table, dMap *dmap.Dmap) {
	// Add the header.
	table.SetCell(0, 0, &tview.TableCell{
		Text:            "File",
		NotSelectable:   true,
		Align:           tview.AlignCenter,
		Color:           tview.Styles.PrimaryTextColor,
		BackgroundColor: tview.Styles.ContrastBackgroundColor,
	})
	table.SetCell(0, 1, &tview.TableCell{
		Text:            "Hash",
		NotSelectable:   true,
		Align:           tview.AlignCenter,
		Color:           tview.Styles.PrimaryTextColor,
		BackgroundColor: tview.Styles.ContrastBackgroundColor,
	})

	// Add the data.
	for hash, files := range dMap.GetMap() {
		if len(files) > 1 {
			for _, file := range files {
				rowCount := table.GetRowCount()
				table.SetCell(rowCount, 0, &tview.TableCell{
					Text:            file,
					NotSelectable:   false,
					Align:           tview.AlignLeft,
					Color:           tview.Styles.PrimaryTextColor,
					BackgroundColor: tview.Styles.ContrastBackgroundColor,
				})
				table.SetCell(rowCount, 1, &tview.TableCell{
					Text:            string(hash),
					NotSelectable:   true,
					Align:           tview.AlignLeft,
					Color:           tview.Styles.PrimaryTextColor,
					BackgroundColor: tview.Styles.ContrastBackgroundColor,
				})
			}
		}
	}
}

// showHeader prints colorful dskDitto fileLoggero.
func showHeader() {

	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		putils.LettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}
