package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/dag"

	"github.com/bharat94/terminal-todo/store"
)

func cmdPrune(args []string) {
	var removedCount int
	updateStore(func(s *store.TaskStore) error {
		completed := make(map[uint64]struct{})
		for _, t := range s.GetAllTasks() {
			if t.Status == store.StatusCompleted {
				completed[t.ID] = struct{}{}
			}
		}
		for _, task := range s.Tasks {
			if _, willRemove := completed[task.ID]; willRemove {
				continue
			}
			kept := task.Depends[:0]
			for _, dependency := range task.Depends {
				dependencyID, local := dag.ParseLocalID(dependency)
				if _, pruned := completed[dependencyID]; local && pruned {
					continue
				}
				kept = append(kept, dependency)
			}
			task.Depends = kept
		}
		for id := range completed {
			s.RemoveTask(id)
			removedCount++
		}
		return nil
	})
	fmt.Printf("Removed %d completed task(s)\n", removedCount)
}
