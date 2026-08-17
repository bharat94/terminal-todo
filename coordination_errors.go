package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
)

// Coordination errors are the domain failures that lifecycle operations can
// raise from inside a store transaction. They exist so that every surface
// classifies a failure the same way.
//
// Before these sentinels existed, the CLI routed every transaction error to
// STORE_CORRUPTED and the JSON-RPC server recovered the class by matching
// substrings of the message. Both were wrong in the same direction: an
// ordinary ownership conflict was reported as a corrupt store, and the error
// text became a load-bearing part of the protocol.
//
// Wrap, do not replace. Callers keep their human-readable message and add the
// sentinel with %w so that classification and presentation stay independent.
var (
	errTaskNotFound      = errors.New("task not found")
	errAlreadyClaimed    = errors.New("task is already claimed")
	errNotOwner          = errors.New("task is owned by another agent")
	errDependency        = errors.New("dependency constraint violated")
	errCycleDetected     = errors.New("dependency cycle detected")
	errInvalidTransition = errors.New("invalid lifecycle transition")
)

// classifyCoordinationError maps a domain error to its protocol error code.
// The second result reports whether the error was recognized; an unrecognized
// error is a genuine internal or storage failure and must not be reported as a
// lifecycle outcome.
func classifyCoordinationError(err error) (ErrorCode, bool) {
	if err == nil {
		return "", false
	}
	// Commands migrated earlier carry their code in the error value itself.
	// Both representations are recognized so there is one classifier rather
	// than one per convention.
	var commandErr *lifecycleCommandError
	if errors.As(err, &commandErr) {
		return commandErr.code, true
	}
	switch {
	case errors.Is(err, errTaskNotFound), errors.Is(err, errLeaseTaskNotFound):
		return ErrTaskNotFound, true
	case errors.Is(err, errAlreadyClaimed):
		return ErrAlreadyClaimed, true
	case errors.Is(err, errNotOwner), errors.Is(err, errLeaseNotOwner):
		return ErrNotOwner, true
	case errors.Is(err, errDependency):
		return ErrDependency, true
	case errors.Is(err, errCycleDetected):
		return ErrCycleDetected, true
	case errors.Is(err, errLeaseNotActive):
		return ErrLeaseNotActive, true
	case errors.Is(err, errInvalidTransition):
		return ErrInvalidTransition, true
	// Allocation outcomes are ordinary scheduler control flow, not failures.
	// NO_WORK in particular is the answer a polling worker expects.
	case errors.Is(err, errNoReadyTasks):
		return ErrNoWork, true
	case errors.Is(err, errAgentAtCapacity):
		return ErrAgentAtCapacity, true
	case errors.Is(err, errAcquireRequestConflict):
		return ErrIdempotencyConflict, true
	default:
		return "", false
	}
}

// Shared lifecycle guards. Both the CLI commands and the JSON-RPC handlers
// call these so the two surfaces cannot disagree about what a violation is or
// what it is called.

// requireTask returns the task or a classified not-found error.
func requireTask(s *store.TaskStore, id uint64) (*store.Task, error) {
	task, ok := s.GetTask(id)
	if !ok {
		return nil, fmt.Errorf("task %d not found: %w", id, errTaskNotFound)
	}
	return task, nil
}

// requireOwner rejects mutation of work leased to a different actor. An
// unowned task is mutable by anyone, which is what makes recovery of an
// expired lease possible.
func requireOwner(task *store.Task, actor string) error {
	if task.Owner != "" && task.Owner != actor {
		return fmt.Errorf("task %d is claimed by %s: %w", task.ID, task.Owner, errNotOwner)
	}
	return nil
}

// requireDependenciesComplete rejects work whose local or cross-repository
// prerequisites have not completed.
func requireDependenciesComplete(task *store.Task, tasks map[uint64]*store.Task, resolver dag.DependencyResolver) error {
	if !dag.DependenciesCompleteWithResolver(task, tasks, resolver) {
		return fmt.Errorf("task %d has incomplete dependencies: %w", task.ID, errDependency)
	}
	return nil
}

// requireInProgress rejects yielding or handing off work that holds no lease.
// This mirrors renewLease: a task without an active lease has nothing to
// yield, so LEASE_NOT_ACTIVE is the honest answer.
func requireInProgress(task *store.Task) error {
	if task.Status != store.StatusInProgress {
		return fmt.Errorf("task %d is not in progress: %w", task.ID, errLeaseNotActive)
	}
	return nil
}

// requireClaimableStatus rejects work whose status cannot enter progress.
// Completing is terminal; blocked work must be unblocked first.
func requireClaimableStatus(task *store.Task) error {
	switch task.Status {
	case store.StatusCompleted:
		return fmt.Errorf("task %d is already completed: %w", task.ID, errInvalidTransition)
	case store.StatusBlocked:
		return fmt.Errorf("task %d is blocked: %w", task.ID, errDependency)
	}
	return nil
}

// requireLeaseAvailable rejects work held under another agent's live lease. An
// expired lease is available, which is how a crashed worker's task returns to
// the pool.
func requireLeaseAvailable(task *store.Task, actor string, now time.Time) error {
	nowMillis := uint64(now.UnixMilli())
	if task.Owner != "" && task.Owner != actor && task.LeaseExpires > nowMillis {
		return fmt.Errorf(
			"task %d already claimed by %s (expires in %s): %w",
			task.ID,
			task.Owner,
			time.Duration(task.LeaseExpires-nowMillis)*time.Millisecond,
			errAlreadyClaimed,
		)
	}
	return nil
}

// failCoordination terminates the CLI with the documented code for err. It is
// the CLI mirror of rpcErrorFromDomain: one classifier, two renderings.
func failCoordination(err error, fallback ErrorCode) {
	if isPersistedInputFailure(err) {
		fail(ErrInvalidArgs, "%v", err)
	}
	if code, ok := classifyCoordinationError(err); ok {
		if diagnostics, hasDiagnostics := allocationDiagnosticsFromError(err); hasDiagnostics {
			failData(code, err.Error(), diagnostics)
		}
		fail(code, "%v", err)
	}
	fail(fallback, "%v", err)
}
