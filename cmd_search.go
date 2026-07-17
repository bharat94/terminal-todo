package main

import (
	"fmt"
	"strings"

	"github.com/bharat94/terminal-todo/store"
)

func cmdSearch(args []string) {
	queryArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--json" {
			queryArgs = append(queryArgs, arg)
		}
	}
	query := strings.Join(queryArgs, " ")
	if query == "" {
		fail(ErrInvalidArgs, "search query required")
	}

	s := loadStore()
	tasks := s.GetAllTasks()

	queryLower := strings.ToLower(query)
	var results []*store.Task
	for _, task := range tasks {
		if strings.Contains(strings.ToLower(task.Title), queryLower) {
			results = append(results, task)
			continue
		}
		for _, tag := range task.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				results = append(results, task)
				break
			}
		}
	}

	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(results))
		for _, task := range results {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks})
		return
	}

	if len(results) == 0 {
		fmt.Println("No tasks found.")
		return
	}

	fmt.Printf("Found %d task(s):\n", len(results))
	for _, t := range results {
		statusIcon := "[ ]"
		switch t.Status {
		case store.StatusInProgress:
			statusIcon = "[/]"
		case store.StatusCompleted:
			statusIcon = "[x]"
		case store.StatusBlocked:
			statusIcon = "[B]"
		}
		fmt.Printf("  %d %s %s\n", t.ID, statusIcon, t.Title)
	}
}
