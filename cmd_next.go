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

	ready := d.GetReadyTasks(s.Tasks)

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
		return ready[i].Priority > ready[j].Priority
	})

	if hasFlag(args, "--json") {
		output, _ := json.MarshalIndent(map[string]interface{}{"ready": ready}, "", "  ")
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
		if capMap[c] {
			return true
		}
	}
	return false
}
