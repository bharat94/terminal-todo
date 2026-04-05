package main

import (
	"encoding/json"
	"fmt"
	"os"
	"terminal-todo/store"
)

func cmdCat(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	s := loadStore()
	task, ok := s.GetTask(ids[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", ids[0])
		os.Exit(1)
	}

	if hasFlag(args, "--json") {
		output, _ := json.MarshalIndent(task, "", "  ")
		fmt.Println(string(output))
		return
	}

	fmt.Printf("ID:         %d\n", task.ID)
	fmt.Printf("Title:      %s\n", task.Title)
	statusStr := "Pending"
	switch task.Status {
	case store.StatusInProgress:
		statusStr = "In-Progress"
	case store.StatusCompleted:
		statusStr = "Completed"
	case store.StatusBlocked:
		statusStr = "Blocked"
	}
	fmt.Printf("Status:     %s\n", statusStr)
	fmt.Printf("Owner:      %s\n", task.Owner)
	fmt.Printf("Depends:    %v\n", task.Depends)
	fmt.Printf("Caps:       %v\n", task.Capabilities)
	fmt.Printf("Priority:   %.2f\n", task.Priority)
	fmt.Printf("Lineage:    %s\n", task.Lineage)
}
