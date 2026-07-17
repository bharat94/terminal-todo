package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("TERMINAL_TODO_FAKE_MCP"); mode != "" {
		os.Exit(runFakeMCPServer(mode))
	}
	os.Exit(m.Run())
}

func TestPrepareIntegrationInstallsAndChecksBothClients(t *testing.T) {
	root := t.TempDir()
	targets := []integrationTarget{integrateCodex, integrateClaude}

	files, err := prepareIntegration(root, targets, "todo", false)
	require.NoError(t, err)
	require.Len(t, files, 6)
	for _, file := range files {
		assert.True(t, file.changed, file.path)
		require.NoError(t, writeIntegrationFile(file.path, file.content))
	}

	codexSkill, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "terminal-todo", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(codexSkill), "Use terminal-todo as the source of truth")

	claudeSkill, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "terminal-todo", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, codexSkill, claudeSkill)

	codexConfig, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(codexConfig), "[mcp_servers.terminal-todo]")
	assert.Contains(t, string(codexConfig), `args = ["mcp", "--stdio"]`)

	var claudeConfig map[string]interface{}
	content, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &claudeConfig))
	server := claudeConfig["mcpServers"].(map[string]interface{})["terminal-todo"].(map[string]interface{})
	assert.Equal(t, "stdio", server["type"])
	assert.Equal(t, "todo", server["command"])

	checked, err := prepareIntegration(root, targets, "todo", false)
	require.NoError(t, err)
	for _, file := range checked {
		assert.False(t, file.changed, file.path)
	}
}

func TestPrepareIntegrationPreservesUnrelatedConfiguration(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".codex"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".codex", "config.toml"),
		[]byte("model = \"gpt-5\"\n"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mcp.json"),
		[]byte(`{"custom":{"keep":true},"mcpServers":{"other":{"command":"other"}}}`),
		0644,
	))

	files, err := prepareIntegration(root, []integrationTarget{integrateCodex, integrateClaude}, "/usr/local/bin/todo", false)
	require.NoError(t, err)
	for _, file := range files {
		require.NoError(t, writeIntegrationFile(file.path, file.content))
	}

	codex, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(codex), `model = "gpt-5"`)
	assert.Contains(t, string(codex), `command = "/usr/local/bin/todo"`)

	var claude map[string]interface{}
	content, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &claude))
	assert.Equal(t, true, claude["custom"].(map[string]interface{})["keep"])
	assert.Contains(t, claude["mcpServers"].(map[string]interface{}), "other")
}

func TestPrepareIntegrationRefusesConflictsWithoutPartialWrites(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, ".agents", "skills", "terminal-todo", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0755))
	require.NoError(t, os.WriteFile(skillPath, []byte("local customization\n"), 0644))

	_, err := prepareIntegration(root, []integrationTarget{integrateCodex}, "todo", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differs from the bundled skill")
	assert.NoFileExists(t, filepath.Join(root, ".codex", "config.toml"))

	files, err := prepareIntegration(root, []integrationTarget{integrateCodex}, "todo", true)
	require.NoError(t, err)
	for _, file := range files {
		require.NoError(t, writeIntegrationFile(file.path, file.content))
	}
	replaced, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Contains(t, string(replaced), "## Acquire work safely")
}

func TestIntegrationConfigConflictsRequireForce(t *testing.T) {
	codexExisting := []byte("[mcp_servers.terminal-todo]\ncommand = \"custom\"\nargs = []\n")
	_, err := mergeCodexMCPConfig(codexExisting, "todo", false)
	require.Error(t, err)
	replaced, err := mergeCodexMCPConfig(codexExisting, "todo", true)
	require.NoError(t, err)
	assert.Contains(t, string(replaced), `command = "todo"`)
	assert.NotContains(t, string(replaced), `command = "custom"`)

	claudeExisting := []byte(`{"mcpServers":{"terminal-todo":{"command":"custom"}}}`)
	_, err = mergeClaudeMCPConfig(claudeExisting, "todo", false)
	require.Error(t, err)
	replaced, err = mergeClaudeMCPConfig(claudeExisting, "todo", true)
	require.NoError(t, err)
	assert.Contains(t, string(replaced), `"command": "todo"`)
}

func TestParseIntegrationTargets(t *testing.T) {
	targets, err := parseIntegrationTargets(nil)
	require.NoError(t, err)
	assert.Equal(t, []integrationTarget{integrateCodex, integrateClaude}, targets)

	targets, err = parseIntegrationTargets([]string{"claude", "--check"})
	require.NoError(t, err)
	assert.Equal(t, []integrationTarget{integrateClaude}, targets)

	_, err = parseIntegrationTargets([]string{"cursor"})
	require.Error(t, err)
}

func TestVerifyMCPIntegrationChecksToolsAndProjectRoot(t *testing.T) {
	root := setupTestProject(t)
	binary := buildTodo(t)

	report, err := verifyMCPIntegration(binary, root)
	require.NoError(t, err)
	assert.Greater(t, report.ToolCount, 10)
	assert.Equal(t, root, report.Project)
}

func TestRequiredMCPIntegrationToolsMatchCuratedSurface(t *testing.T) {
	actual := make([]string, 0, len(terminalTodoMCPTools()))
	for _, tool := range terminalTodoMCPTools() {
		actual = append(actual, tool.Name)
	}
	assert.ElementsMatch(t, actual, requiredMCPIntegrationTools)
}

func TestVerifyMCPIntegrationRejectsMissingCommand(t *testing.T) {
	_, err := verifyMCPIntegration(filepath.Join(t.TempDir(), "missing-todo"), t.TempDir())
	assert.ErrorContains(t, err, "starting")
}

