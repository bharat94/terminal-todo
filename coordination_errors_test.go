package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyCoordinationErrorRecognizesEverySentinel(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{"task not found", fmt.Errorf("task 1 not found: %w", errTaskNotFound), ErrTaskNotFound},
		{"lease task not found", fmt.Errorf("wrapped: %w", errLeaseTaskNotFound), ErrTaskNotFound},
		{"already claimed", fmt.Errorf("wrapped: %w", errAlreadyClaimed), ErrAlreadyClaimed},
		{"not owner", fmt.Errorf("wrapped: %w", errNotOwner), ErrNotOwner},
		{"lease not owner", fmt.Errorf("wrapped: %w", errLeaseNotOwner), ErrNotOwner},
		{"dependency", fmt.Errorf("wrapped: %w", errDependency), ErrDependency},
		{"cycle", fmt.Errorf("wrapped: %w", errCycleDetected), ErrCycleDetected},
		{"lease not active", fmt.Errorf("wrapped: %w", errLeaseNotActive), ErrLeaseNotActive},
		{"invalid transition", fmt.Errorf("wrapped: %w", errInvalidTransition), ErrInvalidTransition},
		{"lifecycle command error", lifecycleError(ErrNotOwner, "owned"), ErrNotOwner},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := classifyCoordinationError(tc.err)
			require.True(t, ok)
			assert.Equal(t, tc.want, code)
			assert.NotEqual(t, rpcStoreCorrupted, rpcCodeForErrorCode(code),
				"a recognized lifecycle failure must never be reported as a corrupt store")
		})
	}
}

func TestClassifyCoordinationErrorLeavesGenuineFailuresUnclassified(t *testing.T) {
	for _, err := range []error{nil, errors.New("write /tmp/tasks.bin: no space left on device")} {
		code, ok := classifyCoordinationError(err)
		assert.False(t, ok)
		assert.Empty(t, code)
	}
}

// Classification must survive wrapping, because guards wrap sentinels with a
// human-readable message and callers wrap again with operation context.
func TestClassifyCoordinationErrorSurvivesRepeatedWrapping(t *testing.T) {
	err := fmt.Errorf("task 4 not found: %w", errTaskNotFound)
	err = fmt.Errorf("resolving dependencies: %w", err)
	err = fmt.Errorf("decompose: %w", err)

	code, ok := classifyCoordinationError(err)
	require.True(t, ok)
	assert.Equal(t, ErrTaskNotFound, code)
}

// Message text must not decide the error class. This is the regression that
// substring matching in the JSON-RPC handlers made possible: a task titled
// "not found" would previously have been enough to change the reported code.
func TestClassificationIgnoresMessageText(t *testing.T) {
	misleading := fmt.Errorf("task 7 not found and already claimed by someone: %w", errDependency)
	code, ok := classifyCoordinationError(misleading)
	require.True(t, ok)
	assert.Equal(t, ErrDependency, code)

	assert.Zero(t, countSubstringErrorClassification(t),
		"error classification must not read message text")
}

func TestRequireLeaseAvailableTreatsExpiredLeasesAsAvailable(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	task := &store.Task{ID: 1, Owner: "crashed-worker"}

	task.LeaseExpires = uint64(now.Add(time.Minute).UnixMilli())
	assert.ErrorIs(t, requireLeaseAvailable(task, "successor", now), errAlreadyClaimed)

	// An expired lease is how a crashed worker's task returns to the pool.
	task.LeaseExpires = uint64(now.Add(-time.Minute).UnixMilli())
	assert.NoError(t, requireLeaseAvailable(task, "successor", now))

	// The holder may always re-enter its own lease.
	task.LeaseExpires = uint64(now.Add(time.Minute).UnixMilli())
	assert.NoError(t, requireLeaseAvailable(task, "crashed-worker", now))
}

func TestRequireOwnerAllowsUnownedTasks(t *testing.T) {
	assert.NoError(t, requireOwner(&store.Task{ID: 1}, "anyone"))
	assert.ErrorIs(t, requireOwner(&store.Task{ID: 1, Owner: "w1"}, "w2"), errNotOwner)
	assert.NoError(t, requireOwner(&store.Task{ID: 1, Owner: "w1"}, "w1"))
}

func TestRequireClaimableStatusRejectsTerminalAndBlockedWork(t *testing.T) {
	assert.ErrorIs(t,
		requireClaimableStatus(&store.Task{ID: 1, Status: store.StatusCompleted}),
		errInvalidTransition)
	assert.ErrorIs(t,
		requireClaimableStatus(&store.Task{ID: 1, Status: store.StatusBlocked}),
		errDependency)
	assert.NoError(t, requireClaimableStatus(&store.Task{ID: 1, Status: store.StatusPending}))
}

func TestRequireInProgressMatchesHeartbeatSemantics(t *testing.T) {
	assert.ErrorIs(t,
		requireInProgress(&store.Task{ID: 1, Status: store.StatusPending}),
		errLeaseNotActive)
	assert.NoError(t, requireInProgress(&store.Task{ID: 1, Status: store.StatusInProgress}))
}

// Every documented protocol identifier must have a JSON-RPC code, and no two
// identifiers may share one. Both tables are append-only protocol surface.
func TestRPCCodesAreTotalAndUnique(t *testing.T) {
	codes := []ErrorCode{
		ErrTaskNotFound, ErrNotInitialized, ErrInvalidArgs, ErrCycleDetected,
		ErrAlreadyClaimed, ErrNotOwner, ErrDependency, ErrStoreCorrupted,
		ErrLockContention, ErrSchemaVersion, ErrNoWork, ErrAgentAtCapacity,
		ErrIdempotencyConflict, ErrLeaseNotActive, ErrInvalidTransition,
	}
	seen := make(map[int]ErrorCode, len(codes))
	for _, code := range codes {
		numeric := rpcCodeForErrorCode(code)
		if code != ErrStoreCorrupted {
			assert.NotEqual(t, rpcStoreCorrupted, numeric, "%s falls through to the default", code)
		}
		if previous, clash := seen[numeric]; clash {
			t.Errorf("%s and %s share JSON-RPC code %d", previous, code, numeric)
		}
		seen[numeric] = code
	}
}

// countSubstringErrorClassification guards against the pattern returning. The
// JSON-RPC handlers previously recovered an error's class by matching
// substrings of its message, which made error text load-bearing protocol
// surface and silently misclassified conditions nobody had matched on.
var errorTextMatch = regexp.MustCompile(`strings\.(Contains|HasPrefix|HasSuffix|Index)\(\s*\w*[eE]rr\w*\.Error\(\)`)

func countSubstringErrorClassification(t *testing.T) int {
	t.Helper()
	sources, err := filepath.Glob("*.go")
	require.NoError(t, err)

	total := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		content, err := os.ReadFile(source)
		require.NoError(t, err)
		hits := errorTextMatch.FindAllString(string(content), -1)
		if len(hits) > 0 {
			t.Errorf("%s classifies errors by message text at %d site(s): %v", source, len(hits), hits)
			total += len(hits)
		}
	}
	return total
}
