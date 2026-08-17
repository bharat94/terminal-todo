package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/bharat94/terminal-todo/store"
)

func cmdAcquire(args []string) {
	actor := optionValue(args, "--as")
	if actor == "" {
		fail(ErrInvalidArgs, "--as <owner> is required")
	}
	if err := validateActor(actor, true); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	requestID := optionValue(args, "--request-id")
	if err := validateAcquireRequestID(requestID); err != nil {
		fail(ErrInvalidArgs, "--request-id: %v", err)
	}
	var wait time.Duration
	if value := optionValue(args, "--wait"); value != "" {
		var waitErr error
		wait, waitErr = time.ParseDuration(value)
		if waitErr != nil || wait <= 0 {
			fail(ErrInvalidArgs, "--wait must be a positive duration")
		}
	}
	if err := touchAgent(actor); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", actor, err)
	}

	var explicitCapabilities []string
	if hasFlag(args, "--capabilities") {
		explicitCapabilities = normalizeCapabilities(optionValue(args, "--capabilities"))
		if explicitCapabilities == nil {
			explicitCapabilities = []string{}
		}
	}
	plan, err := newAcquirePlan(actor, optionValue(args, "--ttl"), explicitCapabilities)
	if err != nil {
		if errors.Is(err, errInvalidTTL) {
			fail(ErrInvalidArgs, "--ttl %v", err)
		}
		fail(ErrStoreCorrupted, "%v", err)
	}
	deadline := time.Now().Add(wait)

	var acquired *store.Task
	var replayed bool
	for {
		preflight := loadStore()
		resolver := snapshotDependencyResolver(preflight.GetAllTasks())
		_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
			var acquireErr error
			acquired, replayed, acquireErr = acquireFromStore(s, plan.Actor, requestID, plan.Fingerprint, plan.TTL, plan.Capabilities, plan.MaxLoad, resolver)
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
		failCoordination(err, ErrStoreCorrupted)
	}

	if receiptRequested(args) {
		receipt := newTaskMutationReceipt("acquire", acquired)
		receipt.Replayed = &replayed
		writeJSON(receipt)
		return
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
