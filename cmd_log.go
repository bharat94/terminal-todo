package main

import (
	"fmt"
	"terminal-todo/store"
)

func cmdLog(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	message := optionValue(args, "--msg")
	if message == "" {
		fail(ErrInvalidArgs, "--msg <text> is required")
	}

	owner := optionValue(args, "--as")

	updateStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return fmt.Errorf("task %d not found", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return fmt.Errorf("task %d is claimed by %s; use --as %s", ids[0], task.Owner, task.Owner)
		}
		s.AddLog(ids[0], owner, message)
		return nil
	})

	fmt.Printf("Logged to task %d: %s\n", ids[0], message)
}
