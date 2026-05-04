//go:build !darwin

package dfs

func setNoCacheFD(_ int) error {
	return nil
}
