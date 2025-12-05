#!/usr/bin/env zsh

# Use gnu-time. macOS time utility isn't as robust it seems.
/opt/homebrew/bin/gtime -f '%Uu %Ss %er %MkB %C' "$@"
