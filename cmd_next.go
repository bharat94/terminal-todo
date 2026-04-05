package main

import (
	"encoding/json"
	"fmt"

	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdNext(args []string) {
	s := loadStore()
	d := dag.NewDAG()
	dagTasks := convertToDAGTasks(s.Tasks)
	d.BuildFromTasks(dagTasks)

	ready := d.GetReadyTasks(dagTasks)
	blocked := d.GetBlockedTasks(dagTasks)

	if hasFlag(args, "--json") {
		type jsonTask struct {
			ID        uint64  `json:"id"`
			Title     string  `json:"title"`
			BlockedBy []uint64 `json:"blocked_by,omitempty"`
		}
		var readyJSON []jsonTask
		for _, t := range ready {
			blocking := blocked[t.ID]
			readyJSON = append(readyJSON, jsonTask{
				ID:        t.ID,
				Title:     t.Title,
				BlockedBy: blocking,
			})
		}
		data, _ := json.Marshal(map[string]interface{}{"ready": readyJSON})
		fmt.Println(string(data))
		return
	}

	if len(ready) == 0 {
		fmt.Println("No tasks ready to work on.")
		return
	}

	fmt.Println("Ready to work:")
	for _, t := range ready {
		blocking := blocked[t.ID]
		if len(blocking) > 0 {
			var depTitles []string
			for _, depID := range blocking {
				if dep, ok := s.GetTask(depID); ok {
					depTitles = append(depTitles, fmt.Sprintf("%d [%s]", depID, formatStatus(dep.Status)))
				}
			}
			fmt.Printf("  - ID %d: %s (blocked by: %s)\n", t.ID, t.Title, joinStrings(depTitles))
		} else {
			fmt.Printf("  - ID %d: %s (no dependencies)\n", t.ID, t.Title)
		}
	}
}

func formatStatus(status store.TaskStatus) string {
	if status == store.StatusCompleted {
		return "x"
	}
	return " "
}

func joinStrings(strs []string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
