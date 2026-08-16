//go:build !darwin && !linux

package dfs

const reflinkSupported = false

func reflink(_, _ string) error {
	return ErrReflinkUnsupported
}
