package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// repoRoot walks up from the package directory to the module root.
//
// Tests that read checked-in documentation or build the binary need paths
// relative to the repository, not to this package. Hard-coding "../.." would
// silently break the next time the package moves; finding go.mod does not.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.mod above %s", dir)
		dir = parent
	}
}

// repoFile reads a file relative to the module root.
func repoFile(t *testing.T, elem ...string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(append([]string{repoRoot(t)}, elem...)...))
	require.NoError(t, err)
	return content
}
