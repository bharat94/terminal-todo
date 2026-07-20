package main

import (
	"fmt"
	"sort"

	"github.com/bharat94/terminal-todo/dag"

	"github.com/bharat94/terminal-todo/store"
)

func cmdPrune(args []string) {
	var removed []*store.Task
	updateStore(func(s *store.TaskStore) error {
		completed := make(map[uint64]struct{})
		for _, t := range s.GetAllTasks() {
			if t.Status == store.StatusCompleted {
				completed[t.ID] = struct{}{}
				removed = append(removed, t)
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
		}
		return nil
	})
	sort.Slice(removed, func(i, j int) bool {
		return removed[i].ID < removed[j].ID
	})
	if receiptRequested(args) {
		ids := make([]uint64, 0, len(removed))
		for _, task := range removed {
			ids = append(ids, task.ID)
		}
		writeJSON(newMutationReceipt("prune", ids))
		return
	}
	if hasFlag(args, "--json") {
		tasks := make([]protocolTask, 0, len(removed))
		for _, task := range removed {
			tasks = append(tasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: tasks})
		return
	}
	fmt.Printf("Removed %d completed task(s)\n", len(removed))
}
