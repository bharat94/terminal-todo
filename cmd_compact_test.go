package main

import (
	"path/filepath"
	"testing"
	"time"

	"terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompactTaskStoreAppliesExplicitRetention(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	s := store.NewTaskStore()
	completed := s.AddTask("Completed", nil)
	completed.Status = store.StatusCompleted
	active := s.AddTask("Active", nil)
	active.Status = store.StatusInProgress
	s.Events = []store.Event{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	s.NextEventID = 5
	old := uint64(now.Add(-100 * time.Hour).UnixMilli())
	recent := uint64(now.Add(-time.Hour).UnixMilli())
	s.Acquisitions = map[string]store.AcquisitionReceipt{
		"completed-old": {RequestID: "completed-old", Created: old, Task: *completed},
		"active-old":    {RequestID: "active-old", Created: old, Task: *active},
		"missing-old":   {RequestID: "missing-old", Created: old, Task: store.Task{ID: 999}},
		"completed-new": {RequestID: "completed-new", Created: recent, Task: *completed},
	}
	keep := 2
	options := compactOptions{KeepEvents: &keep, ReceiptsBefore: 24 * time.Hour}

	preview := compactTaskStore(s, compactOptions{
		KeepEvents:     options.KeepEvents,
		ReceiptsBefore: options.ReceiptsBefore,
		DryRun:         true,
	}, false, now)
	assert.Equal(t, compactResult{EventsRemoved: 2, ReceiptsRemoved: 2, DryRun: true}, preview)
	assert.Len(t, s.Events, 4)
	assert.Len(t, s.Acquisitions, 4)

	result := compactTaskStore(s, options, true, now)
	assert.Equal(t, 2, result.EventsRemoved)
	assert.Equal(t, 2, result.ReceiptsRemoved)
	assert.Equal(t, []store.Event{{ID: 3}, {ID: 4}}, s.Events)
	assert.Equal(t, uint64(5), s.NextEventID, "event IDs must remain monotonic")
	assert.Contains(t, s.Acquisitions, "active-old")
	assert.Contains(t, s.Acquisitions, "completed-new")
	assert.NotContains(t, s.Acquisitions, "completed-old")
	assert.NotContains(t, s.Acquisitions, "missing-old")
}

func TestParseCompactOptions(t *testing.T) {
	options, err := parseCompactOptions([]string{"--keep-events", "1000", "--receipts-before", "2160h", "--dry-run"})
	require.NoError(t, err)
	require.NotNil(t, options.KeepEvents)
	assert.Equal(t, 1000, *options.KeepEvents)
	assert.Equal(t, 2160*time.Hour, options.ReceiptsBefore)
	assert.True(t, options.DryRun)

	for _, args := range [][]string{
		nil,
		{"--keep-events", "-1"},
		{"--receipts-before", "0h"},
		{"--receipts-before", "90d"},
	} {
		_, err := parseCompactOptions(args)
		assert.Error(t, err, args)
	}
}

func TestRestoreBackupRestoresCompleteCoordinationState(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	current := store.NewTaskStore()
	current.AddTask("Current state", nil)
	require.NoError(t, current.Save(tasksBinPath()))

	snapshot := store.NewTaskStore()
	task := snapshot.AddTask("Restored task", nil)
	snapshot.AddEvent(store.EventTaskCreated, task.ID, "backup-agent", map[string]string{"source": "backup"})
	snapshot.Acquisitions["restore-request"] = store.AcquisitionReceipt{
		RequestID: "restore-request",
		Operation: "acquire",
		Actor:     "backup-agent",
		Created:   uint64(time.Now().Add(-time.Hour).UnixMilli()),
		Task:      *task,
	}
	backupPath := filepath.Join(t.TempDir(), "full-backup.bin")
	require.NoError(t, snapshot.Save(backupPath))

	count, err := restoreBackup(backupPath)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	restored, err := store.Load(tasksBinPath())
	require.NoError(t, err)
	assert.Equal(t, snapshot.NextID, restored.NextID)
	assert.Equal(t, snapshot.NextEventID, restored.NextEventID)
	assert.Equal(t, snapshot.Events, restored.Events)
	assert.Equal(t, snapshot.Acquisitions, restored.Acquisitions)
	assert.Equal(t, "Restored task", restored.Tasks[task.ID].Title)
}
