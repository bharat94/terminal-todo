package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/dag"
)

type dependsDep struct {
	ID    uint64 `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	URI   string `json:"uri"`
}

type dependsEnvelope struct {
	SchemaVersion string       `json:"schema_version"`
	TaskID        uint64       `json:"task_id"`
	TaskTitle     string       `json:"task_title"`
	Depends       []dependsDep `json:"depends"`
}

func cmdDepends(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	s := loadStore()
	task, ok := s.GetTask(ids[0])
	if !ok {
		fail(ErrTaskNotFound, "task %d not found", ids[0])
	}

	if hasFlag(args, "--json") {
		deps := make([]dependsDep, 0, len(task.Depends))
		for _, dep := range task.Depends {
			entry := dependsDep{URI: dep}
			if depID, local := dag.ParseLocalID(dep); local {
				entry.ID = depID
				if dt, ok := s.GetTask(depID); ok {
					entry.Title = dt.Title
				}
			}
			deps = append(deps, entry)
		}
		writeJSON(dependsEnvelope{
			SchemaVersion: protocolVersion,
			TaskID:        task.ID,
			TaskTitle:     task.Title,
			Depends:       deps,
		})
		return
	}

	if len(task.Depends) == 0 {
		fmt.Printf("Task %d has no dependencies.\n", ids[0])
		return
	}

	fmt.Printf("Task %d depends on:\n", ids[0])
	for _, dep := range task.Depends {
		depID, local := dag.ParseLocalID(dep)
		if local {
			if dt, ok := s.GetTask(depID); ok {
				fmt.Printf("  - %d: %s\n", depID, dt.Title)
			} else {
				fmt.Printf("  - %d: [not found locally]\n", depID)
			}
		} else {
			fmt.Printf("  - %s\n", dep)
		}
	}
}
