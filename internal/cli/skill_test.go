package cli

import (
	"strings"
	"testing"

	"github.com/bharat94/terminal-todo/integrations"

	"github.com/stretchr/testify/assert"
)

func TestBundledTerminalTodoSkillPreservesCoordinationInvariants(t *testing.T) {
	skill := integrations.Skill
	content := string(skill)

	assert.True(t, strings.HasPrefix(content, "---\nname: terminal-todo\n"))
	assert.Contains(t, content, "description:")
	assert.NotContains(t, content, "TODO")

	for _, required := range []string{
		"Prefer the `terminal_todo_*` MCP tools when available",
		"Do not echo raw commands",
		"Do not narrate every coordination call",
		"terminal_todo_acquire",
		"todo status --json",
		"todo acquire",
		"--request-id <unique-request-id>",
		"todo heartbeat",
		"terminal_todo_handoff",
		"todo handoff",
		"todo update",
		"canonical keys `finding`, `decision`, `tests`, `commit`, `files`, and",
		"A log entry alone is not a",
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
	metadata := integrations.OpenAIAgentMetadata
	content := string(metadata)

	assert.Contains(t, content, `display_name: "Terminal Todo"`)
	assert.Contains(t, content, `short_description: "Coordinate durable multi-agent project work"`)
	assert.Contains(t, content, "$terminal-todo")
	assert.NotContains(t, content, "TODO")
}
