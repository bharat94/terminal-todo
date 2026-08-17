package cli

import (
	"fmt"
	"time"

	"github.com/bharat94/terminal-todo/store"
)

// handoffTask atomically persists structured successor context and returns an
// actively leased task to the pending queue. Unlike release, a normal handoff
// is not counted as a failed attempt.
func handoffTask(s *store.TaskStore, id uint64, actor string, extra map[string]string, now time.Time) (*store.Task, error) {
	task, ok := s.GetTask(id)
	if !ok {
		return nil, fmt.Errorf("task %d not found: %w", id, errLeaseTaskNotFound)
	}
	if task.Status != store.StatusInProgress || task.Owner == "" || task.LeaseExpires == 0 || task.LeaseExpires <= uint64(now.UnixMilli()) {
		return nil, fmt.Errorf("task %d: %w", id, errLeaseNotActive)
	}
	if task.Owner != actor {
		return nil, fmt.Errorf("task %d is claimed by %s: %w", id, task.Owner, errLeaseNotOwner)
	}
	if err := validateProjectedCardinality("extra", len(task.Extra), projectedExtraCount(task.Extra, extra), maxTaskExtraEntries); err != nil {
		return nil, persistedInputFailure(err)
	}

	if task.Extra == nil {
		task.Extra = make(map[string]string)
	}
	for key, value := range extra {
		task.Extra[key] = value
	}
	s.AddEvent(store.EventTaskUpdated, id, actor, map[string]string{"source": "handoff"})
	s.AddLog(id, actor, "handed off with structured context")
	s.AddEvent(store.EventTaskReleased, id, actor, map[string]string{"handoff": "true"})
	task.Owner = ""
	task.LeaseExpires = 0
	task.Status = store.StatusPending
	return task, nil
}
