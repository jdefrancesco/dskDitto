# Performance Log


## Hardware


**Machine:** MacBookPro
**OS:** macOS 11.4 (XNU Kernel 20.5.0)
**CPU:** Intel i7-6820HQ (8) @ 2.70Ghz
**Memory:** 16GiB

### Run 1

#### Command

```bash
~/$ dskDitto / # Run dskDitto on home directory.
```

#### Results

Processed 935375 files in 4m48.788 seconds! However there seems to be a huge slowdown
rendering the results as a tree! This is a PTerm issue I may have to resolve myself.

### Run 2

#### Command

```bash
~/$ dskDitto / --cpuprofile homedirscan.prof
```

#### Results

Processed 936224 files in 4m17.877seconds. This is consistent with run one but I also had
profiling data dumped to a file to see where PTerm is getting hung up displaying results.

Pterm tree rendering is definitely the bottleneck. Taking almost several minutes just to display results.

### Run 3

#### Changes

For debugging purposes, and to determine if Pterm tree rendering was responsible for long wait times before results are shown; I
substituted dMap.ShowResults() with debug function dMap.PrintDump()

#### Command

```bash
./xtime.zsh dskDitto /Users/jdefr89
```

#### Results

Process 999,000+ files in only 4 minutes, and total elapsed time is 4 minutes using dMap.PrintDump().
This confirms Pterm tree rendering is extremely slow for some reason.
