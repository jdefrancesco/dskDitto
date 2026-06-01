#!/usr/bin/env zsh

PID=$(pgrep dskDitto)
max=0

while kill -0 $PID 2>/dev/null; do
    cur=$(ls /proc/$PID/fd | wc -l)
    (( cur > max )) && max=$cur
    sleep 0.1
done

echo "Peak FDs: $max"
