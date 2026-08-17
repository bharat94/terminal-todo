package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// The CLI --into document spells the capability field "caps" and the JSON-RPC
// subtask spells it "capabilities". Both are published and neither can move,
// so the CLI shape converts into the shared one rather than aliasing onto it.
func TestDecomposePayloadPreservesTheCLICapabilitySpelling(t *testing.T) {
	var payload DecomposePayload
	require.NoError(t, json.Unmarshal(
		[]byte(`{"subtasks":[{"title":"Reproduce the race","caps":["go","testing"]}]}`),
		&payload,
	))

	subtasks := payload.subtasks()
	require.Len(t, subtasks, 1)
	assert.Equal(t, "Reproduce the race", subtasks[0].Title)
	assert.Equal(t, []string{"go", "testing"}, subtasks[0].Capabilities)
}

func TestDecomposeTaskYieldsTheParentLeaseToItsChildren(t *testing.T) {
	s := newOpsStore(t, "Fix the race")
	_, err := claimTask(s, 1, "planner", time.Hour, nil, opsNow)
	require.NoError(t, err)

	parent, children, err := decomposeTask(s, 1, "planner", []decomposeSubtask{
		{Title: "Reproduce", Capabilities: []string{"go"}},
		{Title: "Fix", Capabilities: []string{"go", "concurrency"}},
	})
	require.NoError(t, err)
	require.Len(t, children, 2)

	// The parent cannot finish until its children do, so holding the lease
	// would block the graph it just created.
	assert.Equal(t, store.StatusPending, parent.Status)
	assert.Empty(t, parent.Owner)
	assert.Zero(t, parent.LeaseExpires)
	assert.Len(t, parent.Depends, 2)

	for _, child := range children {
		assert.Equal(t, "todo://local/1", child.Lineage)
		// Siblings are parallel unless dependencies are added explicitly.
		assert.Empty(t, child.Depends)
	}
}

func TestDecomposeTaskRejectsACompletedParent(t *testing.T) {
	s := newOpsStore(t, "Already shipped")
	_, err := completeTask(s, 1, "", nil, opsNow)
	require.NoError(t, err)

	_, _, err = decomposeTask(s, 1, "planner", []decomposeSubtask{{Title: "Child"}})
	assert.ErrorIs(t, err, errInvalidTransition)
}

func TestUpdateTaskDetectsCyclesAgainstTheProjectedGraph(t *testing.T) {
	s := newOpsStore(t, "First", "Second")
	// 2 depends on 1.
	_, err := updateTask(s, 2, "", taskUpdate{AddDeps: []string{"todo://local/1"}})
	require.NoError(t, err)

	// Closing the loop must be refused.
	_, err = updateTask(s, 1, "", taskUpdate{AddDeps: []string{"todo://local/2"}})
	assert.ErrorIs(t, err, errCycleDetected)

	// Removing an edge must not be reported as a cycle. Building the graph
	// from current rather than projected edges would produce a false positive
	// here.
	_, err = updateTask(s, 2, "", taskUpdate{RemoveDeps: []string{"todo://local/1"}})
	assert.NoError(t, err)
	assert.Empty(t, s.Tasks[2].Depends)
}

func TestUpdateTaskRejectsUnknownDependencyEdits(t *testing.T) {
	s := newOpsStore(t, "First")

	_, err := updateTask(s, 1, "", taskUpdate{AddDeps: []string{"todo://local/99"}})
	assert.ErrorIs(t, err, errTaskNotFound)

	_, err = updateTask(s, 1, "", taskUpdate{RemoveDeps: []string{"todo://local/99"}})
	assert.ErrorIs(t, err, errTaskNotFound)
}

// Absent and empty are different. A nil field leaves the value alone;
// SetCapabilities with an empty slice deliberately clears the requirement.
func TestUpdateTaskDistinguishesAbsentFromEmpty(t *testing.T) {
	s := newOpsStore(t, "First")
	_, err := updateTask(s, 1, "", taskUpdate{
		SetCapabilities: true,
		Capabilities:    []string{"go", "testing"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"go", "testing"}, s.Tasks[1].Capabilities)

	title := "Renamed"
	_, err = updateTask(s, 1, "", taskUpdate{Title: &title})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", s.Tasks[1].Title)
	assert.Equal(t, []string{"go", "testing"}, s.Tasks[1].Capabilities, "absent means unchanged")

	_, err = updateTask(s, 1, "", taskUpdate{SetCapabilities: true, Capabilities: []string{}})
	require.NoError(t, err)
	assert.Empty(t, s.Tasks[1].Capabilities, "empty means cleared")
}

func TestUpdateTaskRejectsInvalidValues(t *testing.T) {
	s := newOpsStore(t, "First")

	blank := "   "
	_, err := updateTask(s, 1, "", taskUpdate{Title: &blank})
	require.Error(t, err)
	code, ok := classifyCoordinationError(err)
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgs, code)

	tooHigh := float32(1.5)
	_, err = updateTask(s, 1, "", taskUpdate{Priority: &tooHigh})
	require.Error(t, err)
}

func TestPruneCompletedTasksDropsDanglingLocalEdges(t *testing.T) {
	s := newOpsStore(t, "Prerequisite", "Dependent")
	_, err := updateTask(s, 2, "", taskUpdate{AddDeps: []string{"todo://local/1"}})
	require.NoError(t, err)
	_, err = completeTask(s, 1, "", nil, opsNow)
	require.NoError(t, err)

	removed := pruneCompletedTasks(s)
	require.Len(t, removed, 1)
	assert.Equal(t, uint64(1), removed[0].ID)

	// A surviving task that still listed the removed prerequisite could never
	// become ready again.
	require.Contains(t, s.Tasks, uint64(2))
	assert.Empty(t, s.Tasks[2].Depends)
}

