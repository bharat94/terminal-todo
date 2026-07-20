package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPVisibleNoiseBudgetAcrossCoordinationCycle(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("Implement a bounded coordination cycle", nil)
	require.NoError(t, s.Save(path))

	srv := &mcpServer{
		backend:        &server{initialized: true},
		initializeSeen: true,
		initialized:    true,
	}
	actor := "private-session-actor"
	requestID := "private-idempotency-key"
	calls := []string{
		`{"name":"terminal_todo_acquire","arguments":{"actor":"` + actor + `","requestId":"` + requestID + `","ttl":"30m"}}`,
		`{"name":"terminal_todo_heartbeat","arguments":{"id":1,"actor":"` + actor + `","ttl":"30m","receipt":true}}`,
		`{"name":"terminal_todo_update","arguments":{"id":1,"actor":"` + actor + `","extra":{"finding":"verified the server-controlled visible-output budget"},"receipt":true}}`,
		`{"name":"terminal_todo_complete","arguments":{"ids":[1],"actor":"` + actor + `","receipt":true}}`,
	}

	visibleBytes := 0
	structuredBytes := 0
	for _, raw := range calls {
		result, rpcErr := srv.dispatch("tools/call", json.RawMessage(raw))
		require.Nil(t, rpcErr)
		call := result.(mcpCallResult)
		require.False(t, call.IsError)
		require.Len(t, call.Content, 1)

		text := call.Content[0].Text
		assert.LessOrEqual(t, len(text), maxMCPResultSummaryLength)
		assert.NotContains(t, text, actor)
		assert.NotContains(t, text, requestID)
		assert.NotContains(t, text, `"schema_version"`)
		assert.NotContains(t, text, "\n")
		visibleBytes += len(text)

		encoded, err := json.Marshal(call.StructuredContent)
		require.NoError(t, err)
		if _, ok := call.StructuredContent.(mutationReceipt); ok {
			assert.Less(t, len(encoded), 2_000)
		}
		structuredBytes += len(encoded)
	}

	// This metric covers bytes terminal-todo asks the host to display. The host
	// may still render tool names, arguments, chrome, or structured data.
	assert.LessOrEqual(t, visibleBytes, 240)
	assert.Greater(t, structuredBytes, visibleBytes*4)
}

func TestMCPStatusSummaryDoesNotScaleWithGraphSize(t *testing.T) {
	tasks := make([]protocolTask, 200)
	for i := range tasks {
		tasks[i] = protocolTask{
			ID:     uint64(i + 1),
			Title:  strings.Repeat("coordination detail ", 20),
			Status: "pending",
			Metadata: protocolMetadata{
				Owner: "private-owner",
			},
		}
	}
	envelope := tasksEnvelope{SchemaVersion: protocolVersion, Tasks: tasks}
	call := newMCPCallResult("terminal_todo_status", envelope, false)

	visible := call.Content[0].Text
	structured, err := json.Marshal(call.StructuredContent)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(visible), maxMCPResultSummaryLength)
	assert.NotContains(t, visible, "private-owner")
	assert.Greater(t, len(structured), len(visible)*100)
}
