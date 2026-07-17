package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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

	files, err := prepareIntegration(projectRoot, targets, command, hasFlag(args, "--force"))
	if err != nil {
		fail(ErrInvalidArgs, "integration: %v", err)
	}

	check := hasFlag(args, "--check")
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
		fmt.Println("Integration check passed.")
		return
	}
	if changed == 0 {
		fmt.Println("Integrations are already up to date.")
		return
	}
	fmt.Printf("Installed terminal-todo integration for %s.\n", formatIntegrationTargets(targets))
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
