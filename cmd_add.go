package main

import (
	"fmt"
	"os"
	"strings"

	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdAdd(args []string) {
	s := loadStore()

	var title string
	var depends []uint64

	var titleParts []string
	var i int
	for i = 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--after" {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: --after requires a task ID\n")
				os.Exit(1)
			}
			var id uint64
			if _, err := fmt.Sscanf(args[i+1], "%d", &id); err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[i+1])
				os.Exit(1)
			}
			if _, ok := s.GetTask(id); !ok {
				fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
				os.Exit(1)
			}
			depends = append(depends, id)
			i++
		} else if strings.HasPrefix(arg, "--after=") {
			idStr := strings.TrimPrefix(arg, "--after=")
			var id uint64
			if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", idStr)
				os.Exit(1)
			}
			if _, ok := s.GetTask(id); !ok {
				fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
				os.Exit(1)
			}
			depends = append(depends, id)
		} else if !strings.HasPrefix(arg, "-") {
			titleParts = append(titleParts, arg)
		}
	}

	if len(titleParts) == 0 {
		fmt.Fprintf(os.Stderr, "Error: task title required\n")
		os.Exit(1)
	}

	title = strings.Join(titleParts, " ")

	if len(depends) > 0 {
		d := dag.NewDAG()
		dagTasks := convertToDAGTasks(s.Tasks)
		d.BuildFromTasks(dagTasks)
		nextID := s.NextID
		if err := d.DetectCycle(depends, nextID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	task := s.AddTask(title, depends)
	saveStore(s)

	fmt.Printf("Added task %d: %s\n", task.ID, task.Title)
}

func convertToDAGTasks(tasks map[uint64]*store.Task) map[uint64]*dag.Task {
	result := make(map[uint64]*dag.Task)
	for id, t := range tasks {
		result[id] = &dag.Task{
			ID:      t.ID,
			Title:   t.Title,
			Depends: t.Depends,
			Status:  uint8(t.Status),
		}
	}
	return result
}
