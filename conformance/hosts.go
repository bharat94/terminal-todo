package conformance

import (
	"encoding/json"
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
	PersistentSessions bool
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
	runArgs := []string{
		"--ask-for-approval", "never",
		"exec",
	}
	if !options.PersistentSessions {
		runArgs = append(runArgs, "--ephemeral")
	}
	runArgs = append(runArgs,
		"--json",
		"--color", "never",
		"--strict-config",
		"--sandbox", "workspace-write",
		"-C", ConformanceWorkspacePlaceholder,
		"--skip-git-repo-check",
		"-c", "mcp_servers.terminal-todo.command="+strconv.Quote(mcpExecutable),
		"-c", "mcp_servers.terminal-todo.args=[\"mcp\",\"--stdio\"]",
		"-c", "mcp_servers.terminal-todo.required=true",
	)
	host := Host{
		Name:               "codex",
		Version:            strings.TrimSpace(options.Version),
		Model:              strings.TrimSpace(options.Model),
		Transport:          "mcp",
		IntegrationVersion: strings.TrimSpace(options.IntegrationVersion),
		Preflight:          &preflight,
		Run: Command{
			Executable: executable,
			Args:       runArgs,
			Stdin:      ConformancePromptPlaceholder,
			Prompt:     options.Prompt,
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
	}
	if options.PersistentSessions {
		host.ExtractSessionID = codexSessionID
		host.Resume = func(sessionID, prompt string) (Command, error) {
			if err := validateSessionID(sessionID); err != nil {
				return Command{}, err
			}
			return Command{
				Executable: executable,
				Args: []string{
					"--ask-for-approval", "never",
					"exec", "resume",
					"--json",
					"--strict-config",
					"--skip-git-repo-check",
					"-c", "mcp_servers.terminal-todo.command=" + strconv.Quote(mcpExecutable),
					"-c", "mcp_servers.terminal-todo.args=[\"mcp\",\"--stdio\"]",
					"-c", "mcp_servers.terminal-todo.required=true",
					sessionID, "-",
				},
				Stdin:  ConformancePromptPlaceholder,
				Prompt: prompt,
			}, nil
		}
	}
	return host, nil
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
	runArgs := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
	}
	if !options.PersistentSessions {
		runArgs = append(runArgs, "--no-session-persistence")
	}
	runArgs = append(runArgs,
		"--no-chrome",
		"--prompt-suggestions", "false",
		"--permission-mode", "dontAsk",
		"--allowedTools", "mcp__terminal-todo__*",
		"--mcp-config", ConformanceWorkspacePlaceholder+"/"+ClaudeProjectMCPConfigFile,
		"--strict-mcp-config",
	)
	host := Host{
		Name:               "claude",
		Version:            strings.TrimSpace(options.Version),
		Model:              strings.TrimSpace(options.Model),
		Transport:          "mcp",
		IntegrationVersion: strings.TrimSpace(options.IntegrationVersion),
		Preflight:          &preflight,
		Run: Command{
			Executable: executable,
			Args:       runArgs,
			Stdin:      ConformancePromptPlaceholder,
			Prompt:     options.Prompt,
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
	}
	if options.PersistentSessions {
		host.ExtractSessionID = claudeSessionID
		host.Resume = func(sessionID, prompt string) (Command, error) {
			if err := validateSessionID(sessionID); err != nil {
				return Command{}, err
			}
			args := append([]string(nil), runArgs...)
			args = append(args, "--resume", sessionID)
			return Command{
				Executable: executable,
				Args:       args,
				Stdin:      ConformancePromptPlaceholder,
				Prompt:     prompt,
			}, nil
		}
	}
	return host, nil
}

func validateSessionID(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("conformance host session ID is required")
	}
	if len(sessionID) > 256 {
		return errors.New("conformance host session ID exceeds 256 bytes")
	}
	return nil
}

func codexSessionID(stream Stream, line []byte) (string, bool) {
	if stream != StreamStdout {
		return "", false
	}
	var value struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
	}
	if json.Unmarshal(line, &value) != nil || value.Type != "thread.started" || strings.TrimSpace(value.ThreadID) == "" {
		return "", false
	}
	return strings.TrimSpace(value.ThreadID), true
}

func claudeSessionID(stream Stream, line []byte) (string, bool) {
	if stream != StreamStdout {
		return "", false
	}
	var value struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(line, &value) != nil || strings.TrimSpace(value.SessionID) == "" {
		return "", false
	}
	if (value.Type == "system" && value.Subtype == "init") || value.Type == "result" {
		return strings.TrimSpace(value.SessionID), true
	}
	return "", false
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
