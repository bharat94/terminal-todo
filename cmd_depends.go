package main

import (
	"fmt"
	"terminal-todo/dag"
)

func cmdDepends(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	s := loadStore()
	task, ok := s.GetTask(ids[0])
	if !ok {
		fail(ErrTaskNotFound, "task %d not found", ids[0])
	}

	if len(task.Depends) == 0 {
		fmt.Printf("Task %d has no dependencies.\n", ids[0])
		return
	}

	fmt.Printf("Task %d depends on:\n", ids[0])
	for _, dep := range task.Depends {
		depID, local := dag.ParseLocalID(dep)
		if local {
			if dt, ok := s.GetTask(depID); ok {
				fmt.Printf("  - %d: %s\n", depID, dt.Title)
			} else {
				fmt.Printf("  - %d: [not found locally]\n", depID)
			}
		} else {
			fmt.Printf("  - %s\n", dep)
		}
	}
}
