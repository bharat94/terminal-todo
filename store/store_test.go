package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_SaveLoad(t *testing.T) {
	s := NewTaskStore()
	s.AddTask("Task 1", nil)
	s.AddTask("Task 2", []uint64{1})

	tmpFile := "/tmp/test_store.bin"
	defer os.Remove(tmpFile)

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
	assert.Equal(t, []uint64{1}, task2.Depends)
}

func TestStore_LoadNonExistent(t *testing.T) {
	loaded, err := Load("/tmp/nonexistent_store.bin")
	os.Remove("/tmp/nonexistent_store.bin")
	assert.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, uint64(1), loaded.NextID)
}

func TestStore_AddTask(t *testing.T) {
	s := NewTaskStore()
	task := s.AddTask("Test Task", []uint64{1, 2})

	assert.Equal(t, uint64(1), task.ID)
	assert.Equal(t, "Test Task", task.Title)
	assert.Equal(t, StatusPending, task.Status)
	assert.Equal(t, []uint64{1, 2}, task.Depends)
	assert.Greater(t, task.Created, uint64(0))
}

func TestStore_GetTask(t *testing.T) {
	s := NewTaskStore()
	s.AddTask("Task 1", nil)

	task, ok := s.GetTask(1)
	assert.True(t, ok)
	assert.Equal(t, "Task 1", task.Title)

	_, ok = s.GetTask(999)
	assert.False(t, ok)
}

func TestStore_RemoveTask(t *testing.T) {
	s := NewTaskStore()
	s.AddTask("Task 1", nil)

	ok := s.RemoveTask(1)
	assert.True(t, ok)

	_, ok = s.GetTask(1)
	assert.False(t, ok)

	ok = s.RemoveTask(999)
	assert.False(t, ok)
}
