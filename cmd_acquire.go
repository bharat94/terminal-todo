package main

import (
	"errors"
	"fmt"
	"time"

	"terminal-todo/store"
)

func cmdAcquire(args []string) {
	actor := optionValue(args, "--as")
	if actor == "" {
		fail(ErrInvalidArgs, "--as <owner> is required")
	}
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	ttl := parseDefaultTTL(cfg)
	if value := optionValue(args, "--ttl"); value != "" {
		ttl, err = time.ParseDuration(value)
		if err != nil || ttl <= 0 {
			fail(ErrInvalidArgs, "--ttl must be a positive duration")
		}
	}
	if err := touchAgent(actor); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", actor, err)
	}

	var explicitCapabilities []string
	if value := optionValue(args, "--capabilities"); value != "" {
		explicitCapabilities = normalizeCapabilities(value)
	}
	capabilities, maxLoad, err := agentAllocationProfile(actor, explicitCapabilities)
	if err != nil {
		fail(ErrStoreCorrupted, "loading agent profile: %v", err)
	}
	preflight := loadStore()
	resolver := snapshotDependencyResolver(preflight.GetAllTasks())

	var acquired *store.Task
	_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
		var acquireErr error
		acquired, acquireErr = acquireFromStore(s, actor, ttl, capabilities, maxLoad, resolver)
		return acquireErr
	})
	if err != nil {
		switch {
		case errors.Is(err, errNoReadyTasks):
			fail(ErrDependency, "%v", err)
		case errors.Is(err, errAgentAtCapacity):
			fail(ErrAlreadyClaimed, "%v", err)
		default:
			fail(ErrStoreCorrupted, "acquiring task: %v", err)
		}
	}

	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(acquired)})
		return
	}
	fmt.Printf("Acquired task %d: %s (owner: %s, lease: %s)\n", acquired.ID, acquired.Title, actor, ttl)
}
