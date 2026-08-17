package main

import (
	"errors"
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdHeartbeat(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}
	actor := optionValue(args, "--as")
	if actor == "" {
		fail(ErrInvalidArgs, "--as <owner> is required")
	}
	if err := validateActor(actor, true); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	ttl, err := parseLeaseTTL(optionValue(args, "--ttl"))
	if err != nil {
		if errors.Is(err, errInvalidTTL) {
			fail(ErrInvalidArgs, "--ttl %v", err)
		}
		fail(ErrStoreCorrupted, "%v", err)
	}
	if err := touchAgent(actor); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", actor, err)
	}

	id := ids[0]
	var renewed *store.Task
	_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
		var renewErr error
		renewed, renewErr = renewLease(s, id, actor, ttl, projectNow())
		return renewErr
	})
	if err != nil {
		switch {
		case errors.Is(err, errLeaseTaskNotFound):
			fail(ErrTaskNotFound, "%v", err)
		case errors.Is(err, errLeaseNotOwner):
			fail(ErrNotOwner, "%v", err)
		case errors.Is(err, errLeaseNotActive):
			fail(ErrLeaseNotActive, "%v", err)
		default:
			fail(ErrStoreCorrupted, "%v", err)
		}
	}

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("heartbeat", renewed))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(renewed)})
		return
	}
	fmt.Printf("Renewed task %d lease for %s (expires in %s)\n", id, actor, ttl)
}
