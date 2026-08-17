package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bharat94/terminal-todo/store"
)

func cmdUpdate(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	title, hasTitle := optionalValue(args, "--title")
	priorityValue, hasPriority := optionalValue(args, "--priority")
	capsValue, hasCapabilities := optionalValue(args, "--caps")
	owner := optionValue(args, "--as")
	extra, err := parseExtraUpdates(args)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	addDeps := parseRepeatedValues(args, "--add-dep")
	removeDeps := parseRepeatedValues(args, "--remove-dep")

	if !hasTitle && !hasPriority && !hasCapabilities && len(extra) == 0 && len(addDeps) == 0 && len(removeDeps) == 0 {
		fail(ErrInvalidArgs, "provide --title, --priority, --caps, --set, --add-dep, or --remove-dep")
	}
	if hasTitle {
		title = strings.TrimSpace(title)
		if err := validateRequiredPersistedString("title", title, maxTaskTitleBytes); err != nil {
			fail(ErrInvalidArgs, "%v", err)
		}
	}
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if err := validateExtra(extra); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if err := validateDependencies(addDeps); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if err := validateDependencies(removeDeps); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var priority float64
	if hasPriority {
		priority, err = strconv.ParseFloat(priorityValue, 32)
		if err != nil || !validPriority(priority) {
			fail(ErrInvalidArgs, "--priority must be between 0 and 1")
		}
	}
	var capabilities []string
	if hasCapabilities {
		capabilities = normalizeCapabilities(capsValue)
		if err := validateCapabilities(capabilities); err != nil {
			fail(ErrInvalidArgs, "%v", err)
		}
	}

	var titlePointer *string
	if hasTitle {
		titlePointer = &title
	}
	var priorityPointer *float32
	if hasPriority {
		p32 := float32(priority)
		priorityPointer = &p32
	}

	var updated *store.Task
	updateStore(func(s *store.TaskStore) error {
		var updateErr error
		updated, updateErr = updateTask(s, ids[0], owner, taskUpdate{
			Title:           titlePointer,
			Priority:        priorityPointer,
			SetCapabilities: hasCapabilities,
			Capabilities:    capabilities,
			Extra:           extra,
			AddDeps:         addDeps,
			RemoveDeps:      removeDeps,
		})
		return updateErr
	})

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("update", updated))
		return
	}
	if hasFlag(args, "--json") {
		output, err := json.MarshalIndent(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(updated)}, "", "  ")
		if err != nil {
			fail(ErrStoreCorrupted, "Error encoding JSON: %v", err)
		}
		fmt.Println(string(output))
		return
	}
	fmt.Printf("Updated task %d: %s\n", updated.ID, updated.Title)
}

func parseRepeatedValues(args []string, flag string) []string {
	var values []string
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			values = append(values, args[i+1])
		}
	}
	return values
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
