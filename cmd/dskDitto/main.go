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

	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"
	"ditto/internal/dwalk"
	"ditto/internal/ui"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
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

type key int

const (
	loggerKey key = iota
	configKey
	skipSymLinkKey
)

func main() {

	// XXX: TODO. Test global logging!
	dsklog.Dlogger.Debug("Test message")

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
		flNoBanner    = flag.Bool("no-banner", false, "Do not show the dskDitto banner.")
		flCpuProfile  = flag.String("cpuprofile", "", "Write CPU profile to disk for analysis.")
		flNoResults   = flag.Bool("time-only", false, "Use to show only the time taken to scan directory.")
		flMaxFileSize = flag.Int64("max-file-size", 1024*1024*1024*2, "Max file size is 1 GiB by default.")
		// flSkipSymLinks = flag.Bool("no-symlinks", true, "Skip symbolic links. This is on by default.")
	)
	flag.Parse()

	if !*flNoBanner {
		showHeader()
	}

	// Check if user specified max file size.
	var MaxFileSize uint64
	if *flMaxFileSize != 0 {
		if *flMaxFileSize >= 0 {
			MaxFileSize = uint64(*flMaxFileSize)
		}
	} else {
		MaxFileSize = dwalk.MAX_FILE_SIZE
	}

	// Enable CPU profiling
	if *flCpuProfile != "" {
		f, err := os.Create(*flCpuProfile)
		if err != nil {
			log.Fatal("cpuprofile failed")
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	rootDirs := flag.Args()
	if len(rootDirs) == 0 {
		rootDirs = []string{"."}
	}

	// Allow user to quit dskDitto.
	go func() {
		fmt.Println("[+] Press ENTER to stop dskDitto any time.")
		var b [1]byte
		os.Stdin.Read(b[:])
		cancel()
	}()

	// Dmap stores duplicate file information.
	dMap, err := dmap.NewDmap()
	if err != nil {
		log.Println("Failed to make new Dmap: ", err)
		return
	}
	// Recieve files we need to process via this channel.
	dFiles := make(chan *dfs.Dfile)

	walker := dwalk.NewDWalker(rootDirs, dFiles)
	walker.Run(ctx, MaxFileSize)

	start := time.Now()

	// Show progress to user at intervals specified by tick.
	tick := time.Tick(time.Duration(500) * time.Millisecond)
	infoSpinner, _ := pterm.DefaultSpinner.Start()

	// Number of files we need to process.
	var nfiles int64

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
			if dFile == nil {
				dsklog.Dlogger.Warn("Received nil dFile, skipping...")
				continue
			}
			// Add the file to our map.
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
	// if dMap.MapSize() < 200 {
	// 	dMap.ShowAllResults()
	// 	os.Exit(0)
	// } else {
	// 	var prompt string
	// 	fmt.Print("There are too many results to show. Launch TUI? (Y/n): ")
	// 	// Get user input
	// 	fmt.Scanln(&prompt)
	// 	if prompt == "n" || prompt == "N" {
	// 		fmt.Println("got to block")
	// 		os.Exit(0)
	// 	}
	// }
	// dMap.ShowAllResults()
	// Launch interactive TUI to display results.
	ui.LaunchTUI(dMap)
}

// showHeader prints colorful dskDitto fileLoggero.
func showHeader() {

	fmt.Println("")

	pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("dsk", pterm.NewStyle(pterm.FgLightGreen)),
		putils.LettersFromStringWithStyle("Ditto", pterm.NewStyle(pterm.FgLightWhite))).
		Render()
}
