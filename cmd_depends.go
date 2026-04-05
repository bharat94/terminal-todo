package main

import (
	"fmt"
	"os"

	"terminal-todo/store"
)

func cmdDepends(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: task ID required\n")
		os.Exit(1)
	}

	var id uint64
	if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[0])
		os.Exit(1)
	}

	s := loadStore()
	task, ok := s.GetTask(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
		os.Exit(1)
	}

	if len(task.Depends) == 0 {
		fmt.Printf("Task %d has no dependencies\n", id)
		return
	}

	fmt.Printf("Task %d depends on:\n", id)
	for _, depID := range task.Depends {
		dep, ok := s.GetTask(depID)
		if ok {
			status := "[ ]"
			if dep.Status == store.StatusCompleted {
				status = "[x]"
			}
			fmt.Printf("  %d: %s %s\n", depID, status, dep.Title)
		} else {
			fmt.Printf("  %d: (not found)\n", depID)
		}
	}
}
