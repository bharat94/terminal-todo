package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharat94/terminal-todo/fsutil"

	"github.com/bharat94/terminal-todo/lock"

	"github.com/vmihailenco/msgpack/v5"
)

type TaskStatus uint8

const (
	StatusPending TaskStatus = iota
	StatusInProgress
	StatusCompleted
	StatusBlocked
)

type EventType string

const (
	EventTaskCreated       EventType = "created"
	EventTaskCompleted     EventType = "completed"
	EventTaskClaimed       EventType = "claimed"
	EventTaskReleased      EventType = "released"
	EventTaskBlocked       EventType = "blocked"
	EventTaskUnblocked     EventType = "unblocked"
	EventTaskUpdated       EventType = "updated"
	EventTaskDecomposed    EventType = "decomposed"
	EventTaskRemoved       EventType = "removed"
	EventDependencyAdded   EventType = "dep_added"
	EventDependencyRemoved EventType = "dep_removed"
	EventLeaseExpired      EventType = "lease_expired"
	EventLeaseRenewed      EventType = "lease_renewed"
)

type Event struct {
	ID        uint64            `msgpack:"id" json:"id"`
	Timestamp uint64            `msgpack:"ts" json:"timestamp"`
	Type      EventType         `msgpack:"type" json:"type"`
	TaskID    uint64            `msgpack:"task_id" json:"task_id"`
	Actor     string            `msgpack:"actor" json:"actor"`
	Data      map[string]string `msgpack:"data" json:"data"`
}

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
	BlockReason  string            `msgpack:"block_reason" json:"block_reason"`
	Log          []LogEntry        `msgpack:"log" json:"log"`
	Extra        map[string]string `msgpack:"extra" json:"extra"`
}

type AcquisitionReceipt struct {
	RequestID   string `msgpack:"request_id" json:"request_id"`
	Operation   string `msgpack:"operation" json:"operation"`
	Fingerprint string `msgpack:"fingerprint" json:"fingerprint"`
	Actor       string `msgpack:"actor" json:"actor"`
	Created     uint64 `msgpack:"created" json:"created"`
	Task        Task   `msgpack:"task" json:"task"`
}

const CurrentSchemaVersion uint32 = 4

type TaskStore struct {
	SchemaVersion uint32                        `msgpack:"schema_version"`
	Tasks         map[uint64]*Task              `msgpack:"tasks"`
	NextID        uint64                        `msgpack:"next_id"`
	LastModified  uint64                        `msgpack:"last_modified"`
	NextEventID   uint64                        `msgpack:"next_event_id"`
	Events        []Event                       `msgpack:"events"`
	Acquisitions  map[string]AcquisitionReceipt `msgpack:"acquisitions"`
}

type migrationFunc func(*TaskStore) error

var migrations = map[uint32]migrationFunc{
	0: func(s *TaskStore) error {
		if s.Tasks == nil {
			s.Tasks = make(map[uint64]*Task)
		}
		for _, t := range s.Tasks {
			if t.Tags == nil {
				t.Tags = []string{}
			}
			if t.Log == nil {
				t.Log = []LogEntry{}
			}
			if t.Extra == nil {
				t.Extra = map[string]string{}
			}
		}
		s.SchemaVersion = 1
		return nil
	},
	1: func(s *TaskStore) error {
		if s.Events == nil {
			s.Events = []Event{}
		}
		s.NextEventID = 1
		s.SchemaVersion = 2
		return nil
	},
	2: func(s *TaskStore) error {
		s.SchemaVersion = 3
		return nil
	},
	3: func(s *TaskStore) error {
		s.Acquisitions = make(map[string]AcquisitionReceipt)
		s.SchemaVersion = 4
		return nil
	},
}

func runMigrations(s *TaskStore) error {
	for v := s.SchemaVersion; v < CurrentSchemaVersion; v++ {
		migrate, ok := migrations[v]
		if !ok {
			return fmt.Errorf("no migration path from schema version %d to %d", v, v+1)
		}
		if err := migrate(s); err != nil {
			return fmt.Errorf("migration %d→%d failed: %w", v, v+1, err)
		}
	}
	return nil
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		SchemaVersion: CurrentSchemaVersion,
		Tasks:         make(map[uint64]*Task),
		NextID:        1,
		NextEventID:   1,
		Events:        make([]Event, 0),
		Acquisitions:  make(map[string]AcquisitionReceipt),
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
	return task, true
}

