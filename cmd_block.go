package main

import (
	"fmt"
	"os"

	"terminal-todo/store"
)

func cmdBlock(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	reason := optionValue(args, "--reason")
	if reason == "" {
		fmt.Fprintln(os.Stderr, "Error: --reason <text> is required")
		os.Exit(1)
	}

	owner := optionValue(args, "--as")

	updateStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return fmt.Errorf("task %d not found", ids[0])
		}
		if task.Status == store.StatusCompleted {
			return fmt.Errorf("task %d is already completed", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return fmt.Errorf("task %d is claimed by %s; use --as %s", ids[0], task.Owner, task.Owner)
		}
		task.Status = store.StatusBlocked
		s.AddLog(ids[0], owner, fmt.Sprintf("blocked: %s", reason))
		return nil
	})

	fmt.Printf("Blocked task %d: %s\n", ids[0], reason)
}
