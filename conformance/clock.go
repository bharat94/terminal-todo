package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharat94/terminal-todo/internal/projectclock"
)

const fixtureClockPath = ".terminal-todo/conformance-clock"

// ClockFixture creates the deterministic clock file consumed by terminal-todo
// child processes in a conformance workspace.
func ClockFixture(initial time.Time) FixtureFile {
	return FixtureFile{
		Path:    fixtureClockPath,
		Content: []byte(initial.UTC().Format(time.RFC3339Nano) + "\n"),
		Mode:    0o600,
	}
}

// ClockEnvironment configures a host and its MCP children to use the fixture
// clock. The runner expands the workspace placeholder immediately before exec.
func ClockEnvironment() map[string]string {
	return map[string]string{
		projectclock.EnvironmentVariable: filepath.ToSlash(filepath.Join("{workspace}", fixtureClockPath)),
	}
}

// ReadClock returns the current deterministic time for a materialized fixture.
func ReadClock(workspace string) (time.Time, error) {
	path := filepath.Join(workspace, filepath.FromSlash(fixtureClockPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("read conformance clock: %w", err)
	}
	current, err := time.Parse(time.RFC3339Nano, stringTrimSpace(data))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse conformance clock: %w", err)
	}
	return current.UTC(), nil
}

// AdvanceClock atomically advances a materialized fixture clock without
// sleeping. It rejects non-positive movement to keep scenario time monotonic.
func AdvanceClock(workspace string, by time.Duration) (time.Time, error) {
	if by <= 0 {
		return time.Time{}, fmt.Errorf("clock advance must be positive")
	}
	current, err := ReadClock(workspace)
	if err != nil {
		return time.Time{}, err
	}
	next := current.Add(by).UTC()
	path := filepath.Join(workspace, filepath.FromSlash(fixtureClockPath))
	temporary, err := os.CreateTemp(filepath.Dir(path), ".conformance-clock-*")
	if err != nil {
		return time.Time{}, fmt.Errorf("create conformance clock update: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return time.Time{}, fmt.Errorf("secure conformance clock update: %w", err)
	}
	if _, err := temporary.WriteString(next.Format(time.RFC3339Nano) + "\n"); err != nil {
		temporary.Close()
		return time.Time{}, fmt.Errorf("write conformance clock update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return time.Time{}, fmt.Errorf("close conformance clock update: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return time.Time{}, fmt.Errorf("replace conformance clock: %w", err)
	}
	return next, nil
}

func stringTrimSpace(value []byte) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return string(value[start:end])
}