func TestVerifyMCPIntegrationRequiresInitializedProject(t *testing.T) {
	_, err := verifyMCPIntegration(buildTodo(t), t.TempDir())
	assert.ErrorContains(t, err, "not initialized")
}

func TestVerifyMCPIntegrationSequencesInitializeBeforeOtherRequests(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TERMINAL_TODO_FAKE_MCP", "strict")
	t.Setenv("TERMINAL_TODO_FAKE_MCP_PROJECT", root)

	report, err := verifyMCPIntegration(os.Args[0], root)
	require.NoError(t, err)
	assert.Equal(t, len(terminalTodoMCPTools()), report.ToolCount)
	assert.Equal(t, root, report.Project)
}

func TestVerifyMCPIntegrationRejectsStaleToolSet(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TERMINAL_TODO_FAKE_MCP", "stale-tools")
	t.Setenv("TERMINAL_TODO_FAKE_MCP_PROJECT", root)

	_, err := verifyMCPIntegration(os.Args[0], root)
	assert.ErrorContains(t, err, `missing "terminal_todo_bootstrap"`)
}

func TestVerifyMCPIntegrationRejectsStaleCapabilities(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TERMINAL_TODO_FAKE_MCP", "stale-capabilities")
	t.Setenv("TERMINAL_TODO_FAKE_MCP_PROJECT", root)

	_, err := verifyMCPIntegration(os.Args[0], root)
	assert.ErrorContains(t, err, `capabilities are missing "session_bootstrap"`)
}

func TestVerifyMCPIntegrationRejectsEmptyProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TERMINAL_TODO_FAKE_MCP", "empty-project")
	t.Setenv("TERMINAL_TODO_FAKE_MCP_PROJECT", root)

	_, err := verifyMCPIntegration(os.Args[0], root)
	assert.ErrorContains(t, err, "project is empty")
}

func TestVerifyMCPIntegrationAcceptsBoundedLargeResponses(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TERMINAL_TODO_FAKE_MCP", "large-response")
	t.Setenv("TERMINAL_TODO_FAKE_MCP_PROJECT", root)

	_, err := verifyMCPIntegration(os.Args[0], root)
	require.NoError(t, err)
}

func runFakeMCPServer(mode string) int {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	initialize, err := readFakeMCPRequest(reader)
	if err != nil {
		return fakeMCPFailure(err)
	}
	if initialize["method"] != "initialize" || initialize["id"] != float64(1) {
		return fakeMCPFailure(fmt.Errorf("expected initialize request, got %v", initialize))
	}

	type readResult struct {
		request map[string]interface{}
		err     error
	}
	next := make(chan readResult, 1)
	go func() {
		request, err := readFakeMCPRequest(reader)
		next <- readResult{request: request, err: err}
	}()
	select {
	case early := <-next:
		if early.err != nil {
			return fakeMCPFailure(early.err)
		}
		return fakeMCPFailure(errors.New("request was pipelined before the initialize response"))
	case <-time.After(150 * time.Millisecond):
	}

	if err := encoder.Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"result": map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]interface{}{"name": "terminal-todo", "version": "test"},
		},
	}); err != nil {
		return fakeMCPFailure(err)
	}

	initialized := <-next
	if initialized.err != nil {
		return fakeMCPFailure(initialized.err)
	}
	if initialized.request["method"] != "notifications/initialized" {
		return fakeMCPFailure(fmt.Errorf("expected initialized notification, got %v", initialized.request))
	}
	list, err := readFakeMCPRequest(reader)
	if err != nil {
		return fakeMCPFailure(err)
	}
	if list["method"] != "tools/list" || list["id"] != float64(2) {
		return fakeMCPFailure(fmt.Errorf("expected tools/list request, got %v", list))
	}

	tools := make([]map[string]string, 0, len(terminalTodoMCPTools()))
	for _, tool := range terminalTodoMCPTools() {
		if mode == "stale-tools" && tool.Name == "terminal_todo_bootstrap" {
			continue
		}
		entry := map[string]string{"name": tool.Name}
		if mode == "large-response" && len(tools) == 0 {
			entry["description"] = strings.Repeat("x", 128*1024)
		}
		tools = append(tools, entry)
	}
	if err := encoder.Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"result":  map[string]interface{}{"tools": tools},
	}); err != nil {
		return fakeMCPFailure(err)
	}

	ping, err := readFakeMCPRequest(reader)
	if err != nil {
		return fakeMCPFailure(err)
	}
	if ping["method"] != "tools/call" || ping["id"] != float64(3) {
		return fakeMCPFailure(fmt.Errorf("expected tools/call request, got %v", ping))
	}
	params, _ := ping["params"].(map[string]interface{})
	if params["name"] != "terminal_todo_ping" {
		return fakeMCPFailure(fmt.Errorf("expected terminal_todo_ping, got %v", params["name"]))
	}

	capabilities := append([]string(nil), requiredMCPIntegrationCapabilities...)
	if mode == "stale-capabilities" {
		for i, capability := range capabilities {
			if capability == "session_bootstrap" {
				capabilities = append(capabilities[:i], capabilities[i+1:]...)
				break
			}
		}
	}
	project := os.Getenv("TERMINAL_TODO_FAKE_MCP_PROJECT")
	if mode == "empty-project" {
		project = ""
	}
	if err := encoder.Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"result": map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "terminal-todo ready"}},
			"structuredContent": map[string]interface{}{
				"version":          "test",
				"protocol_version": protocolVersion,
				"project":          project,
				"initialized":      true,
				"capabilities":     capabilities,
			},
		},
	}); err != nil {
		return fakeMCPFailure(err)
	}
	return 0
}

func readFakeMCPRequest(reader *bufio.Reader) (map[string]interface{}, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var request map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &request); err != nil {
		return nil, err
	}
	return request, nil
}

func fakeMCPFailure(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 2
}
