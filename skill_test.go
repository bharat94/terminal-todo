package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBundledTerminalTodoSkillPreservesCoordinationInvariants(t *testing.T) {
	skill, err := os.ReadFile("integrations/skills/terminal-todo/SKILL.md")
	assert.NoError(t, err)
	content := string(skill)

	assert.True(t, strings.HasPrefix(content, "---\nname: terminal-todo\n"))
	assert.Contains(t, content, "description:")
	assert.NotContains(t, content, "TODO")

	for _, required := range []string{
		"todo status --json",
		"todo acquire",
		"--request-id <unique-request-id>",
		"todo heartbeat",
		"todo update",
		"todo decompose",
		"todo done",
		"todo release",
		"todo block",
		"Do not implement allocation as `todo next` followed by `todo claim`",
		"never edit its files directly",
	} {
		assert.Contains(t, content, required)
	}
}

func TestBundledTerminalTodoSkillHasCodexMetadata(t *testing.T) {
	metadata, err := os.ReadFile("integrations/skills/terminal-todo/agents/openai.yaml")
	assert.NoError(t, err)
	content := string(metadata)

	assert.Contains(t, content, `display_name: "Terminal Todo"`)
	assert.Contains(t, content, `short_description: "Coordinate durable multi-agent project work"`)
	assert.Contains(t, content, "$terminal-todo")
	assert.NotContains(t, content, "TODO")
}
