package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"terminal-todo/dag"
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
		fail(ErrInvalidArgs, "task ID required")
	}

	var payloadStr string
	for i, arg := range args {
		if arg == "--into" && i+1 < len(args) {
			payloadStr = args[i+1]
			break
		}
	}

	if payloadStr == "" {
		fail(ErrInvalidArgs, "--into <json> is required")
	}

	var payload DecomposePayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		fail(ErrInvalidArgs, "invalid JSON: %v", err)
	}
	if len(payload.Subtasks) == 0 {
		fail(ErrInvalidArgs, "at least one subtask is required")
	}
	for _, sub := range payload.Subtasks {
		if strings.TrimSpace(sub.Title) == "" {
			fail(ErrInvalidArgs, "subtask title is required")
		}
	}

	parentID := ids[0]
	agent := optionValue(args, "--as")
	var added []*store.Task
	updateStore(func(s *store.TaskStore) error {
		parentTask, ok := s.GetTask(parentID)
		if !ok {
			return fmt.Errorf("parent task %d not found", parentID)
		}
		if parentTask.Status == store.StatusCompleted {
			return fmt.Errorf("parent task %d is already completed", parentID)
		}
		if parentTask.Owner != "" && parentTask.Owner != agent {
			return fmt.Errorf("task %d is claimed by %s (use --as %s to decompose)", parentID, parentTask.Owner, parentTask.Owner)
		}
		for _, sub := range payload.Subtasks {
			subTask := s.AddTask(strings.TrimSpace(sub.Title), nil)
			subTask.Capabilities = sub.Caps
			subTask.Lineage = fmt.Sprintf("todo://local/%d", parentID)
			parentTask.Depends = append(parentTask.Depends, fmt.Sprintf("todo://local/%d", subTask.ID))
			added = append(added, subTask)
		}
		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)
		if err := d.DetectCycle(parentTask.Depends, parentID); err != nil {
			return fmt.Errorf("decompose would create a cycle: %w", err)
		}
		parentTask.Status = store.StatusPending
		if agent != "" {
			parentTask.Owner = agent
		} else {
			parentTask.Owner = ""
		}
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
