package main

import (
	"flag"
	"os"
	"reflect"
	"testing"
)

func TestParseOptions(t *testing.T) {
	originalFlags, originalRegistry, originalArgs := flag.CommandLine, flagRegistry, os.Args
	t.Cleanup(func() {
		flag.CommandLine, flagRegistry, os.Args = originalFlags, originalRegistry, originalArgs
	})

	for _, args := range [][]string{
		{"--remove", "3", "--hash", "blake3", "--exclude", "one", "--exclude", "two", "--text", "--no-symlinks=false", "root"},
		{"-r", "3", "-H", "blake3", "-x", "one", "-x", "two", "-t", "--no-symlinks=false", "root"},
	} {
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		flagRegistry = nil
		os.Args = append([]string{"dskDitto"}, args...)
		opts := parseOptions()
		if opts.keep != 3 || opts.hashAlgo != "blake3" || !opts.textOutput || opts.skipSymLinks {
			t.Fatalf("unexpected parsed options: %+v", opts)
		}
		if !reflect.DeepEqual(opts.excludePaths, stringListFlag{"one", "two"}) || !reflect.DeepEqual(opts.paths, []string{"root"}) {
			t.Fatalf("unexpected paths: exclusions=%v roots=%v", opts.excludePaths, opts.paths)
		}
		if opts.depth != -1 || opts.minDups != 2 || !opts.verifyHash {
			t.Fatalf("command defaults changed: %+v", opts)
		}
	}
}
