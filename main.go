package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
)

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

	// Setup signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())
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

	// Open a file for logging
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Fatal("Failed to open log file: ", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.Println("==================== dskDitto Started ====================")

	// TODO: Not implemented yet.
	_ = flMaxFileSize

	// Enable CPU profiling
	if *flCpuProfile != "" {
		f, err := os.Create(*flCpuProfile)
		if err != nil {
			log.Fatal("cpuprofile failed")
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

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
		log.Println("Failed to make new Dmap: ", err)
		return
	}

	// dFiles will be the channel we receive files to be added to the DMap.
	dFiles := make(chan *dfs.Dfile)

	walker := dwalk.NewDWalker(rootDirs, dFiles)
	walker.Run(ctx)

	// Track our start time..
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
	duration := time.Since(start)

	finalInfo := "Total of " + pterm.LightWhite(nfiles) + " files processed in " +
		pterm.LightWhite(duration)
	pterm.Success.Println(finalInfo)

	// FOR DEBUGGING TO TEST SPEED
	if *flNoResults {
		os.Exit(0)
	}

	// Decide if we can dump results directly or if we need to launch TUI.
	if dMap.MapSize() < 50 {
		dMap.ShowAllResults()
		os.Exit(0)
	} else {
		var prompt string
		pterm.Success.Print("There are too many results to show. Launch TUI? (Y/n): ")
		// Get user input
		fmt.Scanln(&prompt)
		if prompt == "n" {
			os.Exit(0)
		}
	}

	// TODO: Refactor... Launch TUI!
	app := tview.NewApplication()
	tree := tview.NewTreeView().
		SetRoot(tview.NewTreeNode("Duplicates").SetColor(tcell.ColorGreen)).
		SetTopLevel(0)

	tree.SetBorder(true).
		SetBorderPadding(0, 0, 1, 1).
		SetTitleColor(tcell.ColorDeepSkyBlue).
		SetTitle("dskDitto: Interactive Duplicate Management").SetBorderColor(tcell.ColorGreen)

	// Add the nodes to the tree.
	addTreeData(tree, dMap)

	// Map to keep track of marked items
	markedItems := make(map[string]*tview.TreeNode)

	// Key binding to quit.
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				app.Stop()

			// Handle marking items for deletion
			case 'm':
				log.Println("Marking item")
				currentNode := tree.GetCurrentNode()
				log.Printf("Current node: %d", currentNode.GetLevel())
				// Skip selection of root node for now.
				if currentNode.GetLevel() == 1 {
					goto Skip
				}

				if node, ok := markedItems[currentNode.GetText()]; !ok {
					markedItems[currentNode.GetText()] = currentNode
					log.Printf("Marked item: %v", markedItems)
					currentNode.SetColor(tcell.ColorYellow)
				} else {
					delete(markedItems, node.GetText())
					node.SetColor(tcell.ColorWhite)
					log.Printf("Unmarking item: %v", markedItems)
				}

			case 'd':
				// for path, node := range markedItems {
				// 	err := os.Remove(path)
				// 	if err != nil {
				// 		log.Printf("Failed to delete file: %s", err)
				// 	} else {
				// 		node.SetColor(tcell.ColorGray).SetText("[Deleted] " + filepath.Base(path))
				// 	}
				// }
				// markedItems = make(map[string]*tview.TreeNode) // Clear marked items
				app.Draw()
			}

		}
	Skip:
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

	// Launch the TUI.
	if err := app.SetRoot(tree, true).
		EnableMouse(true).
		Run(); err != nil {
		panic(err)
	}

}

// addTreeData adds the duplicate file information to the tree.
func addTreeData(tree *tview.TreeView, dMap *dmap.Dmap) {

	// Get file size in bytes..
	getFileSize := func(file_name string) uint64 {
		file, err := os.Stat(file_name)
		if err != nil {
			return 0
		}
		return uint64(file.Size())
	}

	// Add the hash as root node and the files as children.
	for hash, files := range dMap.GetMap() {
		if len(files) > 1 {
			var fmt_str = "%s - %d Duplicates - (%d bytes total)"
			fSize := getFileSize(files[0])
			totalSize := uint64(fSize) * uint64(len(files))
			header := fmt.Sprintf(fmt_str, hash[:8], len(files), totalSize)
			dupSet := tview.NewTreeNode(header).SetSelectable(true)
			for _, file := range files {
				dupSet.AddChild(tview.NewTreeNode(file)).
					SetColor(tcell.ColorLightGreen)
			}
			tree.GetRoot().AddChild(dupSet)
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
