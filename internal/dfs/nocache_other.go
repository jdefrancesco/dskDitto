//go:build !darwin

package dfs

func setNoCacheFD(_ uintptr) error {
	return nil
}
