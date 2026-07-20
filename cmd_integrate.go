package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed integrations/skills/terminal-todo/SKILL.md integrations/skills/terminal-todo/agents/openai.yaml
var integrationAssets embed.FS

type integrationTarget string

const (
	integrateCodex  integrationTarget = "codex"
	integrateClaude integrationTarget = "claude"
)

type integrationFile struct {
	path    string
	content []byte
	changed bool
}

func cmdIntegrate(args []string) {
	targets, err := parseIntegrationTargets(args)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	command := optionValue(args, "--command")
	if command == "" {
		command = "todo"
	}
	check := hasFlag(args, "--check")
	if hasFlag(args, "--live") && !check {
		fail(ErrInvalidArgs, "--live requires --check")
	}

	files, err := prepareIntegration(projectRoot, targets, command, hasFlag(args, "--force"))
	if err != nil {
		fail(ErrInvalidArgs, "integration: %v", err)
	}

	changed := 0
	for _, file := range files {
		state := "ok"
		if file.changed {
			state = "missing or outdated"
			changed++
			if !check {
				if err := writeIntegrationFile(file.path, file.content); err != nil {
					fail(ErrStoreCorrupted, "writing integration: %v", err)
				}
				state = "installed"
			}
		}
		fmt.Printf("  %-45s %s\n", relativeIntegrationPath(file.path), state)
	}

	if check {
		if changed > 0 {
			fail(ErrInvalidArgs, "%d integration file(s) need installation or update", changed)
		}
		if hasFlag(args, "--live") {
			report, err := verifyMCPIntegration(command, projectRoot)
			if err != nil {
				fail(ErrInvalidArgs, "live integration check failed: %v", err)
			}
			fmt.Printf("Live MCP check passed (%d tools, project %s).\n", report.ToolCount, report.Project)
			return
		}
		fmt.Println("Integration check passed.")
		return
	}
	if changed == 0 {
		fmt.Println("Integrations are already up to date.")
		return
	}
	fmt.Printf("Installed terminal-todo integration for %s.\n", formatIntegrationTargets(targets))
}

