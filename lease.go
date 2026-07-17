package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/bharat94/terminal-todo/store"
)

var (
	errLeaseTaskNotFound = errors.New("lease task not found")
	errLeaseNotActive    = errors.New("lease is not active")
	errLeaseNotOwner     = errors.New("lease is owned by another agent")
)

func renewLease(s *store.TaskStore, id uint64, actor string, ttl time.Duration, now time.Time) (*store.Task, error) {
	task, ok := s.GetTask(id)
	if !ok {
		return nil, fmt.Errorf("task %d not found: %w", id, errLeaseTaskNotFound)
	}

	nowMillis := uint64(now.UnixMilli())
	if task.Status != store.StatusInProgress || task.Owner == "" || task.LeaseExpires == 0 || task.LeaseExpires <= nowMillis {
		return nil, fmt.Errorf("task %d: %w", id, errLeaseNotActive)
	}
	if task.Owner != actor {
		return nil, fmt.Errorf("task %d is claimed by %s: %w", id, task.Owner, errLeaseNotOwner)
	}

	previousExpiry := task.LeaseExpires
	task.LeaseExpires = nowMillis + uint64(ttl.Milliseconds())
	s.AddEvent(store.EventLeaseRenewed, id, actor, map[string]string{
		"ttl":              ttl.String(),
		"previous_expires": formatTimestamp(previousExpiry),
	})
	return task, nil
}
