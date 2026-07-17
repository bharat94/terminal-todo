package fsutil

import (
	"path/filepath"
	"testing"
)

func TestSyncDir(t *testing.T) {
	if err := SyncDir(filepath.Dir(filepath.Join(t.TempDir(), "state"))); err != nil {
		t.Fatalf("SyncDir: %v", err)
	}
}
