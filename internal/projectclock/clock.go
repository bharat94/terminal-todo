// Package projectclock provides the wall clock used for persisted project
// timestamps and lease decisions.
package projectclock

import (
	"os"
	"strings"
	"time"
)

// EnvironmentVariable points at a harness-controlled RFC3339 timestamp file.
// It is intentionally process-scoped so real-agent conformance can advance
// time across CLI and MCP child processes without changing production state.
const EnvironmentVariable = "TERMINAL_TODO_CLOCK_FILE"

// Now returns the harness clock when configured and valid, otherwise the real
// UTC wall clock. Production processes do not set EnvironmentVariable.
func Now() time.Time {
	path := strings.TrimSpace(os.Getenv(EnvironmentVariable))
	if path == "" {
		return time.Now().UTC()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Now().UTC()
	}
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Now().UTC()
	}
	return value.UTC()
}
