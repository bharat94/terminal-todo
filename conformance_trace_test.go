package main

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendConformanceTracePreservesConcurrentMCPCalls(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, filepath.FromSlash(conformance.ConformanceTraceFile))
	var group sync.WaitGroup
	for index := range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := appendConformanceTrace(path, conformance.TraceRecord{
				Actor: "worker", Operation: "acquire", Arguments: map[string]any{"index": index},
			}); err != nil {
				t.Errorf("append trace: %v", err)
			}
		}()
	}
	group.Wait()
	operations, _, err := conformance.ReadTrace(workspace)
	require.NoError(t, err)
	assert.Len(t, operations, 20)
}

func TestRecordConformanceToolCallCapturesCanonicalDomainError(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(conformance.ConformanceTraceEnvironment, filepath.Join(workspace, filepath.FromSlash(conformance.ConformanceTraceFile)))
	t.Setenv(conformance.ConformanceActorEnvironment, "eval-subject")
	recordConformanceToolCall("todo.acquire", json.RawMessage(`{"requestId":"stable"}`), nil, &rpcError{
		Code: rpcNoWork, Message: "no compatible ready work", Data: map[string]any{"reason": "no_ready_tasks"},
	})

	operations, domainErrors, err := conformance.ReadTrace(workspace)
	require.NoError(t, err)
	require.Len(t, operations, 1)
	assert.Equal(t, "eval-subject", operations[0].Actor)
	assert.Equal(t, "stable", operations[0].Arguments["requestId"])
	require.Len(t, domainErrors, 1)
	assert.Equal(t, "NO_WORK", domainErrors[0].Code)
}
