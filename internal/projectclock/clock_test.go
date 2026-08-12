package projectclock

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNowUsesConfiguredClockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clock")
	require.NoError(t, os.WriteFile(path, []byte("2026-01-01T12:34:56.123456789Z\n"), 0o600))
	t.Setenv(EnvironmentVariable, path)

	assert.Equal(t, "2026-01-01T12:34:56.123456789Z", Now().Format(time.RFC3339Nano))
}

func TestNowFallsBackToWallClock(t *testing.T) {
	t.Setenv(EnvironmentVariable, filepath.Join(t.TempDir(), "missing"))
	before := time.Now().UTC().Add(-time.Second)
	got := Now()
	after := time.Now().UTC().Add(time.Second)

	assert.True(t, got.After(before))
	assert.True(t, got.Before(after))
}
