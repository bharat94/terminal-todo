package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdBlock(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	reason := optionValue(args, "--reason")
	if reason == "" {
		fail(ErrInvalidArgs, "--reason <text> is required")
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
		task.BlockReason = reason
		task.Owner = ""
		task.LeaseExpires = 0
		s.AddLog(ids[0], owner, fmt.Sprintf("blocked: %s", reason))
		s.AddEvent(store.EventTaskBlocked, ids[0], owner, map[string]string{"reason": reason})
		return nil
	})

	fmt.Printf("Blocked task %d: %s\n", ids[0], reason)
}
