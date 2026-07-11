package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"terminal-todo/store"
)

type lineageProgress struct {
	Total      int     `json:"total"`
	Pending    int     `json:"pending"`
	InProgress int     `json:"in_progress"`
	Completed  int     `json:"completed"`
	Blocked    int     `json:"blocked"`
	Percent    float64 `json:"percent_complete"`
}

type lineageEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Root          protocolTask    `json:"root"`
	Descendants   []protocolTask  `json:"descendants"`
	Progress      lineageProgress `json:"progress"`
}

func cmdLineage(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	s := loadStore()
	root, ok := s.GetTask(ids[0])
	if !ok {
		fail(ErrTaskNotFound, "task %d not found", ids[0])
	}
	descendants := lineageDescendants(root.ID, s.Tasks)
	progress := calculateLineageProgress(append([]*store.Task{root}, descendants...))

	if hasFlag(args, "--json") {
		protocolDescendants := make([]protocolTask, 0, len(descendants))
		for _, task := range descendants {
			protocolDescendants = append(protocolDescendants, newProtocolTask(task))
		}
		output, err := json.MarshalIndent(lineageEnvelope{
			SchemaVersion: protocolVersion,
			Root:          newProtocolTask(root),
			Descendants:   protocolDescendants,
			Progress:      progress,
		}, "", "  ")
		if err != nil {
			fail(ErrStoreCorrupted, "Error encoding JSON: %v", err)
		}
		fmt.Println(string(output))
		return
	}

	fmt.Printf("Objective %d: %s\n", root.ID, root.Title)
	fmt.Printf("Progress: %d/%d complete (%.0f%%)\n", progress.Completed, progress.Total, progress.Percent)
	for _, task := range descendants {
		fmt.Printf("- %d [%s] %s\n", task.ID, statusName(task.Status), task.Title)
	}
}

func lineageDescendants(rootID uint64, tasks map[uint64]*store.Task) []*store.Task {
	children := make(map[string][]*store.Task)
	for _, task := range tasks {
		children[task.Lineage] = append(children[task.Lineage], task)
	}
	for lineage := range children {
		sort.Slice(children[lineage], func(i, j int) bool { return children[lineage][i].ID < children[lineage][j].ID })
	}

	var result []*store.Task
	visited := map[uint64]bool{rootID: true}
	var walk func(uint64)
	walk = func(parentID uint64) {
		for _, child := range children[fmt.Sprintf("todo://local/%d", parentID)] {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			result = append(result, child)
			walk(child.ID)
		}
	}
	walk(rootID)
	return result
}

func calculateLineageProgress(tasks []*store.Task) lineageProgress {
	progress := lineageProgress{Total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case store.StatusPending:
			progress.Pending++
		case store.StatusInProgress:
			progress.InProgress++
		case store.StatusCompleted:
			progress.Completed++
		case store.StatusBlocked:
			progress.Blocked++
		}
	}
	if progress.Total > 0 {
		progress.Percent = float64(progress.Completed) / float64(progress.Total) * 100
	}
	return progress
}
