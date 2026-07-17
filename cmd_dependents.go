package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/dag"
)

type dependentItem struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

type dependentsEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	TaskID        uint64          `json:"task_id"`
	TaskTitle     string          `json:"task_title"`
	Dependents    []dependentItem `json:"dependents"`
}

func cmdDependents(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	targetID := ids[0]
	s := loadStore()
	task, ok := s.GetTask(targetID)
	if !ok {
		fail(ErrTaskNotFound, "task %d not found", targetID)
	}

	if hasFlag(args, "--json") {
		deps := make([]dependentItem, 0)
		for _, t := range s.Tasks {
			for _, dep := range t.Depends {
				id, local := dag.ParseLocalID(dep)
				if local && id == targetID {
					deps = append(deps, dependentItem{ID: t.ID, Title: t.Title})
					break
				}
			}
		}
		writeJSON(dependentsEnvelope{
			SchemaVersion: protocolVersion,
			TaskID:        task.ID,
			TaskTitle:     task.Title,
			Dependents:    deps,
		})
		return
	}

	fmt.Printf("Tasks that depend on %d:\n", targetID)
	found := false
	for _, t := range s.Tasks {
		for _, dep := range t.Depends {
			id, local := dag.ParseLocalID(dep)
			if local && id == targetID {
				fmt.Printf("  - %d: %s\n", t.ID, t.Title)
				found = true
				break
			}
		}
	}

	if !found {
		fmt.Println("  None.")
	}
}