type integrationHealth struct {
	ToolCount int
	Project   string
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func verifyMCPIntegration(command, root string) (integrationHealth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, "mcp", "--stdio")
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return integrationHealth{}, fmt.Errorf("starting %q: %w", command, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return integrationHealth{}, fmt.Errorf("starting %q: %w", command, err)
	}
	var stderr synchronizedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return integrationHealth{}, fmt.Errorf("starting %q: %w", command, err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	send := func(request interface{}) error {
		if err := json.NewEncoder(stdin).Encode(request); err != nil {
			return fmt.Errorf("writing MCP request: %w", err)
		}
		return nil
	}
	read := func(id int) (map[string]interface{}, error) {
		if !scanner.Scan() {
			if ctx.Err() != nil {
				return nil, errors.New("MCP handshake timed out")
			}
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("reading MCP response: %w", err)
			}
			detail := strings.TrimSpace(stderr.String())
			if detail != "" {
				return nil, fmt.Errorf("MCP response %d is missing: %s", id, detail)
			}
			return nil, fmt.Errorf("MCP response %d is missing", id)
		}
		var response map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			return nil, fmt.Errorf("invalid MCP response: %w", err)
		}
		if response["jsonrpc"] != "2.0" {
			return nil, fmt.Errorf("MCP response %d has invalid jsonrpc version", id)
		}
		responseID, ok := response["id"].(float64)
		if !ok || int(responseID) != id || responseID != float64(id) {
			return nil, fmt.Errorf("MCP response %d has unexpected id %v", id, response["id"])
		}
		if rpcErr, ok := response["error"]; ok {
			return nil, fmt.Errorf("MCP response %d failed: %v", id, rpcErr)
		}
		return response, nil
	}

	if err := send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]string{"name": "terminal-todo-integrate", "version": "1"},
		},
	}); err != nil {
		return integrationHealth{}, err
	}
	initializeResponse, err := read(1)
	if err != nil {
		return integrationHealth{}, err
	}
	initialize, ok := initializeResponse["result"].(map[string]interface{})
	if !ok || initialize["protocolVersion"] != mcpProtocolVersion {
		return integrationHealth{}, errors.New("MCP protocol negotiation failed")
	}
	serverInfo, ok := initialize["serverInfo"].(map[string]interface{})
	if !ok || serverInfo["name"] != "terminal-todo" {
		return integrationHealth{}, errors.New("MCP server identity is not terminal-todo")
	}
	serverCapabilities, ok := initialize["capabilities"].(map[string]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP server capabilities are missing")
	}
	toolsCapability, ok := serverCapabilities["tools"].(map[string]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP tools capability is missing")
	}
	if _, ok := toolsCapability["listChanged"].(bool); !ok {
		return integrationHealth{}, errors.New("MCP tools capability listChanged flag is missing")
	}

	if err := send(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	}); err != nil {
		return integrationHealth{}, err
	}
	if err := send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}); err != nil {
		return integrationHealth{}, err
	}
	listResponse, err := read(2)
	if err != nil {
		return integrationHealth{}, err
	}
	listResult, ok := listResponse["result"].(map[string]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP tool list is malformed")
	}
	tools, ok := listResult["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		return integrationHealth{}, errors.New("MCP tool list is empty")
	}
	advertisedTools := make(map[string]struct{}, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]interface{})
		if !ok {
			return integrationHealth{}, errors.New("MCP tool list contains a malformed entry")
		}
		name, _ := tool["name"].(string)
		if name == "" {
			return integrationHealth{}, errors.New("MCP tool list contains an unnamed tool")
		}
		if _, duplicate := advertisedTools[name]; duplicate {
			return integrationHealth{}, fmt.Errorf("MCP tool list contains duplicate %q", name)
		}
		advertisedTools[name] = struct{}{}
	}
	for _, expected := range requiredMCPIntegrationTools {
		if _, ok := advertisedTools[expected]; !ok {
			return integrationHealth{}, fmt.Errorf("MCP tool list is missing %q", expected)
		}
	}

	if err := send(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "terminal_todo_ping",
			"arguments": map[string]interface{}{},
		},
	}); err != nil {
		return integrationHealth{}, err
	}
	pingResponse, err := read(3)
	if err != nil {
		return integrationHealth{}, err
	}
	pingResult, ok := pingResponse["result"].(map[string]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP ping result is malformed")
	}
	if isError, _ := pingResult["isError"].(bool); isError {
		return integrationHealth{}, errors.New("MCP ping reported an error")
	}
	structured, ok := pingResult["structuredContent"].(map[string]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP ping structured content is missing")
	}
	project, _ := structured["project"].(string)
	if strings.TrimSpace(project) == "" {
		return integrationHealth{}, errors.New("MCP ping project is empty")
	}
	initialized, _ := structured["initialized"].(bool)
	if !initialized {
		return integrationHealth{}, errors.New("terminal-todo project state is not initialized")
	}
	if version, _ := structured["version"].(string); strings.TrimSpace(version) == "" {
		return integrationHealth{}, errors.New("MCP ping version is empty")
	}
	if structured["protocol_version"] != protocolVersion {
		return integrationHealth{}, fmt.Errorf("terminal-todo protocol mismatch: got %v, expected %s", structured["protocol_version"], protocolVersion)
	}
	capabilities, ok := structured["capabilities"].([]interface{})
	if !ok {
		return integrationHealth{}, errors.New("MCP ping capabilities are missing")
	}
	advertisedCapabilities := make(map[string]struct{}, len(capabilities))
	for _, raw := range capabilities {
		capability, ok := raw.(string)
		if !ok || capability == "" {
			return integrationHealth{}, errors.New("MCP ping capabilities contain a malformed entry")
		}
		if _, duplicate := advertisedCapabilities[capability]; duplicate {
			return integrationHealth{}, fmt.Errorf("MCP ping capabilities contain duplicate %q", capability)
		}
		advertisedCapabilities[capability] = struct{}{}
	}
	for _, expected := range requiredMCPIntegrationCapabilities {
		if _, ok := advertisedCapabilities[expected]; !ok {
			return integrationHealth{}, fmt.Errorf("MCP ping capabilities are missing %q", expected)
		}
	}
	expectedRoot, err := filepath.Abs(root)
	if err != nil {
		return integrationHealth{}, err
	}
	actualRoot, err := filepath.Abs(project)
	if err != nil {
		return integrationHealth{}, err
	}
	if filepath.Clean(actualRoot) != filepath.Clean(expectedRoot) {
		return integrationHealth{}, fmt.Errorf("MCP resolved project %q, expected %q", actualRoot, expectedRoot)
	}

	if err := stdin.Close(); err != nil {
		return integrationHealth{}, fmt.Errorf("closing MCP input: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return integrationHealth{}, errors.New("MCP handshake timed out")
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return integrationHealth{}, fmt.Errorf("MCP server exited: %s", detail)
		}
		return integrationHealth{}, fmt.Errorf("MCP server exited: %w", err)
	}
	finished = true
	return integrationHealth{ToolCount: len(tools), Project: project}, nil
}

