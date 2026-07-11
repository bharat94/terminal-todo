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

	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, ok := s.GetTask(id)
			if !ok {
				return fmt.Errorf("task %d not found", id)
			}
			if !dag.DependenciesComplete(task, s.Tasks) {
				return fmt.Errorf("task %d has incomplete dependencies", id)
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
