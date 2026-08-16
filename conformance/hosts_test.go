package conformance

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCodexHostUsesFixedArgvAndPromptStdin(t *testing.T) {
	prompt := `coordinate the fixture; $(touch escaped) --ask-for-approval untrusted`
	host, err := NewCodexHost(MachineHostOptions{
		Executable:         " /opt/hosts/codex ",
		MCPExecutable:      ` /opt/terminal todo/bin/todo `,
		Version:            " 0.144.5 ",
		Model:              " gpt-test ",
		IntegrationVersion: " v1 ",
		Prompt:             prompt,
	})
	require.NoError(t, err)

	assert.Equal(t, "codex", host.Name)
	assert.Equal(t, "0.144.5", host.Version)
	assert.Equal(t, "gpt-test", host.Model)
	assert.Equal(t, "v1", host.IntegrationVersion)
	assert.Equal(t, "mcp", host.Transport)
	require.NotNil(t, host.Preflight)
	assert.Equal(t, "/opt/hosts/codex", host.Preflight.Executable)
	assert.Equal(t, []string{"login", "status"}, host.Preflight.Args)

	assert.Equal(t, "/opt/hosts/codex", host.Run.Executable)
	assert.Equal(t, ConformancePromptPlaceholder, host.Run.Stdin)
	assert.Equal(t, prompt, host.Run.Prompt)
	assert.NotContains(t, host.Run.Args, prompt)
	assert.Equal(t, []string{
		"--ask-for-approval", "never",
		"exec",
		"--ephemeral",
		"--json",
		"--color", "never",
		"--strict-config",
		"--sandbox", "workspace-write",
		"-C", "{workspace}",
		"--skip-git-repo-check",
		"-c", `mcp_servers.terminal-todo.command="/opt/terminal todo/bin/todo"`,
		"-c", `mcp_servers.terminal-todo.args=["mcp","--stdio"]`,
		"-c", "mcp_servers.terminal-todo.required=true",
		"-c", `mcp_servers.terminal-todo.env_vars=["TERMINAL_TODO_CONFORMANCE_TRACE","TERMINAL_TODO_CONFORMANCE_ACTOR","TERMINAL_TODO_CLOCK_FILE"]`,
	}, host.Run.Args)
	assert.Less(t, slices.Index(host.Run.Args, "--ask-for-approval"), slices.Index(host.Run.Args, "exec"))
	assertHostFailureRule(t, host, FailureAuthentication, "not logged in")
	assertHostFailureRule(t, host, FailureApproval, "approval required")
}

func TestNewClaudeHostUsesOnlyFixtureMCPConfigAndPromptStdin(t *testing.T) {
	prompt := `use terminal-todo; ; rm -rf / --mcp-config /tmp/other.json`
	host, err := NewClaudeHost(MachineHostOptions{
		Executable:         "/opt/hosts/claude",
		Version:            "2.1.215",
		Model:              "claude-test",
		IntegrationVersion: "v1",
		Prompt:             prompt,
	})
	require.NoError(t, err)

	assert.Equal(t, "claude", host.Name)
	assert.Equal(t, "mcp", host.Transport)
	require.NotNil(t, host.Preflight)
	assert.Equal(t, "/opt/hosts/claude", host.Preflight.Executable)
	assert.Equal(t, []string{"auth", "status"}, host.Preflight.Args)

	assert.Equal(t, "/opt/hosts/claude", host.Run.Executable)
	assert.Equal(t, ConformancePromptPlaceholder, host.Run.Stdin)
	assert.Equal(t, prompt, host.Run.Prompt)
	assert.NotContains(t, host.Run.Args, prompt)
	assert.Equal(t, []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--no-session-persistence",
		"--no-chrome",
		"--prompt-suggestions", "false",
		"--permission-mode", "dontAsk",
		"--allowedTools", "mcp__terminal-todo__*",
		"--mcp-config", "{workspace}/.mcp.json",
		"--strict-mcp-config",
	}, host.Run.Args)
	assertHostFailureRule(t, host, FailureApproval, "pending approval")
	assertHostFailureRule(t, host, FailureAuthentication, "invalid api key")
}

