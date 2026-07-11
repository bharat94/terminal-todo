package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"terminal-todo/store"
)

type DecomposePayload struct {
	Subtasks []struct {
		Title string   `json:"title"`
		Caps  []string `json:"caps"`
	} `json:"subtasks"`
}

func cmdDecompose(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	var payloadStr string
	for i, arg := range args {
		if arg == "--into" && i+1 < len(args) {
			payloadStr = args[i+1]
			break
		}
	}

	if payloadStr == "" {
		fmt.Fprintln(os.Stderr, "Error: --into <json> is required")
		os.Exit(1)
	}

	var payload DecomposePayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON: %v\n", err)
		os.Exit(1)
	}
	if len(payload.Subtasks) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one subtask is required")
		os.Exit(1)
	}
	for _, sub := range payload.Subtasks {
		if strings.TrimSpace(sub.Title) == "" {
			fmt.Fprintln(os.Stderr, "Error: subtask title is required")
			os.Exit(1)
		}
	}

	parentID := ids[0]
	var added []*store.Task
	updateStore(func(s *store.TaskStore) error {
		parentTask, ok := s.GetTask(parentID)
		if !ok {
			return fmt.Errorf("parent task %d not found", parentID)
		}
		if parentTask.Status == store.StatusCompleted {
			return fmt.Errorf("parent task %d is already completed", parentID)
		}
		for _, sub := range payload.Subtasks {
			subTask := s.AddTask(strings.TrimSpace(sub.Title), nil)
			subTask.Capabilities = sub.Caps
			subTask.Lineage = fmt.Sprintf("todo://local/%d", parentID)
			parentTask.Depends = append(parentTask.Depends, fmt.Sprintf("todo://local/%d", subTask.ID))
			added = append(added, subTask)
		}
		parentTask.Status = store.StatusPending
		parentTask.Owner = ""
		parentTask.LeaseExpires = 0
		s.AddEvent(store.EventTaskDecomposed, parentID, "", map[string]string{"count": fmt.Sprintf("%d", len(payload.Subtasks))})
		return nil
	})
	fmt.Printf("Decomposing task %d into %d subtasks...\n", parentID, len(payload.Subtasks))
	for _, subTask := range added {
		fmt.Printf("  Added subtask %d: %s\n", subTask.ID, subTask.Title)
	}
	fmt.Println("Decomposition complete.")
}
