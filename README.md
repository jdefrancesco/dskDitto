# dskDitto

## About

`DskDitto` is a tiny utility written in Go that helps identify duplicate/useless files on your machine
in only a matter of seconds. It is highly concurrent, and efficient. Results are displayed in a slick looking TUI 
that allows you to select what you to keep or clobber. 


## TODO

See TODO.txt


## Screenshots


### `dskDitto` invoked with only to show duplicates as table.

![dskDitto-0](./ss/ss-pretty.png)

### `dskDitto` has awesome TUI for interactively choosing files to remove or keep.

![dskDitto-1](./ss/ss-tui.png)

### `dskDitto` confirmation window ensures you don't shoot yourself in foot!

![dskDitto-2](./ss/ss-confirm.png)

### `dskDitto` screen shots (older)

![dskDitto-3](./ss/dskDitto-ss-one.png)

![dslDotto-4](./ss/dskDitto-ss-two.png)

## Building

Running the following commands will create a new executable `dskDitto`.

```bash
$ git clone https://github.com/jdefrancesco/dskDitto && cd dskDitto
$ make
```

## Benchmarking & Profiling

- `make bench` runs the in-tree micro-benchmarks.
- `make bench-profile` builds the benchmark binary and captures `cpu.prof` and `mem.prof` so you can inspect them later.
- `make pprof-web PROFILE=cpu.prof` launches `go tool pprof` with the web UI (change `PROFILE` or `PPROF_ADDR` as needed).
- At runtime you can expose live profiling handlers by adding `--pprof localhost:6060` (or any host:port) when starting `dskDitto`, then open `http://localhost:6060/debug/pprof/` in your browser.

## Contributing

If you want to work on this just let me know. I don't have a ton of time to dedicate to this, and I might get bored
of it all together..
