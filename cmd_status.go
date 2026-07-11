package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"terminal-todo/store"
)

func cmdStatus(args []string) {
	s := loadStore()
	tasks := s.GetAllTasks()

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(tasks))
		for _, task := range tasks {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		output, err := json.MarshalIndent(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
		return
	}

	fmt.Printf("%-4s %-12s %-30s %-20s %s\n", "ID", "STATUS", "TITLE", "OWNER", "DEPENDS")
	for _, t := range tasks {
		statusStr := "[ ]"
		switch t.Status {
		case store.StatusInProgress:
			statusStr = "[/]"
		case store.StatusCompleted:
			statusStr = "[x]"
		case store.StatusBlocked:
			statusStr = "[B]"
		}

		owner := t.Owner
		if owner == "" {
			owner = "-"
		}

		deps := strings.Join(t.Depends, ", ")
		if deps == "" {
			deps = "-"
		}

		fmt.Printf("%-4d %-12s %-30s %-20s %s\n", t.ID, statusStr, t.Title, owner, deps)
	}
}
