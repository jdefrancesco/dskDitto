package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var completionTestFlags = []flagMeta{
	{long: "completion", argName: "shell", usage: "Print a shell completion script."},
	{long: "hash", short: "H", argName: "algo", usage: "Hash algorithm: sha256 or blake3."},
	{long: "file", short: "f", argName: "path", usage: "Only search for this file."},
	{long: "exclude", short: "x", argName: "path", usage: "Exclude a path (repeatable)."},
	{long: "hidden", usage: "Include hidden files."},
	{long: "depth", short: "d", argName: "levels", usage: "Maximum recursion levels."},
}

func TestWriteCompletionRejectsUnsupportedShell(t *testing.T) {
	for _, shell := range []string{"", "fish", "BASH"} {
		t.Run(shell, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeCompletion(&out, shell, completionTestFlags); err == nil {
				t.Fatal("expected unsupported shell error")
			}
			if out.Len() != 0 {
				t.Fatal("unsupported shell wrote output")
			}
		})
	}
}

type failedCompletionWriter struct{ err error }

func (w failedCompletionWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteCompletionReportsWriteError(t *testing.T) {
	want := errors.New("write failed")
	if err := writeCompletion(failedCompletionWriter{want}, "bash", completionTestFlags); !errors.Is(err, want) {
		t.Fatalf("expected wrapped writer error, got %v", err)
	}
}

func TestBashCompletion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	dir := t.TempDir()
	for _, name := range []string{"sample file.txt", "--hidden-path"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sample dir"), 0700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{"long flags", []string{"dskDitto", "--h"}, []string{"--help", "--hash", "--hidden"}},
		{"short alias", []string{"dskDitto", "-H"}, []string{"-H"}},
		{"shell choices", []string{"dskDitto", "--completion", ""}, []string{"bash", "zsh"}},
		{"hash choice", []string{"dskDitto", "--hash", "bl"}, []string{"blake3"}},
		{"short hash choice", []string{"dskDitto", "-H", "sh"}, []string{"sha256"}},
		{"single dash long", []string{"dskDitto", "-hash", "bl"}, []string{"blake3"}},
		{"attached equals", []string{"dskDitto", "--hash=bl"}, []string{"--hash=blake3"}},
		{"readline equals", []string{"dskDitto", "--hash", "=", "bl"}, []string{"blake3"}},
		{"readline empty equals", []string{"dskDitto", "--hash", "="}, []string{"sha256", "blake3"}},
		{"boolean equals", []string{"dskDitto", "--hidden=f"}, []string{"--hidden=false"}},
		{"boolean readline equals", []string{"dskDitto", "--hidden", "=", "f"}, []string{"false"}},
		{"boolean followed by flag", []string{"dskDitto", "--hidden", "--ha"}, []string{"--hash"}},
		{"no flag after path", []string{"dskDitto", "sample dir", "--h"}, []string{"--hidden-path"}},
		{"end of options", []string{"dskDitto", "--", "--h"}, []string{"--hidden-path"}},
		{"numeric value", []string{"dskDitto", "--depth", ""}, nil},
		{"negative numeric value", []string{"dskDitto", "-d", "-"}, nil},
		{"files with spaces", []string{"dskDitto", "--file", "sample f"}, []string{"sample file.txt"}},
		{"attached file", []string{"dskDitto", "--file=sample f"}, []string{"--file=sample file.txt"}},
		{"positional directory", []string{"dskDitto", "sample d"}, []string{"sample dir"}},
		{"value does not stop flags", []string{"dskDitto", "--file", "sample file.txt", "--ha"}, []string{"--hash"}},
		{"equals value does not stop flags", []string{"dskDitto", "--hash", "=", "blake3", "--hi"}, []string{"--hidden"}},
		{"repeatable path", []string{"dskDitto", "-x", "sample dir", "--exclude", "sample f"}, []string{"sample file.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var script strings.Builder
			script.WriteString(bashCompletion(completionTestFlags))
			script.WriteString("\nCOMP_WORDS=(")
			for _, word := range tt.words {
				script.WriteString(shellQuote(word) + " ")
			}
			script.WriteString(")\nCOMP_CWORD=$((${#COMP_WORDS[@]}-1))\n_dskDitto_complete\n")
			script.WriteString("if ((${#COMPREPLY[@]})); then printf '%s\\n' \"${COMPREPLY[@]}\"; fi\n")
			cmd := exec.Command(bash, "--noprofile", "--norc", "-c", script.String())
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("completion failed: %v\n%s", err, out)
			}
			var got []string
			if len(out) > 0 {
				got = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("completions = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZshCompletionLoads(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is unavailable")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "_dskDitto")
	flags := append([]flagMeta(nil), completionTestFlags...)
	flags = append(flags, flagMeta{long: "quoted", argName: "path", usage: "User's file [path]: $(exit 1) `exit 1`."})
	if err := os.WriteFile(script, []byte(zshCompletion(flags)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, command string }{
		{"source", `autoload -Uz compinit; compinit -D; source "$1"; [[ $_comps[dskDitto] == _dskDitto ]] && (( $+functions[_dskDitto] ))`},
		{"autoload", `fpath=("${1:h}" $fpath); autoload -Uz compinit; compinit -D; [[ $_comps[dskDitto] == _dskDitto ]]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := exec.Command(zsh, "-f", "-c", tt.command, "completion-test", script).CombinedOutput()
			if err != nil {
				t.Fatalf("zsh registration failed: %v\n%s", err, out)
			}
		})
	}
}

// Run the real CLI in a test subprocess so flag parsing and os.Exit cannot
// affect the parent test process, and no separate build is needed.
func TestCompletionCLI(t *testing.T) {
	if os.Getenv("DSKDITTO_COMPLETION_TEST") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"dskDitto"}, os.Args[i+1:]...)
				main()
				os.Exit(0)
			}
		}
		os.Exit(2)
	}
	for _, shell := range []string{"bash", "zsh", "fish", ""} {
		t.Run(shell, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCompletionCLI$", "--", "--completion="+shell,
				"--profile=should-not-exist.prof", "--gui", "--link", "--reflink")
			cmd.Env = append(os.Environ(), "DSKDITTO_COMPLETION_TEST=1")
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if shell == "bash" || shell == "zsh" {
				if err != nil || stderr.Len() != 0 {
					t.Fatalf("completion failed: %v\n%s", err, stderr.String())
				}
				if !strings.HasPrefix(stdout.String(), "#") {
					t.Fatalf("expected clean shell script, got %q", stdout.String())
				}
				for _, flag := range []string{"--completion", "--fuzzy-max-candidates", "--exclude", "--restore", "--help", "-H", "-x", "-h"} {
					if !strings.Contains(stdout.String(), flag) {
						t.Errorf("script missing %s", flag)
					}
				}
			} else if err == nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "choose bash or zsh") {
				t.Fatalf("expected unsupported shell failure; err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("completion generation created files: %v (err=%v)", entries, err)
			}
		})
	}
}
