package dwalk

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
)

func TestNewDWalk(t *testing.T) {

	rootDirs := []string{"test_files"}
	ctx, cancel := context.WithCancel(context.Background())

	dFiles := make(chan int64)
	walker := NewDWalker(rootDirs, dFiles)
	// walker
	walker.Run(ctx)

	var nfiles, nbytes int64
	tick := time.Tick(500 * time.Millisecond)

loop:
	for {
		select {
		case <-ctx.Done():
			for range dFiles {
			}
			break loop
		case size, ok := <-dFiles:
			if !ok {
				break loop
			}
			nfiles++
			if nfiles >= 3 {
				cancel()
			}
			nbytes += size
		case <-tick:
			log.Info().Msgf("Tick...")
		}
	}

	// Print the results periodically.
	fmt.Printf("%d files %.1f GB\n", nfiles, float64(nbytes)/1e9)

}
