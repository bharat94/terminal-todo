package store

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStore_SaveLoad(t *testing.T) {
	s := NewTaskStore()
	s.AddTask("Task 1", nil)
	s.AddTask("Task 2", []string{"todo://local/1"})

	tmpFile := "./test_store.bin"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".tmp")

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

func TestStore_LeaseExpiration(t *testing.T) {
	s := NewTaskStore()
	task := s.AddTask("Task 1", nil)
	task.Status = StatusInProgress
	task.Owner = "agent-1"
	task.LeaseExpires = uint64(time.Now().UnixMilli()) - 1000 // Expired 1s ago

	// GetTask should clean it up
	t1, ok := s.GetTask(1)
	assert.True(t, ok)
	assert.Equal(t, StatusPending, t1.Status)
	assert.Equal(t, "", t1.Owner)
}

func TestStore_AddTask(t *testing.T) {
	s := NewTaskStore()
	task := s.AddTask("Test Task", []string{"todo://local/1", "todo://local/2"})

	assert.Equal(t, uint64(1), task.ID)
	assert.Equal(t, "Test Task", task.Title)
	assert.Equal(t, StatusPending, task.Status)
	assert.Equal(t, []string{"todo://local/1", "todo://local/2"}, task.Depends)
}
