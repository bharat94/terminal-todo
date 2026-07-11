package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"terminal-todo/store"
)

func cmdUpdate(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	title, hasTitle := optionalValue(args, "--title")
	priorityValue, hasPriority := optionalValue(args, "--priority")
	capsValue, hasCapabilities := optionalValue(args, "--caps")
	owner := optionValue(args, "--as")
	extra, err := parseExtraUpdates(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !hasTitle && !hasPriority && !hasCapabilities && len(extra) == 0 {
		fmt.Fprintln(os.Stderr, "Error: provide --title, --priority, --caps, or --set")
		os.Exit(1)
	}
	if hasTitle {
		title = strings.TrimSpace(title)
		if title == "" {
			fmt.Fprintln(os.Stderr, "Error: --title cannot be empty")
			os.Exit(1)
		}
	}

	var priority float64
	if hasPriority {
		priority, err = strconv.ParseFloat(priorityValue, 32)
		if err != nil || priority < 0 || priority > 1 {
			fmt.Fprintln(os.Stderr, "Error: --priority must be between 0 and 1")
			os.Exit(1)
		}
	}
	var capabilities []string
	if hasCapabilities {
		capabilities = normalizeCapabilities(capsValue)
	}

	var updated *store.Task
	updateStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return fmt.Errorf("task %d not found", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return fmt.Errorf("task %d is claimed by %s; use --as %s", task.ID, task.Owner, task.Owner)
		}
		if hasTitle {
			task.Title = title
		}
		if hasPriority {
			task.Priority = float32(priority)
		}
		if hasCapabilities {
			task.Capabilities = capabilities
		}
		if task.Extra == nil {
			task.Extra = make(map[string]string)
		}
		for key, value := range extra {
			task.Extra[key] = value
		}
		updated = task
		return nil
	})

	if hasFlag(args, "--json") {
		output, err := json.MarshalIndent(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(updated)}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
		return
	}
	fmt.Printf("Updated task %d: %s\n", updated.ID, updated.Title)
}

func optionalValue(args []string, option string) (string, bool) {
	for i, arg := range args {
		if arg == option && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func parseExtraUpdates(args []string) (map[string]string, error) {
	updates := make(map[string]string)
	for i, arg := range args {
		if arg != "--set" || i+1 >= len(args) {
			continue
		}
		key, value, found := strings.Cut(args[i+1], "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("--set requires key=value")
		}
		updates[key] = value
	}
	return updates, nil
}

func normalizeCapabilities(value string) []string {
	seen := make(map[string]bool)
	var capabilities []string
	for _, capability := range strings.Split(value, ",") {
		capability = strings.TrimSpace(capability)
		if capability != "" && !seen[capability] {
			seen[capability] = true
			capabilities = append(capabilities, capability)
		}
	}
	return capabilities
}
