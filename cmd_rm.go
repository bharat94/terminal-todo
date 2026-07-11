package main

import (
	"fmt"
	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdRm(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	updateStore(func(s *store.TaskStore) error {
		removing := make(map[uint64]struct{}, len(ids))
		for _, id := range ids {
			if _, ok := s.Tasks[id]; !ok {
				return fmt.Errorf("task %d not found", id)
			}
			removing[id] = struct{}{}
		}
		for _, task := range s.Tasks {
			if _, isRemoving := removing[task.ID]; isRemoving {
				continue
			}
			for _, dependency := range task.Depends {
				dependencyID, local := dag.ParseLocalID(dependency)
				if _, isRemoving := removing[dependencyID]; local && isRemoving {
					return fmt.Errorf("cannot remove task %d: task %d depends on it", dependencyID, task.ID)
				}
			}
		}
		for _, id := range ids {
			s.AddEvent(store.EventTaskRemoved, id, "", nil)
			s.RemoveTask(id)
		}
		return nil
	})
	for _, id := range ids {
		fmt.Printf("Removed task %d\n", id)
	}
}
