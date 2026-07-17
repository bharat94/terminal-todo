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

	var blocked *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return lifecycleError(ErrTaskNotFound, "task %d not found", ids[0])
		}
		if task.Status == store.StatusCompleted {
			return lifecycleError(ErrInvalidArgs, "task %d is already completed", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return lifecycleError(ErrNotOwner, "task %d is claimed by %s; use --as %s", ids[0], task.Owner, task.Owner)
		}
		task.Status = store.StatusBlocked
		task.BlockReason = reason
		task.Owner = ""
		task.LeaseExpires = 0
		s.AddLog(ids[0], owner, fmt.Sprintf("blocked: %s", reason))
		s.AddEvent(store.EventTaskBlocked, ids[0], owner, map[string]string{"reason": reason})
		blocked = task
		return nil
	})

	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(blocked)})
		return
	}
	fmt.Printf("Blocked task %d: %s\n", ids[0], reason)
}
