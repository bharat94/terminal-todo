package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdUnblock(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	owner := optionValue(args, "--as")

	var unblocked *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return lifecycleError(ErrTaskNotFound, "task %d not found", ids[0])
		}
		if task.Status != store.StatusBlocked {
			return lifecycleError(ErrInvalidArgs, "task %d is not blocked", ids[0])
		}
		task.Status = store.StatusPending
		task.BlockReason = ""
		// Blocked tasks do not retain ownership. Clear legacy state written by
		// versions that kept a stale lease while blocked.
		task.Owner = ""
		task.LeaseExpires = 0
		s.AddLog(ids[0], owner, "unblocked")
		s.AddEvent(store.EventTaskUnblocked, ids[0], owner, nil)
		unblocked = task
		return nil
	})

	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(unblocked)})
		return
	}
	fmt.Printf("Unblocked task %d\n", ids[0])
}
