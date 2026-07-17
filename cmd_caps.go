package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bharat94/terminal-todo/store"
)

type capabilityDemand struct {
	Capability string `json:"capability"`
	TaskCount  int    `json:"task_count"`
}

type capsEnvelope struct {
	SchemaVersion         string             `json:"schema_version"`
	RequiredCapabilities  []capabilityDemand `json:"required_capabilities"`
	TotalUnclaimedTasks   int                `json:"total_unclaimed_tasks"`
	TasksWithoutCaps      int                `json:"tasks_without_caps"`
	RegisteredAgents      int                `json:"registered_agents"`
	UnmatchedCapabilities []string           `json:"unmatched_capabilities"`
}

func cmdCaps(args []string) {
	agentName := optionValue(args, "--as")
	showAll := hasFlag(args, "--all")
	hasJSON := hasFlag(args, "--json")

	var allTasks []*store.Task

	s := loadStore()
	allTasks = append(allTasks, s.GetAllTasks()...)

	if showAll {
		registry, err := loadRepositoryRegistry()
		if err == nil {
			for _, linkedPath := range registry.Repositories {
				if !filepath.IsAbs(linkedPath) {
					linkedPath = filepath.Join(projectRoot, linkedPath)
				}
				linkedStore, err := store.LoadCurrent(filepath.Join(filepath.Clean(linkedPath), ".terminal-todo", "tasks.bin"))
				if err == nil {
					allTasks = append(allTasks, linkedStore.GetAllTasks()...)
				}
			}
		}
	}

	capsCount := make(map[string]int)
	totalUnclaimed := 0
	tasksWithoutCaps := 0

	for _, t := range allTasks {
		if t.Status == store.StatusCompleted {
			continue
		}
		if agentName != "" {
			if t.Owner == agentName {
				continue
			}
		} else {
			if t.Owner != "" {
				continue
			}
		}

		totalUnclaimed++
		if len(t.Capabilities) == 0 {
			tasksWithoutCaps++
		} else {
			for _, cap := range t.Capabilities {
				capsCount[cap]++
			}
		}
	}

	agentRegistry, _ := loadAgentRegistry()
	registeredAgents := len(agentRegistry.Agents)

	agentCaps := make(map[string]bool)
	for _, card := range agentRegistry.Agents {
		for _, cap := range card.Capabilities {
			agentCaps[cap] = true
		}
	}
	var unmatched []string
	for cap := range capsCount {
		if !agentCaps[cap] {
			unmatched = append(unmatched, cap)
		}
	}
	sort.Strings(unmatched)

	var capsList []capabilityDemand
	for cap, count := range capsCount {
		capsList = append(capsList, capabilityDemand{Capability: cap, TaskCount: count})
	}
	sort.Slice(capsList, func(i, j int) bool {
		if capsList[i].TaskCount != capsList[j].TaskCount {
			return capsList[i].TaskCount > capsList[j].TaskCount
		}
		return capsList[i].Capability < capsList[j].Capability
	})

	if hasJSON {
		writeJSON(capsEnvelope{
			SchemaVersion:         "1",
			RequiredCapabilities:  capsList,
			TotalUnclaimedTasks:   totalUnclaimed,
			TasksWithoutCaps:      tasksWithoutCaps,
			RegisteredAgents:      registeredAgents,
			UnmatchedCapabilities: unmatched,
		})
		return
	}

	if totalUnclaimed == 0 {
		fmt.Println("No unclaimed tasks found.")
		return
	}

	fmt.Println("Capability Demand:")
	if len(capsList) > 0 {
		unmatchedSet := make(map[string]bool, len(unmatched))
		for _, u := range unmatched {
			unmatchedSet[u] = true
		}
		for _, cd := range capsList {
			label := cd.Capability + ":"
			suffix := ""
			if unmatchedSet[cd.Capability] {
				suffix = " (unmatched)"
			}
			fmt.Printf("  %-15s %d tasks%s\n", label, cd.TaskCount, suffix)
		}
	} else {
		fmt.Println("  (none)")
	}
	fmt.Println()
	fmt.Printf("Total unclaimed tasks: %d\n", totalUnclaimed)
	if tasksWithoutCaps > 0 {
		fmt.Printf("Tasks without capabilities: %d\n", tasksWithoutCaps)
	}
	fmt.Printf("Registered agents: %d\n", registeredAgents)
	if len(unmatched) > 0 {
		fmt.Printf("Unmatched capabilities: %s\n", strings.Join(unmatched, ", "))
	}
}
