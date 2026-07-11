package main

import (
	"fmt"
	"os"
	"terminal-todo/store"
)

func cmdRm(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			if !s.RemoveTask(id) {
				return fmt.Errorf("task %d not found", id)
			}
		}
		return nil
	})
	for _, id := range ids {
		fmt.Printf("Removed task %d\n", id)
	}
}
