package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPInitializeAndListTools(t *testing.T) {
	srv := &mcpServer{backend: &server{initialized: true}}

	result, rpcErr := srv.dispatch("initialize", json.RawMessage(`{
		"protocolVersion":"2025-06-18",
		"capabilities":{},
		"clientInfo":{"name":"test-client","version":"1.0.0","websiteUrl":"https://example.com","icons":[]},
		"_meta":{"test":"value"}
	}`))
	require.Nil(t, rpcErr)
	initialized := result.(map[string]interface{})
	assert.Equal(t, mcpProtocolVersion, initialized["protocolVersion"])
	assert.Equal(t, "terminal-todo", initialized["serverInfo"].(map[string]string)["name"])

	_, rpcErr = srv.dispatch("tools/list", json.RawMessage(`{}`))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcNotInitialized, rpcErr.Code)

	result, rpcErr = srv.dispatch("notifications/initialized", json.RawMessage(`{}`))
	require.Nil(t, rpcErr)
	assert.NotNil(t, result)

	result, rpcErr = srv.dispatch("tools/list", json.RawMessage(`{}`))
	require.Nil(t, rpcErr)
	tools := result.(map[string]interface{})["tools"].([]mcpTool)
	require.NotEmpty(t, tools)
	assert.Equal(t, "terminal_todo_ping", tools[0].Name)
	assert.Equal(t, "terminal_todo_events", tools[len(tools)-1].Name)

	names := make(map[string]bool)
	for _, tool := range tools {
		assert.False(t, names[tool.Name], "duplicate MCP tool %q", tool.Name)
		names[tool.Name] = true
		assert.Equal(t, "object", tool.InputSchema["type"])
		assert.Equal(t, false, tool.InputSchema["additionalProperties"])
	}
	assert.Contains(t, names, "terminal_todo_acquire")
	assert.Contains(t, names, "terminal_todo_heartbeat")
	assert.Contains(t, names, "terminal_todo_complete")
}

func TestMCPToolsHaveTitlesAndCompleteAnnotations(t *testing.T) {
	tools := terminalTodoMCPTools()
	require.NotEmpty(t, tools)

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Title)
			assert.False(t, tool.Annotations.OpenWorldHint, "terminal-todo tools operate only on configured project state")

			encoded, err := json.Marshal(tool)
			require.NoError(t, err)
			var wire map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &wire))

			assert.Equal(t, tool.Title, wire["title"])
			assert.NotContains(t, wire, "outputSchema", "do not advertise an incomplete output contract")

			annotations, ok := wire["annotations"].(map[string]interface{})
			require.True(t, ok)
			assert.Len(t, annotations, 4)
			for _, key := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
				value, present := annotations[key]
				assert.True(t, present, "%s must explicitly include %s", tool.Name, key)
				assert.IsType(t, false, value)
			}
		})
	}
}

