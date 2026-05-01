# Ditto Doc

This document contains explanations on:

    * How dskDitto implements features
    * Core algorithm used for duplicate detection
    * Why I chose certain defaults, etc.

## Default File Size Skip

Running basic python script on various machines yielded an average file size of around
700KiB. This is useful for when I optimize the dwalk and skip large files until end if they
need to be injested.
