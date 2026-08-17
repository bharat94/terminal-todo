package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdRelease(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}
	owner := optionValue(args, "--as")
	errorMsg := optionValue(args, "--error")
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if err := validatePersistedString("error", errorMsg, maxErrorBytes); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	released := make([]*store.Task, 0, len(ids))
	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, err := releaseTask(s, id, owner, errorMsg)
			if err != nil {
				return err
			}
			released = append(released, task)
		}
		return nil
	})
	if receiptRequested(args) {
		ids := make([]uint64, 0, len(released))
		for _, task := range released {
			ids = append(ids, task.ID)
		}
		receipt := newMutationReceipt("release", ids)
		if len(released) == 1 {
			receipt.Task = newMutationTaskReference(released[0])
			receipt.DetailFollowUp = taskDetailFollowUp(released[0].ID)
		} else {
			receipt.DetailFollowUp = graphDetailFollowUp()
		}
		writeJSON(receipt)
		return
	}
	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(released))
		for _, task := range released {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks})
		return
	}
	for _, id := range ids {
		fmt.Printf("Released task %d\n", id)
	}
}
