//go:build darwin

package dfs

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

const reflinkSupported = true

// reflink clones src to dst using the APFS clonefile(2) syscall.
func reflink(dst, src string) error {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		if isReflinkUnsupportedErrno(err) {
			return fmt.Errorf("%w: clonefile %s -> %s: %v", ErrReflinkUnsupported, src, dst, err)
		}
		return fmt.Errorf("clonefile %s -> %s: %w", src, dst, err)
	}
	return nil
}

func isReflinkUnsupportedErrno(err error) bool {
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EINVAL)
}
