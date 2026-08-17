package main

import (
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lifecycle operations are tested in process against an in-memory store.
// Before they were extracted, the same behavior could only be reached by
// building a binary and running it, which is why the CLI command files
// reported no coverage at all.

var opsNow = time.Unix(1_700_000_000, 0).UTC()

// newOpsStore builds a store with the given titles, numbered from 1.
func newOpsStore(t *testing.T, titles ...string) *store.TaskStore {
	t.Helper()
	s := store.NewTaskStore()
	for _, title := range titles {
		s.AddTask(title, nil)
	}
	require.Len(t, s.Tasks, len(titles))
	return s
}

func TestClaimTaskLeasesReadyWork(t *testing.T) {
	s := newOpsStore(t, "Implement")

	task, err := claimTask(s, 1, "w1", 30*time.Minute, nil, opsNow)
	require.NoError(t, err)

	assert.Equal(t, store.StatusInProgress, task.Status)
	assert.Equal(t, "w1", task.Owner)
	assert.Equal(t, uint64(opsNow.Add(30*time.Minute).UnixMilli()), task.LeaseExpires)
	assert.Equal(t, store.EventTaskClaimed, s.Events[len(s.Events)-1].Type)
}

func TestClaimTaskReportsTheMostFundamentalObstacleFirst(t *testing.T) {
	// A completed task that is also leased must report the terminal status
	// rather than the lease: re-claiming is impossible either way, and the
	// status is the reason that cannot be waited out.
	s := newOpsStore(t, "Done already")
	s.Tasks[1].Status = store.StatusCompleted
	s.Tasks[1].Owner = "w1"
	s.Tasks[1].LeaseExpires = uint64(opsNow.Add(time.Hour).UnixMilli())

	_, err := claimTask(s, 1, "w2", time.Minute, nil, opsNow)
	assert.ErrorIs(t, err, errInvalidTransition)
}

func TestClaimTaskAllowsReentryByTheCurrentHolder(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	// Re-claiming extends the lease rather than failing, which is what makes a
	// retried call after a lost response safe.
	later := opsNow.Add(time.Minute)
	task, err := claimTask(s, 1, "w1", time.Hour, nil, later)
	require.NoError(t, err)
	assert.Equal(t, uint64(later.Add(time.Hour).UnixMilli()), task.LeaseExpires)
}

func TestClaimTaskRejectsWorkHeldByAnotherAgent(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	_, err = claimTask(s, 1, "w2", time.Hour, nil, opsNow)
	assert.ErrorIs(t, err, errAlreadyClaimed)

	// Once the lease expires the task returns to the pool. This is the crash
	// recovery path.
	expired := opsNow.Add(2 * time.Hour)
	task, err := claimTask(s, 1, "w2", time.Hour, nil, expired)
	require.NoError(t, err)
	assert.Equal(t, "w2", task.Owner)
}

func TestCompleteTaskClearsOwnershipAndBlockReason(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := blockTask(s, 1, "w1", "waiting on review")
	require.NoError(t, err)
	_, err = unblockTask(s, 1, "w1")
	require.NoError(t, err)
	_, err = claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	task, err := completeTask(s, 1, "w1", nil, opsNow)
	require.NoError(t, err)

	assert.Equal(t, store.StatusCompleted, task.Status)
	assert.Empty(t, task.Owner)
	assert.Zero(t, task.LeaseExpires)
	assert.Empty(t, task.BlockReason)
	assert.Equal(t, uint64(opsNow.UnixMilli()), task.Completed)
}

func TestCompleteTaskRejectsWorkLeasedByAnotherAgent(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	_, err = completeTask(s, 1, "w2", nil, opsNow)
	assert.ErrorIs(t, err, errNotOwner)

	// Unowned work stays completable by anyone, so a human can close out what
	// a crashed agent left behind.
	s.Tasks[1].Owner = ""
	_, err = completeTask(s, 1, "", nil, opsNow)
	assert.NoError(t, err)
}

func TestReleaseTaskChargesARetryAndPreservesTheFinding(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	task, err := releaseTask(s, 1, "w1", "checksum mismatch on retry")
	require.NoError(t, err)

	assert.Equal(t, store.StatusPending, task.Status)
	assert.Empty(t, task.Owner)
	assert.Zero(t, task.LeaseExpires)
	assert.Equal(t, uint32(1), task.RetryCount)
	assert.Equal(t, "checksum mismatch on retry", task.LastError)

	event := s.Events[len(s.Events)-1]
	assert.Equal(t, store.EventTaskReleased, event.Type)
	assert.Equal(t, "checksum mismatch on retry", event.Data["error"])
}

func TestReleaseTaskRequiresAnActiveLease(t *testing.T) {
	s := newOpsStore(t, "Implement")

	_, err := releaseTask(s, 1, "w1", "")
	assert.ErrorIs(t, err, errLeaseNotActive)
}

func TestBlockTaskReleasesOwnershipSoRecoveryDoesNotNeedTheWorker(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	task, err := blockTask(s, 1, "w1", "upstream API is down")
	require.NoError(t, err)

	assert.Equal(t, store.StatusBlocked, task.Status)
	assert.Equal(t, "upstream API is down", task.BlockReason)
	assert.Empty(t, task.Owner, "blocked work must not stay tied to a worker that may never return")
	assert.Zero(t, task.LeaseExpires)
}

func TestUnblockTaskRepairsLegacyStaleOwnership(t *testing.T) {
	s := newOpsStore(t, "Implement")
	s.Tasks[1].Status = store.StatusBlocked
	s.Tasks[1].BlockReason = "upstream API is down"
	// Stores written before blocking released the lease can carry stale
	// ownership. Unblocking is where that is repaired.
	s.Tasks[1].Owner = "long-gone-worker"
	s.Tasks[1].LeaseExpires = uint64(opsNow.Add(time.Hour).UnixMilli())

	task, err := unblockTask(s, 1, "coordinator")
	require.NoError(t, err)

	assert.Equal(t, store.StatusPending, task.Status)
	assert.Empty(t, task.BlockReason)
	assert.Empty(t, task.Owner)
	assert.Zero(t, task.LeaseExpires)
}

func TestUnblockTaskRejectsWorkThatIsNotBlocked(t *testing.T) {
	s := newOpsStore(t, "Implement")

	_, err := unblockTask(s, 1, "coordinator")
	assert.ErrorIs(t, err, errInvalidTransition)
}

func TestLogTaskKeepsTheHandoffRecordAttributable(t *testing.T) {
	s := newOpsStore(t, "Implement")
	_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
	require.NoError(t, err)

	_, err = logTask(s, 1, "w2", "note from a bystander")
	assert.ErrorIs(t, err, errNotOwner)

	task, err := logTask(s, 1, "w1", "verified against the staging checksum")
	require.NoError(t, err)
	require.NotEmpty(t, task.Log)
	assert.Equal(t, "verified against the staging checksum", task.Log[len(task.Log)-1].Message)
}

func TestLifecycleOperationsRejectMissingTasks(t *testing.T) {
	operations := map[string]func(*store.TaskStore) error{
		"claim":    func(s *store.TaskStore) error { _, e := claimTask(s, 99, "w1", time.Hour, nil, opsNow); return e },
		"complete": func(s *store.TaskStore) error { _, e := completeTask(s, 99, "w1", nil, opsNow); return e },
		"release":  func(s *store.TaskStore) error { _, e := releaseTask(s, 99, "w1", ""); return e },
		"block":    func(s *store.TaskStore) error { _, e := blockTask(s, 99, "w1", "reason"); return e },
		"unblock":  func(s *store.TaskStore) error { _, e := unblockTask(s, 99, "w1"); return e },
		"log":      func(s *store.TaskStore) error { _, e := logTask(s, 99, "w1", "note"); return e },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, operation(newOpsStore(t, "Implement")), errTaskNotFound)
		})
	}
}

