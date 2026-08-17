package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
	"time"
)

func cmdClaim(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	var owner string
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	ttl := parseDefaultTTL(cfg)

	for i, arg := range args {
		if arg == "--as" && i+1 < len(args) {
			owner = args[i+1]
		}
		if arg == "--ttl" && i+1 < len(args) {
			t, err := time.ParseDuration(args[i+1])
			if err != nil || t <= 0 {
				fail(ErrInvalidArgs, "--ttl must be a positive duration")
			}
			ttl = t
		}
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
