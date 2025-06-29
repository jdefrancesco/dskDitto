# TODO



---


* Handle Symlink and Hardlinks. User should be able to decide what to do. But we want
sensible defaults.
* Provide grep and unix friendly text output for piping to other tools.
* File restore. To start we can simpply marshal JSON compressed file in order to restore.
with mininmal overhead.
* Handle permissions error. - Need to log to file


## Features/Flags to Add

* --version flag.
* --headless mode will simply show results with no TUI
* --print0 - read and write file names terminated by null character
* --thorough - force byte by byte comparison
* --dry-run
* --no-recurse
* --zero - exclude empty
* --delete-all
* --exclude
* --max-file-size
* --min-file-size
* --inc-dotfiles
* --throttle - Avoid resource hogging.
* --ignore-hiddens
* --ignore-links or "physical mode" so we dont report links at dups..
* --digest

## Considerations

* Some duplicates are the result of a user having say two different versions of a program, or SDK. They
may need both. So perhapse we might detect this and ask the user if they want to keep both.
* Size capping. Maybe first read a small chunk of a file to determine if we need to
hash the file for real. Most of the time the first 100 bytes of files can deternie if they are dupes
* Defer hashing until two or more files have the same size.
* Decide what we will do if our memory map gets too large.
* Definitely need care handling user configs files.
* Consider using a config file for dskDitto itself.
* Consider BLAKE2 for hash algorithm primary.
* Maybe include fuzzy file diffing. We can tell user if
files are similar but maybe not the same. This could be useful especially for images
and other media.

## Potentiall Issues:

* Consider two pass approach for efficiency.
* Memory usage. We need to be careful about how we handle memory.
* Consider IO usage.
* Handling of permissions gracefully.
