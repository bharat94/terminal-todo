package main

import (
	"fmt"
	"os"
	"terminal-todo/dag"
	"terminal-todo/store"
	"time"
)

func cmdDone(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}
	owner := optionValue(args, "--as")
	preflight := loadStore()
	remoteTasks := make([]*store.Task, 0, len(ids))
	for _, id := range ids {
		if task, ok := preflight.GetTask(id); ok {
			remoteTasks = append(remoteTasks, task)
		}
	}
	resolver := snapshotDependencyResolver(remoteTasks)

	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, ok := s.GetTask(id)
			if !ok {
				return fmt.Errorf("task %d not found", id)
			}
			if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
				return fmt.Errorf("task %d has incomplete dependencies", id)
			}
			if task.Owner != "" && task.Owner != owner {
				return fmt.Errorf("task %d is claimed by %s; use --as %s", id, task.Owner, task.Owner)
			}
			task.Status = store.StatusCompleted
			task.Completed = uint64(time.Now().UnixMilli())
			task.Owner = ""
			task.LeaseExpires = 0
		}
		return nil
	})
	for _, id := range ids {
		fmt.Printf("Marked task %d as done\n", id)
	}
}
