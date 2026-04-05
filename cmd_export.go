package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"terminal-todo/store"
)

func cmdExport(args []string) {
	s := loadStore()
	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	if hasFlag(args, "--markdown") {
		fmt.Println("# Project Tasks")
		fmt.Println("\n## Pending")
		for _, t := range tasks {
			if t.Status != store.StatusCompleted {
				fmt.Printf("- [ ] %d: %s\n", t.ID, t.Title)
			}
		}
		fmt.Println("\n## Completed")
		for _, t := range tasks {
			if t.Status == store.StatusCompleted {
				fmt.Printf("- [x] %d: %s\n", t.ID, t.Title)
			}
		}
		return
	}

	// Default to JSON
	data, _ := json.MarshalIndent(tasks, "", "  ")
	fmt.Println(string(data))
}
