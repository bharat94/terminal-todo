package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTraceNormalizesOperationsAndDomainErrors(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, filepath.FromSlash(ConformanceTraceFile))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	records := []TraceRecord{
		{Actor: "eval-alpha", Operation: "acquire", Arguments: map[string]any{"requestId": "stable"}, Result: map[string]any{"task": map[string]any{"id": float64(1)}}},
		{Actor: "eval-beta", Operation: "acquire", Arguments: map[string]any{"requestId": "other"}, Error: &DomainError{Code: "NO_WORK", Message: "none", Data: map[string]any{"reason": "no_ready_tasks"}}},
	}
	file, err := os.Create(path)
	require.NoError(t, err)
	encoder := json.NewEncoder(file)
	for _, record := range records {
		require.NoError(t, encoder.Encode(record))
	}
	require.NoError(t, file.Close())

	operations, domainErrors, err := ReadTrace(workspace)
	require.NoError(t, err)
	require.Len(t, operations, 2)
	assert.Equal(t, "eval-alpha", operations[0].Actor)
	assert.Equal(t, "acquire", operations[0].Operation)
	assert.Equal(t, "stable", operations[0].Arguments["requestId"])
	require.Len(t, domainErrors, 1)
	assert.Equal(t, "NO_WORK", domainErrors[0].Code)
	assert.Equal(t, "acquire", domainErrors[0].Operation)
}

func TestReadTraceAllowsScenarioWithNoCoordinationCalls(t *testing.T) {
	operations, domainErrors, err := ReadTrace(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, operations)
	assert.Empty(t, domainErrors)
}
