//go:build darwin

package dfs

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func setNoCacheFD(fd uintptr) error {
	if uint64(fd) > uint64(math.MaxInt) {
		return fmt.Errorf("file descriptor too large: %d", fd)
	}
	_, err := unix.FcntlInt(fd, unix.F_NOCACHE, 1)
	if err != nil {
		return err
	}
	return nil
}
