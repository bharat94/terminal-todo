package main

import (
	"fmt"
	"os"
	"terminal-todo/store"
)

func cmdRelease(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	s := loadStore()
	for _, id := range ids {
		task, ok := s.GetTask(id)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
			continue
		}
		if task.Status != store.StatusInProgress {
			fmt.Fprintf(os.Stderr, "Error: task %d is not in progress\n", id)
			continue
		}
		task.Status = store.StatusPending
		task.Owner = ""
		task.LeaseExpires = 0
		fmt.Printf("Released task %d\n", id)
	}
	saveStore(s)
}
