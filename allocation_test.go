package main

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
)

func TestAcquireRequestIsAtomicAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("First", nil)
	s.AddTask("Second", nil)
	assert.NoError(t, s.Save(path))
	fingerprint := acquireFingerprint("agent", "default", "registered", nil)

	type result struct {
		task     *store.Task
		replayed bool
		err      error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			var got result
			_, got.err = store.Update(path, func(current *store.TaskStore) error {
				got.task, got.replayed, got.err = acquireFromStore(current, "agent", "request-1", fingerprint, time.Hour, nil, 0, nil)
				return got.err
			})
			results <- got
		}()
	}

	seenFresh, seenReplay := 0, 0
	for i := 0; i < 2; i++ {
		got := <-results
		assert.NoError(t, got.err)
		assert.Equal(t, uint64(1), got.task.ID)
		if got.replayed {
			seenReplay++
		} else {
			seenFresh++
		}
	}
	assert.Equal(t, 1, seenFresh)
	assert.Equal(t, 1, seenReplay)

	persisted, err := store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Len(t, persisted.Acquisitions, 1)
	assert.Len(t, persisted.Events, 1)
	assert.Equal(t, store.StatusPending, persisted.Tasks[2].Status)
}

func TestAcquireReceiptSurvivesTaskRemoval(t *testing.T) {
	s := store.NewTaskStore()
	s.AddTask("Transient", nil)
	fingerprint := acquireFingerprint("agent", "default", "registered", nil)
	acquired, replayed, err := acquireFromStore(s, "agent", "request-1", fingerprint, time.Hour, nil, 0, nil)
	assert.NoError(t, err)
	assert.False(t, replayed)
	assert.True(t, s.RemoveTask(acquired.ID))

	replayedTask, replayed, err := acquireFromStore(s, "agent", "request-1", fingerprint, time.Hour, nil, 0, nil)
	assert.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, acquired.ID, replayedTask.ID)
	assert.Equal(t, "Transient", replayedTask.Title)
	assert.Empty(t, s.Tasks)
}

func TestFailedAcquireDoesNotConsumeRequestID(t *testing.T) {
	s := store.NewTaskStore()
	fingerprint := acquireFingerprint("agent", "default", "registered", nil)
	_, _, err := acquireFromStore(s, "agent", "request-1", fingerprint, time.Hour, nil, 0, nil)
	assert.True(t, errors.Is(err, errNoReadyTasks))
	assert.Empty(t, s.Acquisitions)

	s.AddTask("Arrived later", nil)
	task, replayed, err := acquireFromStore(s, "agent", "request-1", fingerprint, time.Hour, nil, 0, nil)
	assert.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, "Arrived later", task.Title)
}