var requiredMCPIntegrationCapabilities = []string{
	"dag",
	"leases",
	"lease_heartbeat",
	"atomic_acquire",
	"idempotent_acquire",
	"session_bootstrap",
	"compact_receipts",
	"events",
	"event_pages",
	"cross_repository_dependencies",
}

var requiredMCPIntegrationTools = []string{
	"terminal_todo_ping",
	"terminal_todo_init",
	"terminal_todo_bootstrap",
	"terminal_todo_status",
	"terminal_todo_cat",
	"terminal_todo_add",
	"terminal_todo_acquire",
	"terminal_todo_heartbeat",
	"terminal_todo_update",
	"terminal_todo_log",
	"terminal_todo_decompose",
	"terminal_todo_block",
	"terminal_todo_release",
	"terminal_todo_complete",
	"terminal_todo_events",
}

func parseIntegrationTargets(args []string) ([]integrationTarget, error) {
	target := "all"
	for i := 0; i < len(args); i++ {
		if args[i] == "--command" {
			i++
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if target != "all" {
			return nil, errors.New("specify one integration target: codex, claude, or all")
		}
		target = strings.ToLower(args[i])
	}

	switch target {
	case "all":
		return []integrationTarget{integrateCodex, integrateClaude}, nil
	case "codex":
		return []integrationTarget{integrateCodex}, nil
	case "claude":
		return []integrationTarget{integrateClaude}, nil
	default:
		return nil, fmt.Errorf("unknown integration target %q (expected codex, claude, or all)", target)
	}
}

func prepareIntegration(root string, targets []integrationTarget, command string, force bool) ([]integrationFile, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("MCP command cannot be empty")
	}

	skill, err := integrationAssets.ReadFile("integrations/skills/terminal-todo/SKILL.md")
	if err != nil {
		return nil, err
	}
	openaiYAML, err := integrationAssets.ReadFile("integrations/skills/terminal-todo/agents/openai.yaml")
	if err != nil {
		return nil, err
	}

	var files []integrationFile
	for _, target := range targets {
		switch target {
		case integrateCodex:
			files = append(files,
				integrationFile{path: filepath.Join(root, ".agents", "skills", "terminal-todo", "SKILL.md"), content: skill},
				integrationFile{path: filepath.Join(root, ".agents", "skills", "terminal-todo", "agents", "openai.yaml"), content: openaiYAML},
			)
			configPath := filepath.Join(root, ".codex", "config.toml")
			existing, err := readOptionalFile(configPath)
			if err != nil {
				return nil, err
			}
			config, err := mergeCodexMCPConfig(existing, command, force)
			if err != nil {
				return nil, err
			}
			files = append(files, integrationFile{path: configPath, content: config})
		case integrateClaude:
			files = append(files,
				integrationFile{path: filepath.Join(root, ".claude", "skills", "terminal-todo", "SKILL.md"), content: skill},
				integrationFile{path: filepath.Join(root, ".claude", "skills", "terminal-todo", "agents", "openai.yaml"), content: openaiYAML},
			)
			configPath := filepath.Join(root, ".mcp.json")
			existing, err := readOptionalFile(configPath)
			if err != nil {
				return nil, err
			}
			config, err := mergeClaudeMCPConfig(existing, command, force)
			if err != nil {
				return nil, err
			}
			files = append(files, integrationFile{path: configPath, content: config})
		default:
			return nil, fmt.Errorf("unsupported integration target %q", target)
		}
	}

	for i := range files {
		existing, err := readOptionalFile(files[i].path)
		if err != nil {
			return nil, err
		}
		files[i].changed = !reflect.DeepEqual(existing, files[i].content)
		if len(existing) > 0 && files[i].changed && isSkillPath(files[i].path) && !force {
			return nil, fmt.Errorf("%s differs from the bundled skill; rerun with --force to replace it", relativeIntegrationPath(files[i].path))
		}
	}
	return files, nil
}

