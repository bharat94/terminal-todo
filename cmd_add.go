package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	var priority float64
	var capabilities []string
	for i, arg := range args {
		switch arg {
		case "--priority":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --priority requires a value")
				os.Exit(1)
			}
			value, err := strconv.ParseFloat(args[i+1], 32)
			if err != nil || value < 0 || value > 1 {
				fmt.Fprintln(os.Stderr, "Error: --priority must be between 0 and 1")
				os.Exit(1)
			}
			priority = value
		case "--caps":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --caps requires a comma-separated value")
				os.Exit(1)
			}
			for _, capability := range strings.Split(args[i+1], ",") {
				capability = strings.TrimSpace(capability)
				if capability != "" {
					capabilities = append(capabilities, capability)
				}
			}
		}
	}

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
		task.Priority = float32(priority)
		task.Capabilities = capabilities
		taskID = task.ID
		return nil
	})

	fmt.Printf("Added task %d: %s\n", taskID, title)
}