func TestAllocationDiagnosticsExplainNoWorkStates(t *testing.T) {
	t.Run("no pending work", func(t *testing.T) {
		diagnostics := diagnoseAllocation(store.NewTaskStore(), nil, "worker", []string{"go"}, true, 0)

		assert.Equal(t, allocationNoPendingWork, diagnostics.Reason)
		assert.Equal(t, []string{"go"}, diagnostics.RequestedCapabilities)
		assert.Equal(t, 1, diagnostics.RequestedCapabilitiesTotal)
		assert.False(t, diagnostics.RequestedCapabilitiesTruncated)
		assert.Zero(t, diagnostics.Queue.Pending)
		assert.NotNil(t, diagnostics.Worker)
	})

	t.Run("dependencies incomplete", func(t *testing.T) {
		s := store.NewTaskStore()
		task := s.AddTask("Waiting", []string{"todo://backend/7", "99"})
		task.RetryCount = 2

		diagnostics := diagnoseAllocation(s, nil, "worker", nil, true, 0)

		assert.Equal(t, allocationDependenciesIncomplete, diagnostics.Reason)
		assert.Equal(t, 1, diagnostics.Queue.Pending)
		assert.Equal(t, 1, diagnostics.Queue.DependencyBlocked)
		assert.Equal(t, []string{"99", "todo://backend/7"}, diagnostics.Queue.DependencyBlockers)
		assert.Equal(t, 2, diagnostics.Queue.DependencyBlockersTotal)
		assert.False(t, diagnostics.Queue.DependencyBlockersTruncated)
		assert.Equal(t, 1, diagnostics.Queue.RetriedPending)
		assert.Zero(t, diagnostics.Queue.Ready)
	})

	t.Run("capability mismatch", func(t *testing.T) {
		s := store.NewTaskStore()
		task := s.AddTask("Python work", nil)
		task.Capabilities = []string{"python", "docker"}

		diagnostics := diagnoseAllocation(s, nil, "worker", []string{"go"}, true, 0)

		assert.Equal(t, allocationCapabilitiesUnavailable, diagnostics.Reason)
		assert.Equal(t, 1, diagnostics.Queue.Ready)
		assert.Zero(t, diagnostics.Queue.CompatibleReady)
		assert.Equal(t, 1, diagnostics.Queue.CapabilityMismatch)
		assert.Equal(t, []string{"docker", "python"}, diagnostics.Queue.MissingCapabilities)
		assert.Equal(t, 2, diagnostics.Queue.MissingCapabilitiesTotal)
		assert.False(t, diagnostics.Queue.MissingCapabilitiesTruncated)
	})

	t.Run("work owned by others", func(t *testing.T) {
		s := store.NewTaskStore()
		task := s.AddTask("Claimed", nil)
		task.Status = store.StatusInProgress
		task.Owner = "other-worker"
		task.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())

		diagnostics := diagnoseAllocation(s, nil, "worker", nil, true, 0)

		assert.Equal(t, allocationWorkOwnedByOthers, diagnostics.Reason)
		assert.Equal(t, 1, diagnostics.Queue.InProgress)
		assert.Equal(t, 1, diagnostics.Worker.OwnedByOthers)
		assert.Zero(t, diagnostics.Worker.Owned)
	})

	t.Run("worker at capacity takes precedence", func(t *testing.T) {
		s := store.NewTaskStore()
		owned := s.AddTask("Owned", nil)
		owned.Status = store.StatusInProgress
		owned.Owner = "worker"
		owned.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())
		s.AddTask("Ready", nil)

		diagnostics := diagnoseAllocation(s, nil, "worker", nil, true, 1)

		assert.Equal(t, allocationWorkerAtCapacity, diagnostics.Reason)
		assert.Equal(t, 1, diagnostics.Worker.CurrentLoad)
		assert.Equal(t, 1, diagnostics.Worker.MaxLoad)
		assert.Equal(t, 1, diagnostics.Queue.CompatibleReady)
	})
}

func TestAcquireErrorsCarryAllocationDiagnostics(t *testing.T) {
	s := store.NewTaskStore()
	task := s.AddTask("Needs Python", nil)
	task.Capabilities = []string{"python"}
	fingerprint := acquireFingerprint("worker", "default", "explicit", []string{"go"})

	_, _, err := acquireFromStore(s, "worker", "diagnostic-request", fingerprint, time.Hour, []string{"go"}, 0, nil)

	assert.True(t, errors.Is(err, errNoReadyTasks))
	diagnostics, ok := allocationDiagnosticsFromError(err)
	assert.True(t, ok)
	assert.Equal(t, allocationCapabilitiesUnavailable, diagnostics.Reason)
	assert.Equal(t, 1, diagnostics.Queue.CapabilityMismatch)
	assert.Equal(t, []string{"python"}, diagnostics.Queue.MissingCapabilities)
	assert.Equal(t, "ready work requires capabilities this worker does not provide", err.Error())
}

