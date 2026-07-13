package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
)

func TestStore_SaveLoad(t *testing.T) {
	s := NewTaskStore()
	s.AddTask("Task 1", nil)
	s.AddTask("Task 2", []string{"todo://local/1"})

	tmpFile := "./test_store.bin"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".tmp")
	defer os.Remove(tmpFile + ".lock")

	err := s.Save(tmpFile)
	assert.NoError(t, err)

	loaded, err := Load(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), loaded.NextID)
	assert.Equal(t, 2, len(loaded.Tasks))

	task1, ok := loaded.GetTask(1)
	assert.True(t, ok)
	assert.Equal(t, "Task 1", task1.Title)
	assert.Equal(t, StatusPending, task1.Status)

	task2, ok := loaded.GetTask(2)
	assert.True(t, ok)
	assert.Equal(t, "Task 2", task2.Title)
	assert.Equal(t, []string{"todo://local/1"}, task2.Depends)
}

func TestStore_ConcurrentUpdatesDoNotLoseTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.bin")
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Update(path, func(s *TaskStore) error {
				s.AddTask("concurrent task", nil)
				return nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}

	s, err := Load(path)
	assert.NoError(t, err)
	assert.Len(t, s.Tasks, writers)
	assert.Equal(t, uint64(writers+1), s.NextID)
}

func TestStore_LeaseExpiration(t *testing.T) {
	s := NewTaskStore()
	task := s.AddTask("Task 1", nil)
	task.Status = StatusInProgress
	task.Owner = "agent-1"
	task.LeaseExpires = uint64(time.Now().UnixMilli()) - 1000 // Expired 1s ago

	// Reads are pure; expiration requires an explicit state transition.
	t1, ok := s.GetTask(1)
	assert.True(t, ok)
	assert.Equal(t, StatusInProgress, t1.Status)
	assert.Equal(t, "agent-1", t1.Owner)
}

func TestStore_LoadCurrentPersistsLeaseExpirationEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.bin")
	s := NewTaskStore()
	task := s.AddTask("expired work", nil)
	task.Status = StatusInProgress
	task.Owner = "agent-crashed"
	task.LeaseExpires = uint64(time.Now().Add(-time.Minute).UnixMilli())
	assert.NoError(t, s.Save(path))

	current, err := LoadCurrent(path)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, current.Tasks[1].Status)
	assert.Empty(t, current.Tasks[1].Owner)
	assert.Len(t, current.Events, 1)
	assert.Equal(t, EventLeaseExpired, current.Events[0].Type)
	assert.Equal(t, "agent-crashed", current.Events[0].Actor)
	assert.Equal(t, "agent-crashed", current.Events[0].Data["owner"])

	reloaded, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, StatusPending, reloaded.Tasks[1].Status)
	assert.Len(t, reloaded.Events, 1)
}

func TestStore_AddTask(t *testing.T) {
	s := NewTaskStore()
	task := s.AddTask("Test Task", []string{"todo://local/1", "todo://local/2"})

	assert.Equal(t, uint64(1), task.ID)
	assert.Equal(t, "Test Task", task.Title)
	assert.Equal(t, StatusPending, task.Status)
	assert.Equal(t, []string{"todo://local/1", "todo://local/2"}, task.Depends)
}

func TestStore_MigrationV0ToCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.bin")

	type v0Task struct {
		ID    uint64            `msgpack:"id"`
		Title string            `msgpack:"title"`
		Tags  []string          `msgpack:"tags"`
		Log   []LogEntry        `msgpack:"log"`
		Extra map[string]string `msgpack:"extra"`
	}
	type v0Store struct {
		Tasks  map[uint64]*v0Task `msgpack:"tasks"`
		NextID uint64             `msgpack:"next_id"`
	}

	task := &v0Task{
		ID:    1,
		Title: "migration task",
		Tags:  nil,
		Log:   nil,
		Extra: nil,
	}
	v0 := &v0Store{
		Tasks:  map[uint64]*v0Task{1: task},
		NextID: 2,
	}

	data, err := msgpack.Marshal(v0)
	assert.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	assert.NoError(t, err)

	s, err := loadUnlocked(path)
	assert.NoError(t, err)

	assert.Equal(t, CurrentSchemaVersion, s.SchemaVersion)
	assert.Equal(t, 1, len(s.Tasks))
	assert.NotNil(t, s.Tasks)

	loaded, ok := s.Tasks[1]
	assert.True(t, ok)
	assert.Equal(t, "migration task", loaded.Title)
	assert.Equal(t, []string{}, loaded.Tags)
	assert.Equal(t, []LogEntry{}, loaded.Log)
	assert.Equal(t, map[string]string{}, loaded.Extra)
	assert.NotNil(t, s.Events)
	assert.Equal(t, 0, len(s.Events))
	assert.Equal(t, uint64(1), s.NextEventID)
	assert.Equal(t, uint64(2), s.NextID)
}

func TestStore_MigrationV1ToCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.bin")

	type v1Store struct {
		SchemaVersion uint32           `msgpack:"schema_version"`
		Tasks         map[uint64]*Task `msgpack:"tasks"`
		NextID        uint64           `msgpack:"next_id"`
		LastModified  uint64           `msgpack:"last_modified"`
	}

	v1 := &v1Store{
		SchemaVersion: 1,
		Tasks:         map[uint64]*Task{1: {ID: 1, Title: "v1 task"}},
		NextID:        2,
	}

	data, err := msgpack.Marshal(v1)
	assert.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	assert.NoError(t, err)

	s, err := loadUnlocked(path)
	assert.NoError(t, err)

	assert.Equal(t, CurrentSchemaVersion, s.SchemaVersion)
	assert.NotNil(t, s.Events)
	assert.Equal(t, 0, len(s.Events))
	assert.Equal(t, uint64(1), s.NextEventID)
	assert.Equal(t, uint64(2), s.NextID)
}

func TestStore_SchemaVersionTooNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.bin")

	type futureStore struct {
		SchemaVersion uint32 `msgpack:"schema_version"`
	}

	future := &futureStore{SchemaVersion: CurrentSchemaVersion + 1}
	data, err := msgpack.Marshal(future)
	assert.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	assert.NoError(t, err)

	_, err = loadUnlocked(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store schema version")
}

func TestStore_CleanExpiredLeases(t *testing.T) {
	s := NewTaskStore()

	t1 := s.AddTask("active lease", nil)
	t1.Status = StatusInProgress
	t1.Owner = "agent-1"
	t1.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())

	t2 := s.AddTask("expired lease", nil)
	t2.Status = StatusInProgress
	t2.Owner = "agent-2"
	t2.LeaseExpires = uint64(time.Now().Add(-time.Hour).UnixMilli())

	t3 := s.AddTask("no lease", nil)
	t3.Status = StatusPending

	cleaned := s.CleanExpiredLeases()
	assert.Equal(t, 1, cleaned)

	assert.Equal(t, StatusPending, t2.Status)
	assert.Equal(t, "", t2.Owner)
	assert.Equal(t, uint64(0), t2.LeaseExpires)

	assert.Equal(t, StatusInProgress, t1.Status)
	assert.Equal(t, "agent-1", t1.Owner)

	assert.Equal(t, StatusPending, t3.Status)
	assert.Len(t, s.Events, 1)
	assert.Equal(t, EventLeaseExpired, s.Events[0].Type)
}
