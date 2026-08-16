//go:build linux

package dfs

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const reflinkSupported = true

// reflink clones src to dst using the FICLONE ioctl, supported by
// copy-on-write filesystems such as Btrfs and XFS (with reflink=1).
func reflink(dst, src string) error {
	srcFile, err := os.Open(src) // #nosec G304 -- caller-controlled duplicate-group path
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = srcFile.Close() }()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()) // #nosec G304 -- caller-controlled duplicate-group path
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = dstFile.Close() }()

	if err := unix.IoctlFileClone(int(dstFile.Fd()), int(srcFile.Fd())); err != nil {
		_ = os.Remove(dst)
		if isReflinkUnsupportedErrno(err) {
			return fmt.Errorf("%w: FICLONE %s -> %s: %v", ErrReflinkUnsupported, src, dst, err)
		}
		return fmt.Errorf("FICLONE %s -> %s: %w", src, dst, err)
	}
	return nil
}

func isReflinkUnsupportedErrno(err error) bool {
	return errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EINVAL)
}