func TestMachineHostConstructorsRequireExplicitExecutableAndPrompt(t *testing.T) {
	constructors := map[string]func(MachineHostOptions) (Host, error){
		"codex":  NewCodexHost,
		"claude": NewClaudeHost,
	}
	for name, constructor := range constructors {
		t.Run(name+" executable", func(t *testing.T) {
			_, err := constructor(MachineHostOptions{Prompt: "evaluate"})
			assert.EqualError(t, err, "conformance host executable is required")
		})
		t.Run(name+" prompt", func(t *testing.T) {
			_, err := constructor(MachineHostOptions{Executable: "/host"})
			assert.EqualError(t, err, "conformance host prompt is required")
		})
	}
	_, err := NewCodexHost(MachineHostOptions{Executable: "/host", Prompt: "evaluate"})
	assert.EqualError(t, err, "conformance MCP executable is required")
}

func TestMachineHostConfigPathsRemainInsideFixture(t *testing.T) {
	assert.Equal(t, ".codex/config.toml", CodexProjectConfigFile)
	assert.Equal(t, ".mcp.json", ClaudeProjectMCPConfigFile)
	assert.NotContains(t, CodexProjectConfigFile, "..")
	assert.NotContains(t, ClaudeProjectMCPConfigFile, "..")
}

func TestPersistentCodexHostBuildsSafeResumeCommand(t *testing.T) {
	host, err := NewCodexHost(MachineHostOptions{
		Executable: "/opt/codex", MCPExecutable: "/opt/todo", Prompt: "start", PersistentSessions: true,
	})
	require.NoError(t, err)
	assert.NotContains(t, host.Run.Args, "--ephemeral")
	require.NotNil(t, host.Resume)
	require.NotNil(t, host.ExtractSessionID)

	resume, err := host.Resume("thread-123", `continue; $(touch escaped)`)
	require.NoError(t, err)
	assert.Equal(t, ConformancePromptPlaceholder, resume.Stdin)
	assert.Equal(t, `continue; $(touch escaped)`, resume.Prompt)
	assert.Contains(t, resume.Args, "thread-123")
	assert.NotContains(t, resume.Args, resume.Prompt)
	assert.NotContains(t, resume.Args, "--color")
	assert.Contains(t, resume.Args, `mcp_servers.terminal-todo.env_vars=["TERMINAL_TODO_CONFORMANCE_TRACE","TERMINAL_TODO_CONFORMANCE_ACTOR","TERMINAL_TODO_CLOCK_FILE"]`)
	assert.Equal(t, "-", resume.Args[len(resume.Args)-1])

	line := json.RawMessage(`{"type":"thread.started","thread_id":"thread-123"}`)
	sessionID, matched := host.ExtractSessionID(StreamStdout, line)
	require.True(t, matched)
	assert.Equal(t, "thread-123", sessionID)
}

func TestPersistentClaudeHostBuildsSafeResumeCommand(t *testing.T) {
	host, err := NewClaudeHost(MachineHostOptions{
		Executable: "/opt/claude", Prompt: "start", PersistentSessions: true,
	})
	require.NoError(t, err)
	assert.NotContains(t, host.Run.Args, "--no-session-persistence")
	require.NotNil(t, host.Resume)
	require.NotNil(t, host.ExtractSessionID)

	resume, err := host.Resume("session-123", "continue")
	require.NoError(t, err)
	assert.Equal(t, ConformancePromptPlaceholder, resume.Stdin)
	assert.Equal(t, []string{"--resume", "session-123"}, resume.Args[len(resume.Args)-2:])

	line := json.RawMessage(`{"type":"system","subtype":"init","session_id":"session-123"}`)
	sessionID, matched := host.ExtractSessionID(StreamStdout, line)
	require.True(t, matched)
	assert.Equal(t, "session-123", sessionID)
}

func TestSessionExtractorsRejectMissingMachineReadableID(t *testing.T) {
	_, matched := codexSessionID(StreamStdout, []byte(`{"type":"turn.started"}`))
	assert.False(t, matched)
	_, matched = claudeSessionID(StreamStdout, []byte(`{"type":"assistant"}`))
	assert.False(t, matched)
}

func assertHostFailureRule(t *testing.T, host Host, kind FailureKind, contains string) {
	t.Helper()
	for _, rule := range host.FailureRules {
		if rule.Kind == kind && rule.Contains == contains {
			assert.Equal(t, DispositionSkip, rule.Disposition)
			assert.Equal(t, StreamAny, rule.Stream)
			return
		}
	}
	t.Fatalf("missing %s failure rule for %q", kind, contains)
}
