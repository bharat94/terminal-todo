package store

import (
	"bytes"
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

type LogEntry struct {
	Timestamp uint64 `msgpack:"ts" json:"timestamp"`
	Agent     string `msgpack:"agent" json:"agent"`
	Message   string `msgpack:"msg" json:"message"`
}

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
	Tags         []string          `msgpack:"tags" json:"tags"`
	RetryCount   uint32            `msgpack:"retry_count" json:"retry_count"`
	LastError    string            `msgpack:"last_error" json:"last_error"`
	Log          []LogEntry        `msgpack:"log" json:"log"`
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
		Tags:    make([]string, 0),
		Log:     make([]LogEntry, 0),
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

func (s *TaskStore) AddLog(id uint64, agent, message string) bool {
	task, ok := s.Tasks[id]
	if !ok {
		return false
	}
	task.Log = append(task.Log, LogEntry{
		Timestamp: uint64(time.Now().UnixMilli()),
		Agent:     agent,
		Message:   message,
	})
	s.LastModified = uint64(time.Now().UnixMilli())
	return true
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

	lock, err := lockFile(path, syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer unlockFile(lock)
	return writeStore(path, s)
}

func Load(path string) (*TaskStore, error) {
	lock, err := lockFile(path, syscall.LOCK_SH)
	if err != nil {
		return nil, err
	}
	defer unlockFile(lock)
	return loadUnlocked(path)
}

// Update serializes a complete read-modify-write operation. The callback runs
// while holding a lock on a stable sidecar file, so replacing tasks.bin cannot
// invalidate the lock and concurrent processes cannot overwrite newer state.
func Update(path string, mutate func(*TaskStore) error) (*TaskStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	lock, err := lockFile(path, syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}
	defer unlockFile(lock)

	s, err := loadUnlocked(path)
	if err != nil {
		return nil, err
	}
	if err := mutate(s); err != nil {
		return nil, err
	}
	if err := writeStore(path, s); err != nil {
		return nil, err
	}
	return s, nil
}

func lockFile(path string, mode int) (*os.File, error) {
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), mode); err != nil {
		lock.Close()
		return nil, fmt.Errorf("failed to lock store: %w", err)
	}
	return lock, nil
}

func unlockFile(lock *os.File) {
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

func loadUnlocked(path string) (*TaskStore, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewTaskStore(), nil
		}
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

func writeStore(path string, s *TaskStore) error {
	s.LastModified = uint64(time.Now().UnixMilli())
	data, err := msgpack.Marshal(s)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tasks-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.ReadFrom(bytes.NewReader(data)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