// A failed operation must not leave a partial mutation or a stray event
// behind. The store transaction is what guarantees this at the file level, but
// the operation itself must also validate before it writes.
func TestFailedLifecycleOperationsRecordNothing(t *testing.T) {
	operations := map[string]func(*store.TaskStore) error{
		"claim_held_by_another": func(s *store.TaskStore) error {
			_, e := claimTask(s, 1, "w2", time.Hour, nil, opsNow)
			return e
		},
		"complete_held_by_another": func(s *store.TaskStore) error {
			_, e := completeTask(s, 1, "w2", nil, opsNow)
			return e
		},
		"release_held_by_another": func(s *store.TaskStore) error {
			_, e := releaseTask(s, 1, "w2", "")
			return e
		},
		"log_held_by_another": func(s *store.TaskStore) error {
			_, e := logTask(s, 1, "w2", "note")
			return e
		},
		"unblock_not_blocked": func(s *store.TaskStore) error {
			_, e := unblockTask(s, 1, "w2")
			return e
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			s := newOpsStore(t, "Implement")
			_, err := claimTask(s, 1, "w1", time.Hour, nil, opsNow)
			require.NoError(t, err)

			before := *s.Tasks[1]
			events := len(s.Events)

			require.Error(t, operation(s))

			assert.Equal(t, before.Status, s.Tasks[1].Status)
			assert.Equal(t, before.Owner, s.Tasks[1].Owner)
			assert.Equal(t, before.LeaseExpires, s.Tasks[1].LeaseExpires)
			assert.Equal(t, before.RetryCount, s.Tasks[1].RetryCount)
			assert.Len(t, s.Events, events, "a rejected operation must not append an event")
		})
	}
}
