package main

import (
	"fmt"
	"os"
	"terminal-todo/dag"
)

func cmdAdd(args []string) {
	title := extractTitle(args)
	if title == "" {
		fmt.Fprintln(os.Stderr, "Error: title is required")
		os.Exit(1)
	}

	afterIDs := extractAfterIDs(args)
	
	s := loadStore()
	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)

	// Validate dependencies and check for cycles
	var finalDeps []string
	for _, dep := range afterIDs {
		depID, local := dag.ParseLocalID(dep)
		if local {
			if _, ok := s.Tasks[depID]; !ok {
				fmt.Fprintf(os.Stderr, "Error: dependency task %d not found\n", depID)
				os.Exit(1)
			}
			finalDeps = append(finalDeps, fmt.Sprintf("todo://local/%d", depID))
		} else {
			// Assume it's a cross-repo URI, add as is
			finalDeps = append(finalDeps, dep)
		}
	}

	// Cycle detection (only for local tasks)
	if err := d.DetectCycle(finalDeps, s.NextID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	task := s.AddTask(title, finalDeps)
	saveStore(s)

	fmt.Printf("Added task %d: %s\n", task.ID, task.Title)
}
