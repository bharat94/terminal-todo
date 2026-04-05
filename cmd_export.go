package main

import (
	"encoding/json"
	"fmt"

	"terminal-todo/store"
)

func cmdExport(args []string) {
	s := loadStore()

	if hasFlag(args, "--markdown") {
		exportMarkdown(s)
		return
	}

	if hasFlag(args, "--pretty") {
		data, _ := json.MarshalIndent(map[string]interface{}{"tasks": tasksToJSON(s)}, "", "  ")
		fmt.Println(string(data))
		return
	}

	data, _ := json.Marshal(map[string]interface{}{"tasks": tasksToJSON(s)})
	fmt.Println(string(data))
}

func tasksToJSON(s *store.TaskStore) []map[string]interface{} {
	var result []map[string]interface{}
	for _, t := range s.GetAllTasks() {
		taskMap := map[string]interface{}{
			"id":       t.ID,
			"title":    t.Title,
			"status":   statusToString(t.Status),
			"depends":  t.Depends,
			"created":  t.Created,
		}
		if t.Completed > 0 {
			taskMap["completed"] = t.Completed
		}
		result = append(result, taskMap)
	}
	return result
}

func statusToString(status store.TaskStatus) string {
	switch status {
	case store.StatusPending:
		return "pending"
	case store.StatusInProgress:
		return "in_progress"
	case store.StatusCompleted:
		return "completed"
	default:
		return "unknown"
	}
}

func exportMarkdown(s *store.TaskStore) {
	tasks := s.GetAllTasks()

	fmt.Println("# Project Tasks")
	fmt.Println()

	var completed, pending []string
	for _, t := range tasks {
		line := fmt.Sprintf("- [%s] ID %d: %s", statusChar(t.Status), t.ID, t.Title)
		if len(t.Depends) > 0 {
			var depTitles []string
			for _, depID := range t.Depends {
				if dep, ok := s.GetTask(depID); ok {
					depTitles = append(depTitles, fmt.Sprintf("%d - %s", depID, dep.Title))
				}
			}
			line += fmt.Sprintf(" (blocked by: %s)", joinStrings(depTitles))
		}

		if t.Status == store.StatusCompleted {
			completed = append(completed, line)
		} else {
			pending = append(pending, line)
		}
	}

	if len(completed) > 0 {
		fmt.Println("## Completed")
		for _, c := range completed {
			fmt.Println(c)
		}
		fmt.Println()
	}

	if len(pending) > 0 {
		fmt.Println("## Pending")
		for _, p := range pending {
			fmt.Println(p)
		}
	}
}

func statusChar(status store.TaskStatus) string {
	if status == store.StatusCompleted {
		return "x"
	}
	return " "
}
