package conformance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/internal/projectclock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureClockAdvancesWithoutSleeping(t *testing.T) {
	workspace := t.TempDir()
	fixture := ClockFixture(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	path := filepath.Join(workspace, filepath.FromSlash(fixture.Path))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, fixture.Content, fixture.Mode))

	advanced, err := AdvanceClock(workspace, 90*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01T12:01:30Z", advanced.Format(time.RFC3339))
	current, err := ReadClock(workspace)
	require.NoError(t, err)
	assert.Equal(t, advanced, current)
	assert.Equal(t, "{workspace}/.terminal-todo/conformance-clock", ClockEnvironment()[projectclock.EnvironmentVariable])
}

func TestFixtureClockRejectsNonPositiveAdvance(t *testing.T) {
	_, err := AdvanceClock(t.TempDir(), 0)
	assert.EqualError(t, err, "clock advance must be positive")
}
