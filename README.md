# dskDitto 

## About

DskDitto is a tiny utility written in Go that helps identify duplicate/useless files on your machine
in only a matter of seconds. It is highly concurrent, and efficient. The project is still in its infancy
so many features are still yet to be implemented. 

**WARNING:** Although in the future this tool will be cross-platform, I have only been testing/developing it on macOS.

## Screenshots

![dskDitto-1](./ss/dskDitto-ss-latest.png)

![dslDotto-2](./ss/dskDitto-ss.png)

## Building

**NOTE:** A Makefile will be created soon. For now you simply have to call `go build` yourself. 

```bash
$ git clone https://github.com/jdefrancesco/dskDitto && cd dskDitto
$ go build 
$ mv ditto dskDitto # The executable created is called ditto which will conflict with another macOS utility called ditto. Simply rename it for now.
```

## Contributing

TODO

