package main

import (
	"encoding/json"
	"fmt"
	"os"
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

	s := loadStore()
	parentID := ids[0]
	parentTask, ok := s.GetTask(parentID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: parent task %d not found\n", parentID)
		os.Exit(1)
	}

	fmt.Printf("Decomposing task %d into %d subtasks...\n", parentID, len(payload.Subtasks))
	
	for _, sub := range payload.Subtasks {
		subTask := s.AddTask(sub.Title, []string{})
		subTask.Capabilities = sub.Caps
		subTask.Lineage = fmt.Sprintf("todo://local/%d", parentID)
		
		// Parent now depends on all new subtasks
		parentTask.Depends = append(parentTask.Depends, fmt.Sprintf("todo://local/%d", subTask.ID))
		fmt.Printf("  Added subtask %d: %s\n", subTask.ID, sub.Title)
	}

	parentTask.Status = store.StatusBlocked
	saveStore(s)
	fmt.Println("Decomposition complete.")
}
