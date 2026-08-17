package main

import (
	"fmt"
	"time"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
)

// Lifecycle operations.
//
// Each function is the single implementation of one state transition. It runs
// inside a store transaction, validates through the shared guards, applies the
// mutation, and records the log entry and audit event. It renders nothing and
// decides nothing about presentation.
//
// Surfaces are adapters over these functions: the CLI parses flags and prints,
// the JSON-RPC server decodes parameters and builds envelopes, and MCP wraps
// the JSON-RPC dispatch table. Because the transition itself exists once,
// the surfaces cannot drift apart on what a transition does, which fields it
// touches, or which events it emits.
//
// renewLease in lease.go is the same shape and predates this file.

// claimTask leases a specific ready task to actor.
//
// Guards run in a deliberate order: a terminal or blocked status is reported
// before an unmet prerequisite, and an unmet prerequisite before a competing
// lease. The most fundamental reason the task cannot be worked is the one the
// caller hears about.
func claimTask(
	s *store.TaskStore,
	id uint64,
	actor string,
	ttl time.Duration,
	resolver dag.DependencyResolver,
	now time.Time,
) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireClaimableStatus(task); err != nil {
		return nil, err
	}
	if err := requireDependenciesComplete(task, s.Tasks, resolver); err != nil {
		return nil, err
	}
	if err := requireLeaseAvailable(task, actor, now); err != nil {
		return nil, err
	}

	task.Owner = actor
	task.Status = store.StatusInProgress
	task.LeaseExpires = uint64(now.UnixMilli()) + uint64(ttl.Milliseconds())
	s.AddLog(id, actor, "claimed")
	s.AddEvent(store.EventTaskClaimed, id, actor, map[string]string{"ttl": ttl.String()})
	return task, nil
}

// completeTask marks owned work complete and clears its lease.
//
// An unowned task may be completed by anyone, which is what lets a human close
// out work a crashed agent left behind.
func completeTask(
	s *store.TaskStore,
	id uint64,
	actor string,
	resolver dag.DependencyResolver,
	now time.Time,
) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireDependenciesComplete(task, s.Tasks, resolver); err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.Status = store.StatusCompleted
	task.Completed = uint64(now.UnixMilli())
	task.Owner = ""
	task.LeaseExpires = 0
	task.BlockReason = ""
	s.AddEvent(store.EventTaskCompleted, id, actor, nil)
	return task, nil
}

// releaseTask yields an owned lease back to the pool.
//
// A release counts as a failed attempt: it increments the retry count and, when
// a reason is supplied, records it as the task's last error so the next worker
// inherits the finding. handoffTask exists for the case where a worker yields
// deliberately and should not be charged a retry.
func releaseTask(s *store.TaskStore, id uint64, actor, failure string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireInProgress(task); err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.RetryCount++
	data := map[string]string{}
	if failure != "" {
		task.LastError = failure
		data["error"] = failure
		s.AddLog(id, actor, fmt.Sprintf("released with error: %s", failure))
	} else {
		s.AddLog(id, actor, "released")
	}
	s.AddEvent(store.EventTaskReleased, id, actor, data)
	task.Owner = ""
	task.LeaseExpires = 0
	task.Status = store.StatusPending
	return task, nil
}

// blockTask records a blocker and releases ownership.
//
// Ownership is dropped deliberately: recovery of blocked work must not depend
// on the worker that discovered the blocker still being alive.
func blockTask(s *store.TaskStore, id uint64, actor, reason string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if task.Status == store.StatusCompleted {
		return nil, fmt.Errorf("task %d is already completed: %w", id, errInvalidTransition)
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.Status = store.StatusBlocked
	task.BlockReason = reason
	task.Owner = ""
	task.LeaseExpires = 0
	s.AddLog(id, actor, fmt.Sprintf("blocked: %s", reason))
	s.AddEvent(store.EventTaskBlocked, id, actor, map[string]string{"reason": reason})
	return task, nil
}

// unblockTask returns blocked work to the pending pool.
//
// Blocked tasks hold no lease, so the owner fields are cleared rather than
// checked: stores written before blocking released ownership can still carry a
// stale lease, and unblocking is the point at which that legacy state is
// repaired.
func unblockTask(s *store.TaskStore, id uint64, actor string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if task.Status != store.StatusBlocked {
		return nil, fmt.Errorf("task %d is not blocked: %w", id, errInvalidTransition)
	}

	task.Status = store.StatusPending
	task.BlockReason = ""
	task.Owner = ""
	task.LeaseExpires = 0
	s.AddLog(id, actor, "unblocked")
	s.AddEvent(store.EventTaskUnblocked, id, actor, nil)
	return task, nil
}

// logTask appends a finding to a task's audit trail without changing its
// lifecycle state. Writing to work leased by another agent is refused, because
// the log is the handoff record and must stay attributable.
func logTask(s *store.TaskStore, id uint64, actor, message string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	s.AddLog(id, actor, message)
	return task, nil
}
