package main

import (
	"fmt"

	"terminal-todo/store"
)

func cmdPrune(args []string) {
	var removedCount int
	updateStore(func(s *store.TaskStore) error {
		for _, t := range s.GetAllTasks() {
			if t.Status == store.StatusCompleted {
				s.RemoveTask(t.ID)
				removedCount++
			}
		}
		return nil
	})
	fmt.Printf("Removed %d completed task(s)\n", removedCount)
}
