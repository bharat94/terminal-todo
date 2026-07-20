package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
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

	var logged *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(ids[0])
		if !ok {
			return lifecycleError(ErrTaskNotFound, "task %d not found", ids[0])
		}
		if task.Owner != "" && task.Owner != owner {
			return lifecycleError(ErrNotOwner, "task %d is claimed by %s; use --as %s", ids[0], task.Owner, task.Owner)
		}
		s.AddLog(ids[0], owner, message)
		logged = task
		return nil
	})

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("log", logged))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(logged)})
		return
	}
	fmt.Printf("Logged to task %d: %s\n", ids[0], message)
}