func TestMCPToolAnnotationsMatchCoordinationEffects(t *testing.T) {
	expected := map[string]mcpToolAnnotations{
		"terminal_todo_ping":      {ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_init":      {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_bootstrap": {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_status":    {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_cat":       {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_add":       {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_acquire":   {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_heartbeat": {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_update":    {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_log":       {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_decompose": {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_block":     {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_release":   {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false},
		"terminal_todo_complete":  {ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		"terminal_todo_events":    {ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
	}

	tools := terminalTodoMCPTools()
	assert.Len(t, tools, len(expected), "every new MCP tool needs an explicit safety classification")
	for _, tool := range tools {
		want, ok := expected[tool.Name]
		require.True(t, ok, "missing expected annotation classification for %s", tool.Name)
		assert.Equal(t, want, tool.Annotations, tool.Name)
	}
}

func TestMCPReadAnnotationsMatchExpiredLeasePersistence(t *testing.T) {
	annotations := make(map[string]mcpToolAnnotations)
	for _, tool := range terminalTodoMCPTools() {
		annotations[tool.Name] = tool.Annotations
	}

	tests := []struct {
		name      string
		arguments string
	}{
		{name: "terminal_todo_bootstrap", arguments: `{"actor":"audit-worker"}`},
		{name: "terminal_todo_status", arguments: `{}`},
		{name: "terminal_todo_cat", arguments: `{"id":1}`},
		{name: "terminal_todo_events", arguments: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, annotations[tt.name].ReadOnlyHint)

			oldRoot := projectRoot
			projectRoot = t.TempDir()
			defer func() { projectRoot = oldRoot }()

			path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
			s := store.NewTaskStore()
			task := s.AddTask("expired MCP work", nil)
			task.Status = store.StatusInProgress
			task.Owner = "expired-worker"
			task.LeaseExpires = uint64(time.Now().Add(-time.Minute).UnixMilli())
			require.NoError(t, s.Save(path))

			srv := &mcpServer{
				backend:        &server{initialized: true},
				initializeSeen: true,
				initialized:    true,
			}
			request := json.RawMessage(`{"name":"` + tt.name + `","arguments":` + tt.arguments + `}`)
			result, rpcErr := srv.dispatch("tools/call", request)
			require.Nil(t, rpcErr)
			assert.False(t, result.(mcpCallResult).IsError)

			persisted, err := store.Load(path)
			require.NoError(t, err)
			assert.Equal(t, store.StatusPending, persisted.Tasks[1].Status)
			assert.Empty(t, persisted.Tasks[1].Owner)
			require.Len(t, persisted.Events, 1)
			assert.Equal(t, store.EventLeaseExpired, persisted.Events[0].Type)
		})
	}

	t.Run("terminal_todo_ping", func(t *testing.T) {
		assert.True(t, annotations["terminal_todo_ping"].ReadOnlyHint)

		oldRoot := projectRoot
		projectRoot = t.TempDir()
		defer func() { projectRoot = oldRoot }()

		path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
		s := store.NewTaskStore()
		task := s.AddTask("expired MCP work", nil)
		task.Status = store.StatusInProgress
		task.Owner = "expired-worker"
		task.LeaseExpires = uint64(time.Now().Add(-time.Minute).UnixMilli())
		require.NoError(t, s.Save(path))

		srv := &mcpServer{
			backend:        &server{initialized: true},
			initializeSeen: true,
			initialized:    true,
		}
		result, rpcErr := srv.dispatch(
			"tools/call",
			json.RawMessage(`{"name":"terminal_todo_ping","arguments":{}}`),
		)
		require.Nil(t, rpcErr)
		assert.False(t, result.(mcpCallResult).IsError)

		persisted, err := store.Load(path)
		require.NoError(t, err)
		assert.Equal(t, store.StatusInProgress, persisted.Tasks[1].Status)
		assert.Equal(t, "expired-worker", persisted.Tasks[1].Owner)
		assert.Empty(t, persisted.Events)
	})
}

func TestMCPToolCallUsesCoordinationBackend(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("MCP allocated work", nil)
	require.NoError(t, s.Save(path))

	srv := &mcpServer{
		backend:        &server{initialized: true},
		initializeSeen: true,
		initialized:    true,
	}
	result, rpcErr := srv.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_acquire",
		"arguments":{"actor":"mcp-worker","requestId":"mcp-allocation-1"}
	}`))
	require.Nil(t, rpcErr)
	call := result.(mcpCallResult)
	assert.False(t, call.IsError)
	require.Len(t, call.Content, 1)
	assert.Equal(t, "text", call.Content[0].Type)
	assert.Equal(t, "Acquired task 1: MCP allocated work.", call.Content[0].Text)
	assert.NotContains(t, call.Content[0].Text, `"schema_version"`)
	structured := call.StructuredContent.(acquireEnvelope)
	assert.Equal(t, "MCP allocated work", structured.Task.Title)
	assert.Equal(t, "mcp-worker", structured.Task.Metadata.Owner)

	persisted, err := store.LoadCurrent(path)
	require.NoError(t, err)
	assert.Equal(t, "mcp-worker", persisted.Tasks[1].Owner)
}

func TestMCPBusinessErrorsAreToolResults(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	require.NoError(t, store.NewTaskStore().Save(path))
	srv := &mcpServer{backend: &server{initialized: true}, initializeSeen: true, initialized: true}

	result, rpcErr := srv.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_acquire",
		"arguments":{"actor":"mcp-worker","requestId":"empty-queue"}
	}`))
	require.Nil(t, rpcErr)
	call := result.(mcpCallResult)
	assert.True(t, call.IsError)
	assert.Equal(t, "NO_WORK: no pending work", call.Content[0].Text)
	detail := call.StructuredContent.(map[string]interface{})
	assert.Equal(t, rpcNoWork, detail["code"])
	assert.Equal(t, "no pending work", detail["message"])
	diagnostics := detail["data"].(allocationDiagnostics)
	assert.Equal(t, allocationNoPendingWork, diagnostics.Reason)
	assert.Zero(t, diagnostics.Queue.Pending)
}

func TestMCPToolTextIsCompactWhileStructuredContentStaysComplete(t *testing.T) {
	tasks := make([]protocolTask, 40)
	for i := range tasks {
		tasks[i] = protocolTask{ID: uint64(i + 1), Title: strings.Repeat("verbose ", 20), Status: "pending"}
	}

	call := newMCPCallResult("terminal_todo_status", tasksEnvelope{
		SchemaVersion: protocolVersion,
		Tasks:         tasks,
	}, false)

	assert.Equal(t, "40 task(s): 40 pending, 0 in progress, 0 blocked, 0 completed.", call.Content[0].Text)
	assert.LessOrEqual(t, len(call.Content[0].Text), maxMCPResultSummaryLength)
	assert.Len(t, call.StructuredContent.(tasksEnvelope).Tasks, 40)
}

func TestMCPToolTextLimitPreservesUTF8(t *testing.T) {
	call := newMCPCallResult("terminal_todo_cat", protocolTask{
		ID:     1,
		Status: "pending",
		Title:  strings.Repeat("界", 200),
	}, false)

	assert.LessOrEqual(t, len(call.Content[0].Text), maxMCPResultSummaryLength)
	assert.True(t, strings.HasSuffix(call.Content[0].Text, "..."))
	assert.True(t, json.Valid([]byte(`"`+call.Content[0].Text+`"`)))
}

func TestMCPRejectsUnknownToolsAndArguments(t *testing.T) {
	srv := &mcpServer{backend: &server{initialized: true}, initializeSeen: true, initialized: true}

	_, rpcErr := srv.dispatch("tools/call", json.RawMessage(`{"name":"shell_exec","arguments":{}}`))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "unknown tool")

	_, rpcErr = srv.dispatch("tools/call", json.RawMessage(`{"name":"terminal_todo_ping","arguments":{},"surprise":true}`))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "unknown field")
}

func TestMCPNotificationDoesNotEmitResponse(t *testing.T) {
	var output bytes.Buffer
	srv := &mcpServer{
		backend: &server{initialized: true},
		encoder: json.NewEncoder(&output),
	}
	input := strings.NewReader(
		"{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1\"}}}\n" +
			"{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\",\"params\":{}}\n" +
			"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n",
	)
	require.NoError(t, srv.readRequests(input))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], `"id":1`)
	assert.Contains(t, lines[1], `"id":2`)
	assert.NotContains(t, output.String(), `"id":null`)
}

func TestMCPRequestValidationAndLargeMessages(t *testing.T) {
	var output bytes.Buffer
	srv := &mcpServer{
		backend: &server{initialized: true},
		encoder: json.NewEncoder(&output),
	}
	padding := strings.Repeat("x", 70*1024)
	input := strings.NewReader(
		"{not-json}\n" +
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"},"unknown":"` + padding + `"}}` + "\n",
	)
	require.NoError(t, srv.readRequests(input))
	assert.Contains(t, output.String(), `"code":-32700`)
	assert.Contains(t, output.String(), `"code":-32602`)
}
