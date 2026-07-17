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
	requestID := optionValue(args, "--request-id")
	if err := validateAcquireRequestID(requestID); err != nil {
		fail(ErrInvalidArgs, "--request-id: %v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	ttl := parseDefaultTTL(cfg)
	ttlMode := "default"
	if value := optionValue(args, "--ttl"); value != "" {
		ttl, err = time.ParseDuration(value)
		if err != nil || ttl <= 0 {
			fail(ErrInvalidArgs, "--ttl must be a positive duration")
		}
		ttlMode = "explicit:" + ttl.String()
	}
	var wait time.Duration
	if value := optionValue(args, "--wait"); value != "" {
		wait, err = time.ParseDuration(value)
		if err != nil || wait <= 0 {
			fail(ErrInvalidArgs, "--wait must be a positive duration")
		}
	}
	if err := touchAgent(actor); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", actor, err)
	}

	var explicitCapabilities []string
	capabilitiesMode := "registered"
	if hasFlag(args, "--capabilities") {
		explicitCapabilities = normalizeCapabilities(optionValue(args, "--capabilities"))
		if explicitCapabilities == nil {
			explicitCapabilities = []string{}
		}
		capabilitiesMode = "explicit"
	}
	capabilities, maxLoad, err := agentAllocationProfile(actor, explicitCapabilities)
	if err != nil {
		fail(ErrStoreCorrupted, "loading agent profile: %v", err)
	}
	fingerprint := acquireFingerprint(actor, ttlMode, capabilitiesMode, explicitCapabilities)
	deadline := time.Now().Add(wait)

	var acquired *store.Task
	var replayed bool
	for {
		preflight := loadStore()
		resolver := snapshotDependencyResolver(preflight.GetAllTasks())
		_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
			var acquireErr error
			acquired, replayed, acquireErr = acquireFromStore(s, actor, requestID, fingerprint, ttl, capabilities, maxLoad, resolver)
			return acquireErr
		})
		if err == nil {
			break
		}
		if errors.Is(err, errNoReadyTasks) && wait > 0 {
			remaining := time.Until(deadline)
			if remaining > 0 {
				delay := 250 * time.Millisecond
				if remaining < delay {
					delay = remaining
				}
				time.Sleep(delay)
				continue
			}
		}
		switch {
		case errors.Is(err, errNoReadyTasks):
			fail(ErrNoWork, "%v", err)
		case errors.Is(err, errAgentAtCapacity):
			fail(ErrAgentAtCapacity, "%v", err)
		case errors.Is(err, errAcquireRequestConflict):
			fail(ErrIdempotencyConflict, "%v", err)
		default:
			fail(ErrStoreCorrupted, "acquiring task: %v", err)
		}
	}

	if hasFlag(args, "--json") {
		writeJSON(acquireEnvelope{SchemaVersion: protocolVersion, RequestID: requestID, Replayed: replayed, Task: newProtocolTask(acquired)})
		return
	}
	verb := "Acquired"
	if replayed {
		verb = "Replayed acquisition for"
	}
	fmt.Printf("%s task %d: %s (owner: %s, lease expires: %s)\n", verb, acquired.ID, acquired.Title, actor, formatTimestamp(acquired.LeaseExpires))
}
