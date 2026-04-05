package main

import (
	"fmt"
	"os"
	"time"

	"terminal-todo/store"
)

func cmdDone(args []string) {
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

	task.Status = store.StatusCompleted
	task.Completed = uint64(time.Now().UnixMilli())
	saveStore(s)

	fmt.Printf("Marked task %d as completed\n", id)
}
