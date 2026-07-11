package main

import (
	"fmt"
	"terminal-todo/dag"
)

func cmdDependents(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	targetID := ids[0]
	s := loadStore()
	if _, ok := s.GetTask(targetID); !ok {
		fail(ErrTaskNotFound, "task %d not found", targetID)
	}

	fmt.Printf("Tasks that depend on %d:\n", targetID)
	found := false
	for _, t := range s.Tasks {
		for _, dep := range t.Depends {
			id, local := dag.ParseLocalID(dep)
			if local && id == targetID {
				fmt.Printf("  - %d: %s\n", t.ID, t.Title)
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Println("  None.")
	}
}
