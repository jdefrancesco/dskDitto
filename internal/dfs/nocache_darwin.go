//go:build darwin

package dfs

import "syscall"

func setNoCacheFD(fd int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_NOCACHE), 1)
	if errno != 0 {
		return errno
	}
	return nil
}
