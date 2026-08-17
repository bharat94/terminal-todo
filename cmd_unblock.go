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
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var unblocked *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		var err error
		unblocked, err = unblockTask(s, ids[0], owner)
		return err
	})

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("unblock", unblocked))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(unblocked)})
		return
	}
	fmt.Printf("Unblocked task %d\n", ids[0])
}