func (s *TaskStore) AddEvent(eventType EventType, taskID uint64, actor string, data map[string]string) *Event {
	if data == nil {
		data = map[string]string{}
	}
	event := &Event{
		ID:        s.NextEventID,
		Timestamp: uint64(time.Now().UnixMilli()),
		Type:      eventType,
		TaskID:    taskID,
		Actor:     actor,
		Data:      data,
	}
	s.Events = append(s.Events, *event)
	s.NextEventID++
	s.LastModified = uint64(time.Now().UnixMilli())
	return event
}

func (s *TaskStore) EventsSince(id uint64) []Event {
	if id == 0 {
		result := make([]Event, len(s.Events))
		copy(result, s.Events)
		return result
	}
	for i, e := range s.Events {
		if e.ID > id {
			result := make([]Event, len(s.Events)-i)
			copy(result, s.Events[i:])
			return result
		}
	}
	return []Event{}
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
	for _, task := range s.Tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// CleanExpiredLeases scans all tasks and resets any whose lease has expired.
// Returns the number of leases cleaned. Must be called under a write lock.
func (s *TaskStore) CleanExpiredLeases() int {
	now := uint64(time.Now().UnixMilli())
	cleaned := 0
	for _, task := range s.Tasks {
		if task.Status == StatusInProgress && task.LeaseExpires > 0 && task.LeaseExpires < now {
			owner := task.Owner
			task.Status = StatusPending
			task.Owner = ""
			task.LeaseExpires = 0
			s.AddEvent(EventLeaseExpired, task.ID, owner, map[string]string{"owner": owner})
			cleaned++
		}
	}
	if cleaned > 0 {
		s.LastModified = uint64(time.Now().UnixMilli())
	}
	return cleaned
}

func (s *TaskStore) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	lk, err := lock.Open(path)
	if err != nil {
		return err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Write); err != nil {
		return err
	}
	defer lk.Release()
	return writeStore(path, s)
}

func Load(path string) (*TaskStore, error) {
	lk, err := lock.Open(path)
	if err != nil {
		return nil, err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Read); err != nil {
		return nil, err
	}
	defer lk.Release()
	return loadUnlocked(path)
}

// LoadCurrent returns a store after durably reclaiming expired leases. Lease
// expiration is an explicit write transition rather than a side effect of a
// read, and emits one lease_expired event per reclaimed task.
func LoadCurrent(path string) (*TaskStore, error) {
	snapshot, err := Load(path)
	if err != nil {
		return nil, err
	}
	if !snapshot.HasExpiredLeases() {
		return snapshot, nil
	}

	lk, err := lock.Open(path)
	if err != nil {
		return nil, err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Write); err != nil {
		return nil, err
	}
	defer lk.Release()

	s, err := loadUnlocked(path)
	if err != nil {
		return nil, err
	}
	if s.CleanExpiredLeases() > 0 {
		if err := writeStore(path, s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *TaskStore) HasExpiredLeases() bool {
	now := uint64(time.Now().UnixMilli())
	for _, task := range s.Tasks {
		if task.Status == StatusInProgress && task.LeaseExpires > 0 && task.LeaseExpires < now {
			return true
		}
	}
	return false
}

// Update serializes a complete read-modify-write operation. The callback runs
// while holding a lock so replacing tasks.bin cannot invalidate the lock and
// concurrent processes cannot overwrite newer state.
func Update(path string, mutate func(*TaskStore) error) (*TaskStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lk, err := lock.Open(path)
	if err != nil {
		return nil, err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Write); err != nil {
		return nil, err
	}
	defer lk.Release()

	s, err := loadUnlocked(path)
	if err != nil {
		return nil, err
	}
	cleaned := s.CleanExpiredLeases()
	if err := mutate(s); err != nil {
		if cleaned > 0 {
			if writeErr := writeStore(path, s); writeErr != nil {
				return nil, fmt.Errorf("mutation failed: %v; persisting lease expiration: %w", err, writeErr)
			}
		}
		return nil, err
	}
	if err := writeStore(path, s); err != nil {
		return nil, err
	}
	return s, nil
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
	if store.SchemaVersion < CurrentSchemaVersion {
		if err := runMigrations(&store); err != nil {
			return nil, err
		}
	} else if store.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("store schema version %d is newer than this binary (max %d); upgrade todo", store.SchemaVersion, CurrentSchemaVersion)
	}
	if store.Acquisitions == nil {
		store.Acquisitions = make(map[string]AcquisitionReceipt)
	}
	return &store, nil
}

func writeStore(path string, s *TaskStore) error {
	s.SchemaVersion = CurrentSchemaVersion
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

	if err := tmp.Chmod(0600); err != nil {
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

	return fsutil.SyncDir(filepath.Dir(path))
}
