package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, string(codexSkill), "Use `terminal-todo` as the source of truth")

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