func TestPruneCompletedTasksLeavesCrossRepositoryEdgesAlone(t *testing.T) {
	s := newOpsStore(t, "Local prerequisite", "Dependent")
	_, err := updateTask(s, 2, "", taskUpdate{
		AddDeps: []string{"todo://local/1", "todo://upstream/7"},
	})
	require.NoError(t, err)
	_, err = completeTask(s, 1, "", nil, opsNow)
	require.NoError(t, err)

	pruneCompletedTasks(s)

	// The remote edge resolves against another store; its absence here means
	// nothing and must not be treated as dangling.
	assert.Equal(t, []string{"todo://upstream/7"}, s.Tasks[2].Depends)
}

func TestNewlyReadyAfterReportsOnlyWorkTheCompletionReleased(t *testing.T) {
	s := newOpsStore(t, "Prerequisite", "Dependent", "Independent", "Still blocked")
	_, err := updateTask(s, 2, "", taskUpdate{AddDeps: []string{"todo://local/1"}})
	require.NoError(t, err)
	_, err = updateTask(s, 4, "", taskUpdate{AddDeps: []string{"todo://local/1", "todo://local/3"}})
	require.NoError(t, err)
	_, err = completeTask(s, 1, "", nil, opsNow)
	require.NoError(t, err)

	unblocked := newlyReadyAfter(s, []uint64{1}, nil)
	require.Len(t, unblocked, 1)
	// 2 was released. 3 was already ready, so it is not news. 4 still waits on 3.
	assert.Equal(t, uint64(2), unblocked[0].ID)
}

func TestNewlyReadyAfterIsEmptyWithoutCompletions(t *testing.T) {
	s := newOpsStore(t, "First")
	assert.Empty(t, newlyReadyAfter(s, nil, nil))
}

func TestNewAcquirePlanDerivesAStableFingerprint(t *testing.T) {
	root := t.TempDir()
	previous := projectRoot
	projectRoot = root
	defer func() { projectRoot = previous }()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".terminal-todo"), 0o700))
	require.NoError(t, store.NewTaskStore().Save(tasksBinPath()))

	// The fingerprint is what makes a retried acquire replay instead of
	// allocating a second task, so identical requests must agree on it and
	// different requests must not.
	first, err := newAcquirePlan("w1", "30m", []string{"go"})
	require.NoError(t, err)
	second, err := newAcquirePlan("w1", "30m", []string{"go"})
	require.NoError(t, err)
	assert.Equal(t, first.Fingerprint, second.Fingerprint)
	assert.Equal(t, 30*time.Minute, first.TTL)

	other, err := newAcquirePlan("w1", "45m", []string{"go"})
	require.NoError(t, err)
	assert.NotEqual(t, first.Fingerprint, other.Fingerprint)

	// Registered capabilities and an explicit empty list are different
	// requests, not the same one.
	registered, err := newAcquirePlan("w1", "30m", nil)
	require.NoError(t, err)
	explicitlyNone, err := newAcquirePlan("w1", "30m", []string{})
	require.NoError(t, err)
	assert.NotEqual(t, registered.Fingerprint, explicitlyNone.Fingerprint)

	_, err = newAcquirePlan("w1", "not-a-duration", nil)
	assert.ErrorIs(t, err, errInvalidTTL)
	_, err = newAcquirePlan("w1", "-5m", nil)
	assert.ErrorIs(t, err, errInvalidTTL)
}

// Dependencies resolve numerically but are compared as strings, so every
// spelling of one edge must collapse to one stored key. Before this held,
// `1`, `todo://local/1`, and `todo://local/01` were three edges pointing at
// the same task: they inflated the dependency count, and removing one left
// the others, so a task stayed blocked by an edge the user believed deleted.
func TestDependencyEditsCollapseEverySpellingOfOneEdge(t *testing.T) {
	s := newOpsStore(t, "Prerequisite", "Dependent")

	task, err := updateTask(s, 2, "", taskUpdate{
		AddDeps: []string{"1", "todo://local/1", "todo://local/01"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"todo://local/1"}, task.Depends)

	// Any spelling must remove the edge.
	task, err = updateTask(s, 2, "", taskUpdate{RemoveDeps: []string{"1"}})
	require.NoError(t, err)
	assert.Empty(t, task.Depends)
}

func TestDependencyEditsRepairLegacySpellingsOnTheEditedTask(t *testing.T) {
	s := newOpsStore(t, "Prerequisite", "Other", "Dependent")
	// A store written before normalization can hold several spellings of one
	// edge. Editing the task is where that is repaired.
	s.Tasks[3].Depends = []string{"1", "todo://local/01", "todo://local/1"}

	task, err := updateTask(s, 3, "", taskUpdate{AddDeps: []string{"2"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"todo://local/1", "todo://local/2"}, task.Depends)
}

func TestCanonicalDependencyRejectsUnparseableReferences(t *testing.T) {
	for _, reference := range []string{"", "0", "-1", "todo://", "todo:///1", "not a ref"} {
		_, err := canonicalDependency(reference)
		assert.Error(t, err, "canonicalDependency(%q) must fail", reference)
	}

	for reference, want := range map[string]string{
		"1":                  "todo://local/1",
		"todo://local/01":    "todo://local/1",
		"todo://upstream/07": "todo://upstream/7",
	} {
		canonical, err := canonicalDependency(reference)
		require.NoError(t, err, reference)
		assert.Equal(t, want, canonical)
	}
}
