#!/usr/bin/env zsh
set -x

# Use gnu-time. macOS time utility isn't as robust it seems.
/usr/local/bin/gtime -f '%Uu %Ss %er %MkB %C' "$@"
