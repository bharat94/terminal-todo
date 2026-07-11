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
	owner := optionValue(args, "--as")
	errorMsg := optionValue(args, "--error")

	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, ok := s.GetTask(id)
			if !ok {
				return fmt.Errorf("task %d not found", id)
			}
			if task.Status != store.StatusInProgress {
				return fmt.Errorf("task %d is not in progress", id)
			}
			if task.Owner != "" && task.Owner != owner {
				return fmt.Errorf("task %d is claimed by %s; use --as %s", id, task.Owner, task.Owner)
			}
			task.Status = store.StatusPending
			task.RetryCount++
			if errorMsg != "" {
				task.LastError = errorMsg
				s.AddLog(id, owner, fmt.Sprintf("released with error: %s", errorMsg))
			} else {
				s.AddLog(id, owner, "released")
			}
			task.Owner = ""
			task.LeaseExpires = 0
		}
		return nil
	})
	for _, id := range ids {
		fmt.Printf("Released task %d\n", id)
	}
}
