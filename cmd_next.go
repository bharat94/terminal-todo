package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdNext(args []string) {
	s := loadStore()
	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)

	resolver := dependencyResolver()
	ready := d.GetReadyTasksWithResolver(s.Tasks, resolver)

	// Filter by capabilities if requested
	var caps []string
	for i, arg := range args {
		if arg == "--capabilities" && i+1 < len(args) {
			caps = strings.Split(args[i+1], ",")
		}
	}

	if len(caps) > 0 {
		var filtered []*store.Task
		for _, t := range ready {
			if matchesCapabilities(t.Capabilities, caps) {
				filtered = append(filtered, t)
			}
		}
		ready = filtered
	}

	// Sort by priority (descending)
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].Priority == ready[j].Priority {
			return ready[i].ID < ready[j].ID
		}
		return ready[i].Priority > ready[j].Priority
	})

	if hasFlag(args, "--json") {
		available := make([]availableTask, 0, len(ready))
		for _, task := range ready {
			capabilities := append([]string(nil), task.Capabilities...)
			if capabilities == nil {
				capabilities = []string{}
			}
			available = append(available, availableTask{
				ID: task.ID, Title: task.Title, Priority: task.Priority,
				Capabilities: capabilities, Reason: "ready: all dependencies completed",
			})
		}
		output, err := json.MarshalIndent(nextEnvelope{
			SchemaVersion: protocolVersion, AvailableTasks: available,
			BlockedSummary: newBlockedSummaryWithResolver(s.Tasks, resolver),
		}, "", "  ")
		if err != nil {
			fail(ErrStoreCorrupted, "Error encoding JSON: %v", err)
		}
		fmt.Println(string(output))
		return
	}

	if len(ready) == 0 {
		fmt.Println("No tasks ready to work on.")
		return
	}

	fmt.Println("Ready to work:")
	for _, t := range ready {
		fmt.Printf("- %d: %s (Priority: %.2f)\n", t.ID, t.Title, t.Priority)
	}
}

func matchesCapabilities(taskCaps, agentCaps []string) bool {
	if len(taskCaps) == 0 {
		return true
	}
	capMap := make(map[string]bool)
	for _, c := range agentCaps {
		capMap[c] = true
	}
	for _, c := range taskCaps {
		if !capMap[c] {
			return false
		}
	}
	return true
}
