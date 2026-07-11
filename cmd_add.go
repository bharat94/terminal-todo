package main

import (
	"fmt"
	"os"
	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdAdd(args []string) {
	title := extractTitle(args)
	if title == "" {
		fmt.Fprintln(os.Stderr, "Error: title is required")
		os.Exit(1)
	}

	afterIDs := extractAfterIDs(args)

	var taskID uint64
	updateStore(func(s *store.TaskStore) error {
		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)
		var finalDeps []string
		for _, dep := range afterIDs {
			depID, local := dag.ParseLocalID(dep)
			if local {
				if _, ok := s.Tasks[depID]; !ok {
					return fmt.Errorf("dependency task %d not found", depID)
				}
				finalDeps = append(finalDeps, fmt.Sprintf("todo://local/%d", depID))
			} else {
				finalDeps = append(finalDeps, dep)
			}
		}
		if err := d.DetectCycle(finalDeps, s.NextID); err != nil {
			return err
		}
		task := s.AddTask(title, finalDeps)
		taskID = task.ID
		return nil
	})

	fmt.Printf("Added task %d: %s\n", taskID, title)
}
