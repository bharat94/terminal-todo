package main

import (
	"fmt"
	"terminal-todo/dag"
	"terminal-todo/store"
	"time"
)

func cmdClaim(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	var owner string
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	ttl := parseDefaultTTL(cfg)

	for i, arg := range args {
		if arg == "--as" && i+1 < len(args) {
			owner = args[i+1]
		}
		if arg == "--ttl" && i+1 < len(args) {
			t, err := time.ParseDuration(args[i+1])
			if err != nil || t <= 0 {
				fail(ErrInvalidArgs, "--ttl must be a positive duration")
			}
			ttl = t
		}
	}

	if owner == "" {
		fail(ErrInvalidArgs, "--as <owner> is required")
	}
	if err := touchAgent(owner); err != nil {
		fail(ErrStoreCorrupted, "registering agent %s: %v", owner, err)
	}

	id := ids[0]
	preflight := loadStore()
	var resolver dag.DependencyResolver
	if task, ok := preflight.GetTask(id); ok {
		resolver = snapshotDependencyResolver([]*store.Task{task})
	}
	var retryCount uint32
	var lastError string
	var claimed *store.Task
	updateStore(func(s *store.TaskStore) error {
		task, ok := s.GetTask(id)
		if !ok {
			return fmt.Errorf("task %d not found", id)
		}
		if task.Status == store.StatusCompleted {
			return fmt.Errorf("task %d is already completed", id)
		}
		if task.Status == store.StatusBlocked {
			return fmt.Errorf("task %d is blocked", id)
		}
		if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
			return fmt.Errorf("task %d has incomplete dependencies", id)
		}
		now := uint64(time.Now().UnixMilli())
		if task.Owner != "" && task.Owner != owner && task.LeaseExpires > now {
			return fmt.Errorf("task %d already claimed by %s (expires in %s)", id, task.Owner, time.Duration(task.LeaseExpires-now)*time.Millisecond)
		}
		retryCount = task.RetryCount
		lastError = task.LastError
		task.Owner = owner
		task.Status = store.StatusInProgress
		task.LeaseExpires = now + uint64(ttl.Milliseconds())
		s.AddLog(id, owner, "claimed")
		s.AddEvent(store.EventTaskClaimed, id, owner, map[string]string{"ttl": ttl.String()})
		claimed = task
		return nil
	})
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(claimed)})
		return
	}

	msg := fmt.Sprintf("Task %d claimed by %s (expires in %s)", id, owner, ttl)
	if retryCount > 0 {
		msg += fmt.Sprintf(" [retry #%d]", retryCount)
	}
	if lastError != "" {
		msg += fmt.Sprintf(" (previous error: %s)", lastError)
	}
	fmt.Println(msg)

}
