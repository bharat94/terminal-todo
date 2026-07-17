//go:build windows

package fsutil

// SyncDir is a no-op on Windows. Go cannot open directory handles with the
// access required by Sync, so callers still flush the file and atomically
// replace it but cannot explicitly flush the containing directory.
func SyncDir(string) error {
	return nil
}
