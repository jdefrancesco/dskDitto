# dskDitto

## About

DskDitto is a tiny utility written in Go that helps identify duplicate/useless files on your machine
in only a matter of seconds. It is highly concurrent, and efficient. The project is still in it's infancy
so many features are yet to be implemented. I wouldn't use the tool until I complete the currently standing
TODO list.

## TODO

* Increase efficiency for determining duplicates. Right now we just hastily MD5 a file and keep a giant hashmap. This won't scale well
for large disks, or machines with minimal resources.
* Handling of soft/hard symlinks and permissions.
* Handling of large files.
* Deal appropriately with collisions.
* Create appropriate UI for letting the user decide what to trash and what to keep.


**WARNING:** Although in the future this tool will be cross-platform, I have only been testing/developing it on macOS.

## Screenshots

![dskDitto-1](./ss/dskDitto-ss-one.png)

![dslDotto-2](./ss/dskDitto-ss-two.png)

## Building

Running the following commands will create a new executable `dskDitto`.

```bash
$ git clone https://github.com/jdefrancesco/dskDitto && cd dskDitto
$ make
```

## Contributing

If you want to work on this just let me know. I don't have a ton of time to dedicate to this, and I might get bored
of it all together, who knows?
