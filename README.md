# Ditto 

## About

Ditto is a tiny utility written in Go that helps identify duplicate/useless files  on your machine
so that you may remove them.


## Design 

Directory Traversal - saracen/walker
TUI - PTerm/PTerm


## Modules

### dutil

Implements dutil object. The object will handle file manipulations such as:
    1. Reading
    2. Hashing
    3. Getting stat info

Lightweight struct simply stores file's:
    1. Name
    2. Size
    3. MD5 Hash


### dmap

Implement out dictionary structure that stores mapping between file md5 hash
and files sharing that key.

{ MD5Hash -> [FileName1, FileName2] }
