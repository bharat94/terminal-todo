package main

import (
	"fmt"
	"sort"
	"time"

	"terminal-todo/store"
)

func cmdMy(args []string) {
	owner := optionValue(args, "--as")
	if owner == "" {
		fail(ErrInvalidArgs, "--as <agent-name> is required")
	}

	s := loadStore()
	tasks := s.GetAllTasks()

	var mine []*store.Task
	for _, t := range tasks {
		if t.Owner == owner {
			mine = append(mine, t)
		}
	}

	sort.Slice(mine, func(i, j int) bool {
		if mine[i].Status != mine[j].Status {
			return mine[i].Status < mine[j].Status
		}
		return mine[i].ID < mine[j].ID
	})

	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(mine))
		for _, task := range mine {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks})
		return
	}

	if len(mine) == 0 {
		fmt.Printf("No tasks claimed by %s\n", owner)
		return
	}

	fmt.Printf("Tasks claimed by %s:\n\n", owner)
	for _, t := range mine {
		statusIcon := "○"
		switch t.Status {
		case store.StatusInProgress:
			statusIcon = "◐"
		case store.StatusBlocked:
			statusIcon = "⊗"
		}
		leaseInfo := ""
		if t.LeaseExpires > 0 {
			remaining := int64(t.LeaseExpires) - int64(time.Now().UnixMilli())
			if remaining > 0 {
				leaseInfo = fmt.Sprintf(" (lease: %s)", time.Duration(remaining)*time.Millisecond)
			} else {
				leaseInfo = " (lease EXPIRED)"
			}
		}
		fmt.Printf("  %s %d %-40s%s\n", statusIcon, t.ID, t.Title, leaseInfo)
	}
}
