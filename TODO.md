# TODO

* Refactor ShowResults to use a single function for all output. The format will depend on output flag we pass it.

1. Add skip zero size files 
2. Handle symlinks similar to duff
3. Decide if we want to process dotfiles (hidden)  by default or make it an option. duff skips.
---

* Update README.md with modern screenshots and examples.

## Features/Flags to Add

* --print0 - read and write file names terminated by null character
* --paranoid - force byte by byte comparison
* --dry-run
* --no-recurse
* --zero - exclude empty
* --delete-all
* --exclude - Exclude directories/files from processing
* --min-size - Skip files smaller than this size
* --hidden
* --throttle - Avoid resource hogging.
* --ignore-links or "physical mode" so we dont report links at dups..
* --digest - Select digest. May want at least three options: BLAKE2, SHA256, MD5.
* --restore - Restore files from a backup.

## Considerations

* Some duplicates are the result of a user having say two different versions of a program, or SDK. They
may need both. So perhaps we might detect this and ask the user if they want to keep both.
* Size capping. Maybe first read a small chunk of a file to determine if we need to
hash the file for real. Most of the time the first 100 bytes of files can deternie if they are dupes
* Decide what we will do if our memory map gets too large.
* Consider BLAKE2 for hash algorithm primary.
* Maybe include fuzzy file diffing. We can tell user if
files are similar but maybe not the same. This could be useful especially for images
and other media.

## Potential Issues

* Consider two pass approach for efficiency.
* Memory usage. We need to be careful about how we handle memory.
* Consider IO usage.
* Adding atomic file operations when we restore/delete.
