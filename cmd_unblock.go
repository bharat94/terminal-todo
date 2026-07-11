package main

import (
	"fmt"
	"os"

	"terminal-todo/store"
)

func cmdUnblock(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	owner := optionValue(args, "--as")

	updateStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return fmt.Errorf("task %d not found", ids[0])
		}
		if task.Status != store.StatusBlocked {
			return fmt.Errorf("task %d is not blocked", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return fmt.Errorf("task %d is claimed by %s; use --as %s", ids[0], task.Owner, task.Owner)
		}
		task.Status = store.StatusPending
		s.AddLog(ids[0], owner, "unblocked")
		return nil
	})

	fmt.Printf("Unblocked task %d\n", ids[0])
}
