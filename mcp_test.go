package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"terminal-todo/store"

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

	var textResult acquireEnvelope
	require.NoError(t, json.Unmarshal([]byte(call.Content[0].Text), &textResult))
	assert.Equal(t, "MCP allocated work", textResult.Task.Title)
	assert.Equal(t, "mcp-worker", textResult.Task.Metadata.Owner)
	assert.NotNil(t, call.StructuredContent)

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
	assert.Contains(t, call.Content[0].Text, `"code":-32010`)
	assert.Contains(t, call.Content[0].Text, `"message":"no `)
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
