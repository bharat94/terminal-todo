package main

import (
	"fmt"
	"os"
	"terminal-todo/dag"
	"terminal-todo/store"
	"time"
)

func cmdClaim(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	var owner string
	var ttl time.Duration = 15 * time.Minute

	for i, arg := range args {
		if arg == "--as" && i+1 < len(args) {
			owner = args[i+1]
		}
		if arg == "--ttl" && i+1 < len(args) {
			t, err := time.ParseDuration(args[i+1])
			if err != nil || t <= 0 {
				fmt.Fprintln(os.Stderr, "Error: --ttl must be a positive duration")
				os.Exit(1)
			}
			ttl = t
		}
	}

	if owner == "" {
		fmt.Fprintln(os.Stderr, "Error: --as <owner> is required")
		os.Exit(1)
	}

	id := ids[0]
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
		if !dag.DependenciesComplete(task, s.Tasks) {
			return fmt.Errorf("task %d has incomplete dependencies", id)
		}
		now := uint64(time.Now().UnixMilli())
		if task.Owner != "" && task.Owner != owner && task.LeaseExpires > now {
			return fmt.Errorf("task %d already claimed by %s (expires in %s)", id, task.Owner, time.Duration(task.LeaseExpires-now)*time.Millisecond)
		}
		task.Owner = owner
		task.Status = store.StatusInProgress
		task.LeaseExpires = now + uint64(ttl.Milliseconds())
		return nil
	})
	fmt.Printf("Task %d claimed by %s (expires in %s)\n", id, owner, ttl)
}
