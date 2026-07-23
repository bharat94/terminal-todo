package conformance

import (
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
