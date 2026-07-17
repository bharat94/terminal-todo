package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

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
