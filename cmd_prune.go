package main

import (
	"fmt"

	"terminal-todo/store"
)

func cmdPrune(args []string) {
	s := loadStore()
	tasks := s.GetAllTasks()

	var removedCount int
	for _, t := range tasks {
		if t.Status == store.StatusCompleted {
			s.RemoveTask(t.ID)
			removedCount++
		}
	}

	if removedCount > 0 {
		saveStore(s)
	}
	fmt.Printf("Removed %d completed task(s)\n", removedCount)
}