func TestAllocationDiagnosticsBoundLargeGraphDetails(t *testing.T) {
	s := store.NewTaskStore()
	longDependency := "todo://backend/" + strings.Repeat("界", maxAllocationDiagnosticValueBytes)
	for i := 0; i < maxAllocationDiagnosticItems+5; i++ {
		dependency := "todo://backend/" + leftPadInt(i, 3)
		if i == 0 {
			dependency = longDependency
		}
		task := s.AddTask("Waiting", []string{dependency})
		task.Capabilities = []string{"cap-" + leftPadInt(i, 3)}
	}

	diagnostics := diagnoseAllocation(s, nil, "worker", []string{"go"}, true, 0)

	assert.Len(t, diagnostics.Queue.DependencyBlockers, maxAllocationDiagnosticItems)
	assert.Equal(t, maxAllocationDiagnosticItems+5, diagnostics.Queue.DependencyBlockersTotal)
	assert.True(t, diagnostics.Queue.DependencyBlockersTruncated)
	assert.Len(t, diagnostics.Queue.MissingCapabilities, 0)
	assert.Zero(t, diagnostics.Queue.MissingCapabilitiesTotal)
	for _, blocker := range diagnostics.Queue.DependencyBlockers {
		assert.LessOrEqual(t, len(blocker), maxAllocationDiagnosticValueBytes)
	}

	readyStore := store.NewTaskStore()
	for i := 0; i < maxAllocationDiagnosticItems+5; i++ {
		task := readyStore.AddTask("Capability work", nil)
		task.Capabilities = []string{"cap-" + leftPadInt(i, 3)}
	}
	diagnostics = diagnoseAllocation(readyStore, nil, "worker", []string{"go"}, true, 0)
	assert.Len(t, diagnostics.Queue.MissingCapabilities, maxAllocationDiagnosticItems)
	assert.Equal(t, maxAllocationDiagnosticItems+5, diagnostics.Queue.MissingCapabilitiesTotal)
	assert.True(t, diagnostics.Queue.MissingCapabilitiesTruncated)
	assert.Equal(t, "cap-000", diagnostics.Queue.MissingCapabilities[0])
	assert.Equal(t, "cap-019", diagnostics.Queue.MissingCapabilities[maxAllocationDiagnosticItems-1])
}

func TestAllocationDiagnosticsBoundRequestedCapabilitiesWithoutChangingMatching(t *testing.T) {
	s := store.NewTaskStore()
	task := s.AddTask("Matched by omitted diagnostic capability", nil)
	task.Capabilities = []string{"cap-024"}
	capabilities := make([]string, 0, maxAllocationDiagnosticItems+5)
	for i := maxAllocationDiagnosticItems + 4; i >= 0; i-- {
		capabilities = append(capabilities, "cap-"+leftPadInt(i, 3))
	}
	capabilities[1] = strings.Repeat("界", maxAllocationDiagnosticValueBytes)
	capabilities = append(capabilities, "cap-024")

	diagnostics := diagnoseAllocation(s, nil, "worker", capabilities, true, 0)

	assert.Equal(t, allocationReady, diagnostics.Reason)
	assert.Equal(t, 1, diagnostics.Queue.CompatibleReady)
	assert.Len(t, diagnostics.RequestedCapabilities, maxAllocationDiagnosticItems)
	assert.Equal(t, maxAllocationDiagnosticItems+5, diagnostics.RequestedCapabilitiesTotal)
	assert.True(t, diagnostics.RequestedCapabilitiesTruncated)
	assert.Equal(t, "cap-000", diagnostics.RequestedCapabilities[0])
	assert.Equal(t, "cap-019", diagnostics.RequestedCapabilities[maxAllocationDiagnosticItems-1])
}

func TestAllocationDiagnosticValueCompactionIsUTF8Safe(t *testing.T) {
	longCapability := strings.Repeat("界", maxAllocationDiagnosticValueBytes)
	values := map[string]struct{}{longCapability: {}}

	items, total, truncated := boundedAllocationDiagnosticValues(values)

	assert.Equal(t, 1, total)
	assert.True(t, truncated)
	assert.Len(t, items, 1)
	assert.True(t, utf8.ValidString(items[0]))
	assert.LessOrEqual(t, len(items[0]), maxAllocationDiagnosticValueBytes)
	assert.True(t, strings.HasSuffix(items[0], "..."))
}

func leftPadInt(value, width int) string {
	result := strconv.Itoa(value)
	for len(result) < width {
		result = "0" + result
	}
	return result
}
