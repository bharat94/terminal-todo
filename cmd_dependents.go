package main

import (
	"fmt"
	"os"
	"terminal-todo/dag"
)

func cmdDependents(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	targetID := ids[0]
	s := loadStore()
	if _, ok := s.GetTask(targetID); !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", targetID)
		os.Exit(1)
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
