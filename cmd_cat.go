package main

import (
	"fmt"
	"os"
	"time"

	"terminal-todo/store"
)

func cmdCat(args []string) {
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

	fmt.Printf("ID:       %d\n", task.ID)
	fmt.Printf("Title:    %s\n", task.Title)
	
	statusStr := "pending"
	if task.Status == store.StatusInProgress {
		statusStr = "in_progress"
	} else if task.Status == store.StatusCompleted {
		statusStr = "completed"
	}
	fmt.Printf("Status:   %s\n", statusStr)
	
	if len(task.Depends) > 0 {
		fmt.Printf("Depends:  %v\n", task.Depends)
	} else {
		fmt.Printf("Depends:  -\n")
	}
	
	fmt.Printf("Created:  %s\n", time.UnixMilli(int64(task.Created)).Format(time.RFC3339))
	
	if task.Completed > 0 {
		fmt.Printf("Completed: %s\n", time.UnixMilli(int64(task.Completed)).Format(time.RFC3339))
	}
}
