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
	if err := validateRequiredPersistedString("reason", reason, maxReasonBytes); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	owner := optionValue(args, "--as")
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var blocked *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		var err error
		blocked, err = blockTask(s, ids[0], owner, reason)
		return err
	})

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("block", blocked))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(blocked)})
		return
	}
	fmt.Printf("Blocked task %d: %s\n", ids[0], reason)
}
