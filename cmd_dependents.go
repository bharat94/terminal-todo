package main

import (
	"fmt"
	"os"

	"terminal-todo/store"
)

func cmdDependents(args []string) {
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
	if _, ok := s.GetTask(id); !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
		os.Exit(1)
	}

	var dependents []uint64
	for _, t := range s.GetAllTasks() {
		for _, dep := range t.Depends {
			if dep == id {
				dependents = append(dependents, t.ID)
				break
			}
		}
	}

	if len(dependents) == 0 {
		fmt.Printf("No tasks depend on task %d\n", id)
		return
	}

	fmt.Printf("Tasks that depend on %d:\n", id)
	for _, depID := range dependents {
		task, _ := s.GetTask(depID)
		status := "[ ]"
		if task.Status == store.StatusCompleted {
			status = "[x]"
		}
		fmt.Printf("  %d: %s %s\n", depID, status, task.Title)
	}
}
