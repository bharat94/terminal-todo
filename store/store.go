package store

import (
	"os"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type TaskStatus uint8

const (
	StatusPending TaskStatus = iota
	StatusInProgress
	StatusCompleted
)

type Task struct {
	ID        uint64     `msgpack:"id"`
	Title     string     `msgpack:"title"`
	Status    TaskStatus `msgpack:"status"`
	Depends   []uint64  `msgpack:"depends"`
	Created   uint64     `msgpack:"created"`
	Completed uint64     `msgpack:"completed"`
}

type TaskStore struct {
	Tasks        map[uint64]*Task
	NextID       uint64
	LastModified uint64
}

func NewTaskStore() *TaskStore {
	return &TaskStore{
		Tasks:  make(map[uint64]*Task),
		NextID: 1,
	}
}

func (s *TaskStore) AddTask(title string, depends []uint64) *Task {
	task := &Task{
		ID:      s.NextID,
		Title:   title,
		Status:  StatusPending,
		Depends: depends,
		Created: uint64(time.Now().UnixMilli()),
	}
	s.Tasks[task.ID] = task
	s.NextID++
	s.LastModified = uint64(time.Now().UnixMilli())
	return task
}

func (s *TaskStore) GetTask(id uint64) (*Task, bool) {
	task, ok := s.Tasks[id]
	return task, ok
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

func (s *TaskStore) Save(path string) error {
	dir := path
	if !isDir(path) {
		dir = getDir(path)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := msgpack.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Load(path string) (*TaskStore, error) {
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

func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
