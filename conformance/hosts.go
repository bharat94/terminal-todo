package conformance

import (
	"errors"
	"strconv"
	"strings"
)

const (
	// ConformanceWorkspacePlaceholder is expanded by Runner to the disposable
	// fixture workspace. Host commands use it instead of accepting an arbitrary
	// working directory from a scenario prompt.
	ConformanceWorkspacePlaceholder = "{workspace}"
	// ConformancePromptPlaceholder is expanded by Runner from Command.Prompt.
	// Host constructors send it over stdin so prompt text can never become a
	// command-line option.
	ConformancePromptPlaceholder = "{prompt}"

	CodexProjectConfigFile     = ".codex/config.toml"
	ClaudeProjectMCPConfigFile = ".mcp.json"
)

// MachineHostOptions identifies an explicitly installed host and the prompt
// for one opt-in conformance run. Executable is deliberately required: the
// harness must record exactly which binary it evaluated instead of relying on
// shell lookup or aliases.
type MachineHostOptions struct {
	Executable         string
	MCPExecutable      string
	Version            string
	Model              string
	IntegrationVersion string
	Prompt             string
}

// NewCodexHost builds a non-interactive Codex adapter. The project fixture is
// expected to contain CodexProjectConfigFile with a terminal-todo MCP entry.
// The MCP server is required at startup so Codex cannot silently evaluate a
// scenario without the integration under test.
func NewCodexHost(options MachineHostOptions) (Host, error) {
	if err := validateMachineHostOptions(options); err != nil {
		return Host{}, err
	}
	if strings.TrimSpace(options.MCPExecutable) == "" {
		return Host{}, errors.New("conformance MCP executable is required")
	}
	executable := strings.TrimSpace(options.Executable)
	mcpExecutable := strings.TrimSpace(options.MCPExecutable)

	preflight := Command{
		Executable: executable,
		Args:       []string{"login", "status"},
	}
	return Host{
		Name:               "codex",
		Version:            strings.TrimSpace(options.Version),
		Model:              strings.TrimSpace(options.Model),
		Transport:          "mcp",
		IntegrationVersion: strings.TrimSpace(options.IntegrationVersion),
		Preflight:          &preflight,
		Run: Command{
			Executable: executable,
			Args: []string{
				"--ask-for-approval", "never",
				"exec",
				"--ephemeral",
				"--json",
				"--color", "never",
				"--strict-config",
				"--sandbox", "workspace-write",
				"-C", ConformanceWorkspacePlaceholder,
				"--skip-git-repo-check",
				"-c", "mcp_servers.terminal-todo.command=" + strconv.Quote(mcpExecutable),
				"-c", "mcp_servers.terminal-todo.args=[\"mcp\",\"--stdio\"]",
				"-c", "mcp_servers.terminal-todo.required=true",
			},
			Stdin:  ConformancePromptPlaceholder,
			Prompt: options.Prompt,
		},
		FailureRules: []FailureRule{
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "not logged in",
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "authentication_failed",
			},
			{
				Kind: FailureApproval, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "approval required",
			},
		},
	}, nil
}

// NewClaudeHost builds a non-interactive Claude Code adapter. It loads only
// the fixture's project MCP file and allowlists terminal-todo tools. A project
// MCP entry that still needs interactive trust or approval is classified as an
// infrastructure skip by the preflight rule below.
func NewClaudeHost(options MachineHostOptions) (Host, error) {
	if err := validateMachineHostOptions(options); err != nil {
		return Host{}, err
	}
	executable := strings.TrimSpace(options.Executable)

	preflight := Command{
		Executable: executable,
		Args:       []string{"auth", "status"},
	}
	return Host{
		Name:               "claude",
		Version:            strings.TrimSpace(options.Version),
		Model:              strings.TrimSpace(options.Model),
		Transport:          "mcp",
		IntegrationVersion: strings.TrimSpace(options.IntegrationVersion),
		Preflight:          &preflight,
		Run: Command{
			Executable: executable,
			Args: []string{
				"-p",
				"--output-format", "stream-json",
				"--verbose",
				"--no-session-persistence",
				"--no-chrome",
				"--prompt-suggestions", "false",
				"--permission-mode", "dontAsk",
				"--allowedTools", "mcp__terminal-todo__*",
				"--mcp-config", ConformanceWorkspacePlaceholder + "/" + ClaudeProjectMCPConfigFile,
				"--strict-mcp-config",
			},
			Stdin:  ConformancePromptPlaceholder,
			Prompt: options.Prompt,
		},
		FailureRules: []FailureRule{
			{
				Kind: FailureApproval, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "pending approval",
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "not logged in",
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: `"loggedIn":false`,
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: `"loggedIn": false`,
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "authentication_failed",
			},
			{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamAny, Contains: "invalid api key",
			},
		},
	}, nil
}

func validateMachineHostOptions(options MachineHostOptions) error {
	if strings.TrimSpace(options.Executable) == "" {
		return errors.New("conformance host executable is required")
	}
	if strings.TrimSpace(options.Prompt) == "" {
		return errors.New("conformance host prompt is required")
	}
	return nil
}
