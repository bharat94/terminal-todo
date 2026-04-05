package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"terminal-todo/store"
)

func cmdStatus(args []string) {
	s := loadStore()
	tasks := s.GetAllTasks()

	if hasFlag(args, "--json") {
		type jsonTask struct {
			ID       uint64   `json:"id"`
			Title    string   `json:"title"`
			Status   string   `json:"status"`
			Depends  []uint64 `json:"depends"`
			Created  uint64   `json:"created"`
			Completed uint64  `json:"completed,omitempty"`
		}
		var jsonTasks []jsonTask
		for _, t := range tasks {
			statusStr := "pending"
			if t.Status == store.StatusInProgress {
				statusStr = "in_progress"
			} else if t.Status == store.StatusCompleted {
				statusStr = "completed"
			}
			jsonTasks = append(jsonTasks, jsonTask{
				ID:        t.ID,
				Title:     t.Title,
				Status:    statusStr,
				Depends:   t.Depends,
				Created:   t.Created,
				Completed: t.Completed,
			})
		}
		data, _ := json.Marshal(map[string]interface{}{"tasks": jsonTasks})
		fmt.Println(string(data))
		return
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks yet. Run 'todo add <title>' to add one.")
		return
	}

	fmt.Printf("%-5s %-12s %-30s %s\n", "ID", "STATUS", "TITLE", "DEPENDS")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range tasks {
		statusStr := "[ ]"
		if t.Status == store.StatusInProgress {
			statusStr = "[~]"
		} else if t.Status == store.StatusCompleted {
			statusStr = "[x]"
		}
		depsStr := "-"
		if len(t.Depends) > 0 {
			depsStr = fmt.Sprint(t.Depends)
		}
		fmt.Printf("%-5d %-12s %-30s %s\n", t.ID, statusStr, truncate(t.Title, 30), depsStr)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
