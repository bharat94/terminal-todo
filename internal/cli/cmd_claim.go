package cli

import (
	"errors"
	"fmt"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
)

func cmdClaim(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	owner := optionValue(args, "--as")
	ttl, err := parseLeaseTTL(optionValue(args, "--ttl"))
	if err != nil {
		if errors.Is(err, errInvalidTTL) {
			fail(ErrInvalidArgs, "--ttl %v", err)
		}
		fail(ErrStoreCorrupted, "%v", err)
	}

	if owner == "" {
		fail(ErrInvalidArgs, "--as <owner> is required")
	}
	if err := validateActor(owner, true); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if err := touchAgent(owner); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", owner, err)
	}

	id := ids[0]
	preflight := loadStore()
	var resolver dag.DependencyResolver
	if task, ok := preflight.GetTask(id); ok {
		resolver = snapshotDependencyResolver([]*store.Task{task})
	}
	var retryCount uint32
	var lastError string
	var claimed *store.Task
	updateStore(func(s *store.TaskStore) error {
		task, err := requireTask(s, id)
		if err != nil {
			return err
		}
		// Retry history is read before the transition so the operator sees
		// what the previous attempt left behind.
		retryCount = task.RetryCount
		lastError = task.LastError
		claimed, err = claimTask(s, id, owner, ttl, resolver, projectNow())
		return err
	})
	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("claim", claimed))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(claimed)})
		return
	}

	msg := fmt.Sprintf("Task %d claimed by %s (expires in %s)", id, owner, ttl)
	if retryCount > 0 {
		msg += fmt.Sprintf(" [retry #%d]", retryCount)
	}
	if lastError != "" {
		msg += fmt.Sprintf(" (previous error: %s)", lastError)
	}
	fmt.Println(msg)

}
