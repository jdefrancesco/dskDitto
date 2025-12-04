# dskDitto

`dskDitto` is a fast, parallel duplicate-file detector with an optional interactive terminal UI that lets you review, keep, or safely delete redundant files.

## Features

- Concurrent directory walker tuned for large trees and multi-core systems
- SHA-256 (default) or BLAKE3 hashing with smart size filtering
- Multiple output modes: TUI, pretty tree, bullet lists, or text-friendly dumps
- Optional automated duplicate removal with confirmation safety rails
- Profiling toggles and micro-benchmarks for power users

## Install

Install straight from source using Go 1.22+:

```bash
go install github.com/jdefrancesco/dskDitto/cmd/dskDitto@latest
```

This drops the binary at `$(go env GOPATH)/bin/dskDitto` (or `~/go/bin` by default).

Prefer cloning? Build locally via Make:

```bash
git clone https://github.com/jdefrancesco/dskDitto
cd dskDitto
make          # builds into ./bin/dskDitto
```

The resulting binary lives in `bin/dskDitto`. Add it to your `$PATH` or run it from the repo root.

## Usage

```bash
./bin/dskDitto [options] PATH...
```

Common flags:

| Flag | Description |
| ---- | ----------- |
| `--min-size <bytes>` | Ignore files smaller than the provided size |
| `--max-size <bytes>` | Skip files larger than the provided size (default 4 GiB) |
| `--hidden` | Include dot files and dot-directories |
| `--no-symlinks` | Skip symbolic links |
| `--empty` | Include zero-byte files |
| `--include-vfs` | Include virtual filesystem directories such as `/proc` or `/dev` |
| `--no-recurse` | Restrict the scan to the provided paths only |
| `--depth <levels>` | Limit recursion to `<levels>` directories below the starting paths |
| `--text`, `--bullet`, `--pretty` | Render duplicates without launching the TUI |
| `--remove <keep>` | Delete duplicates, keeping the first `<keep>` entries per group |

Press `Ctrl+C` at any time to abort a scan. When duplicates are removed, a confirmation dialog prevents accidental mass deletion.

## Configuration

- **Log level:** set `DSKDITTO_LOG_LEVEL` to `debug`, `info`, `warn`, etc.
- **Default options:** wrap `dskDitto` in a shell alias or script with your favorite defaults.
- **Profiling:** supply `--pprof host:port` to expose Go's `pprof` endpoints while the tool runs.

## Screenshots

### `dskDitto` rendered as a table

![Screenshot: pretty table output](./ss/ss-pretty.png)

### TUI for interactively selecting files to remove or keep

![Screenshot: interactive TUI](./ss/ss-tui.png)

### Confirmation window keeps you from deleting the wrong files

![Confirmation dialog screenshot](./ss/ss-confirm.png)

### Legacy UI shots

![Legacy screenshot 3](./ss/dskDitto-ss-one.png)

![Legacy screenshot 4](./ss/dskDitto-ss-two.png)

## Development

```bash
make debug         # Create development build 
make test          # go test ./...
make bench         # run benchmarks (adds -benchmem)
make bench-profile # capture cpu.prof and mem.prof into the repo root
make pprof-web     # launch go tool pprof with HTTP UI for the latest profile
```

The TODO backlog lives in `TODO.md`.

## Contributing

Issues and PRs are welcome. Open an issue if you have ideas for improvements, new output modes, or performance tweaks.

## License

This project is released under the Apache license. See [`LICENSE`](LICENSE) for details.
