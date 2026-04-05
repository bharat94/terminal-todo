package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type TaskStatus uint8

const (
	StatusPending TaskStatus = iota
	StatusInProgress
	StatusCompleted
	StatusBlocked
)

type Task struct {
	ID           uint64            `msgpack:"id" json:"id"`
	Title        string            `msgpack:"title" json:"title"`
	Status       TaskStatus        `msgpack:"status" json:"status"`
	Depends      []string          `msgpack:"depends" json:"depends"` // URIs: todo://local/1 or todo://repo/1
	Created      uint64            `msgpack:"created" json:"created"`
	Completed    uint64            `msgpack:"completed" json:"completed"`
	Capabilities []string          `msgpack:"caps" json:"capabilities"`
	Owner        string            `msgpack:"owner" json:"owner"`
	LeaseExpires uint64            `msgpack:"lease_exp" json:"lease_expires"`
	Priority     float32           `msgpack:"priority" json:"priority"`
	Lineage      string            `msgpack:"lineage" json:"lineage"`
	Extra        map[string]string `msgpack:"extra" json:"extra"`
}

type TaskStore struct {
	Tasks        map[uint64]*Task `msgpack:"tasks"`
	NextID       uint64           `msgpack:"next_id"`
	LastModified uint64           `msgpack:"last_modified"`
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		Tasks:  make(map[uint64]*Task),
		NextID: 1,
	}
}

func (s *TaskStore) AddTask(title string, depends []string) *Task {
	task := &Task{
		ID:      s.NextID,
		Title:   title,
		Status:  StatusPending,
		Depends: depends,
		Created: uint64(time.Now().UnixMilli()),
		Extra:   make(map[string]string),
	}
	s.Tasks[task.ID] = task
	s.NextID++
	s.LastModified = uint64(time.Now().UnixMilli())
	return task
}

func (s *TaskStore) GetTask(id uint64) (*Task, bool) {
	task, ok := s.Tasks[id]
	if !ok {
		return nil, false
	}
	// Check for expired lease
	if task.Status == StatusInProgress && task.LeaseExpires > 0 && task.LeaseExpires < uint64(time.Now().UnixMilli()) {
		task.Status = StatusPending
		task.Owner = ""
		task.LeaseExpires = 0
	}
	return task, true
}

func (s *TaskStore) RemoveTask(id uint64) bool {
	if _, ok := s.Tasks[id]; !ok {
		return false
	}
	delete(s.Tasks, id)
	s.LastModified = uint64(time.Now().UnixMilli())
	return true
}

func (s *TaskStore) GetAllTasks() []*Task {
	tasks := make([]*Task, 0, len(s.Tasks))
	now := uint64(time.Now().UnixMilli())
	for _, task := range s.Tasks {
		// Clean up expired leases on the fly
		if task.Status == StatusInProgress && task.LeaseExpires > 0 && task.LeaseExpires < now {
			task.Status = StatusPending
			task.Owner = ""
			task.LeaseExpires = 0
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func (s *TaskStore) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 1. Open/Create the file for locking
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 2. Acquire Exclusive Lock (EX)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock store: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// 3. Prepare data
	s.LastModified = uint64(time.Now().UnixMilli())
	data, err := msgpack.Marshal(s)
	if err != nil {
		return err
	}

	// 4. Atomic Rename-Swap
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func Load(path string) (*TaskStore, error) {
	// 1. Open for reading and Shared Lock (SH)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewTaskStore(), nil
		}
		return nil, err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("failed to read-lock store: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var store TaskStore
	if err := msgpack.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Tasks == nil {
		store.Tasks = make(map[uint64]*Task)
	}
	return &store, nil
}
