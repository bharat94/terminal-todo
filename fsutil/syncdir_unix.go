//go:build !windows

package fsutil

import "os"

// SyncDir asks the operating system to persist directory metadata after an
// atomic replace.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
