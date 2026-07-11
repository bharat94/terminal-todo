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

	var unblocked []uint64
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
			s.AddEvent(store.EventTaskCompleted, id, owner, nil)

			// Auto-unblock dependents whose all deps are now met
			for _, depTask := range s.Tasks {
				if depTask.Status != store.StatusBlocked {
					continue
				}
				for _, depURI := range depTask.Depends {
					depID, local := dag.ParseLocalID(depURI)
					if local && depID == id && dag.DependenciesCompleteWithResolver(depTask, s.Tasks, resolver) {
						depTask.Status = store.StatusPending
						s.AddEvent(store.EventTaskUnblocked, depTask.ID, owner, nil)
						unblocked = append(unblocked, depTask.ID)
						break
					}
				}
			}
		}
		return nil
	})
	for _, id := range ids {
		fmt.Printf("Marked task %d as done\n", id)
	}
	for _, id := range unblocked {
		fmt.Printf("Unblocked task %d (all dependencies met)\n", id)
	}
}
