package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_MCPAllocationAndProjectIntegration(t *testing.T) {
	todo := buildTodo(t)
	project := t.TempDir()

	run := func(dir string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(todo, args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%v: %s", args, output)
		return output
	}
	run(project, "init")
	run(project, "add", "Allocate through a real MCP subprocess")

	cmd := exec.Command(todo, "mcp", "--stdio")
	cmd.Dir = project
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	encoder := json.NewEncoder(stdin)
	scanner := bufio.NewScanner(stdout)
	send := func(message interface{}) {
		t.Helper()
		require.NoError(t, encoder.Encode(message))
	}
	read := func() map[string]interface{} {
		t.Helper()
		require.True(t, scanner.Scan(), "MCP process ended early: %v", scanner.Err())
		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &response))
		return response
	}

	send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]string{"name": "e2e-test", "version": "1"},
		},
	})
	initialized := read()
	assert.Equal(t, float64(1), initialized["id"])

	send(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	})
	send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	})
	listed := read()
	tools := listed["result"].(map[string]interface{})["tools"].([]interface{})
	assert.NotEmpty(t, tools)

	send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "terminal_todo_acquire",
			"arguments": map[string]interface{}{
				"actor":     "e2e-worker",
				"requestId": "e2e-request-1",
			},
		},
	})
	acquired := read()
	result := acquired["result"].(map[string]interface{})
	assert.NotEqual(t, true, result["isError"])
	content := result["content"].([]interface{})
	assert.Equal(t, "Acquired task 1: Allocate through a real MCP subprocess.", content[0].(map[string]interface{})["text"])
	structured := result["structuredContent"].(map[string]interface{})
	task := structured["task"].(map[string]interface{})
	assert.Equal(t, "Allocate through a real MCP subprocess", task["title"])

	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())
	status := run(project, "status", "--json")
	assert.Contains(t, string(status), `"owner": "e2e-worker"`)

	integrationProject := t.TempDir()
	run(integrationProject, "integrate", "--command", "todo")
	run(integrationProject, "integrate", "--check", "--command", "todo")
	assert.FileExists(t, filepath.Join(integrationProject, ".codex", "config.toml"))
	assert.FileExists(t, filepath.Join(integrationProject, ".mcp.json"))
}
