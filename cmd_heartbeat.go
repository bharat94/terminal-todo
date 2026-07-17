package main

import (
	"errors"
	"fmt"
	"time"

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

	id := ids[0]
	var renewed *store.Task
	_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
		var renewErr error
		renewed, renewErr = renewLease(s, id, actor, ttl, time.Now())
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

	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(renewed)})
		return
	}
	fmt.Printf("Renewed task %d lease for %s (expires in %s)\n", id, actor, ttl)
}