func mergeCodexMCPConfig(existing []byte, command string, force bool) ([]byte, error) {
	block := "[mcp_servers.terminal-todo]\ncommand = " + strconv.Quote(command) + "\nargs = [\"mcp\", \"--stdio\"]"
	if len(existing) == 0 {
		return []byte(block + "\n"), nil
	}

	lines := strings.Split(strings.TrimSuffix(string(existing), "\n"), "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[mcp_servers.terminal-todo]" || trimmed == `[mcp_servers."terminal-todo"]` {
			start = i
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
					end = j
					break
				}
			}
			break
		}
	}
	if start < 0 {
		return []byte(strings.TrimRight(string(existing), "\n") + "\n\n" + block + "\n"), nil
	}

	current := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if current == block {
		return existing, nil
	}
	if !force {
		return nil, errors.New(".codex/config.toml already defines mcp_servers.terminal-todo differently; rerun with --force to replace that section")
	}
	replacement := append([]string{}, lines[:start]...)
	replacement = append(replacement, strings.Split(block, "\n")...)
	replacement = append(replacement, lines[end:]...)
	return []byte(strings.TrimRight(strings.Join(replacement, "\n"), "\n") + "\n"), nil
}

func mergeClaudeMCPConfig(existing []byte, command string, force bool) ([]byte, error) {
	config := map[string]interface{}{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &config); err != nil {
			return nil, fmt.Errorf(".mcp.json is not valid JSON: %w", err)
		}
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		if config["mcpServers"] != nil {
			return nil, errors.New(".mcp.json mcpServers must be an object")
		}
		servers = map[string]interface{}{}
		config["mcpServers"] = servers
	}
	desired := map[string]interface{}{
		"type":    "stdio",
		"command": command,
		"args":    []interface{}{"mcp", "--stdio"},
	}
	if current, exists := servers["terminal-todo"]; exists && !reflect.DeepEqual(current, desired) && !force {
		return nil, errors.New(".mcp.json already defines terminal-todo differently; rerun with --force to replace it")
	}
	servers["terminal-todo"] = desired

	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func readOptionalFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return content, err
}

func writeIntegrationFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".terminal-todo-integrate-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func isSkillPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/skills/terminal-todo/")
}

func relativeIntegrationPath(path string) string {
	if relative, err := filepath.Rel(projectRoot, path); err == nil {
		return relative
	}
	return path
}

func formatIntegrationTargets(targets []integrationTarget) string {
	names := make([]string, len(targets))
	for i, target := range targets {
		names[i] = string(target)
	}
	return strings.Join(names, " and ")
}
