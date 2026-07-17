package main

import (
	"fmt"
	"strconv"
	"strings"
)

func cmdBootstrap(args []string) {
	actor := optionValue(args, "--as")
	limit, err := bootstrapIntOption(args, "--limit", defaultBootstrapLimit)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	eventLimit, err := bootstrapIntOption(args, "--events", defaultBootstrapEventLimit)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	objectiveID, err := bootstrapUintOption(args, "--objective")
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	capabilitiesExplicit := hasFlag(args, "--capabilities")
	var capabilities []string
	if capabilitiesExplicit {
		capabilities = normalizeCapabilities(optionValue(args, "--capabilities"))
	}

	s := loadStore()
	registry, err := loadAgentRegistry()
	if err != nil {
		fail(ErrStoreCorrupted, "loading agent registry: %v", err)
	}
	result, err := buildBootstrap(s, registry, bootstrapParams{
		Actor:        actor,
		Capabilities: capabilities,
		ObjectiveID:  objectiveID,
		Limit:        limit,
		EventLimit:   eventLimit,
	}, capabilitiesExplicit, snapshotDependencyResolver(s.GetAllTasks()))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			fail(ErrTaskNotFound, "%v", err)
		}
		fail(ErrInvalidArgs, "%v", err)
	}

	if hasFlag(args, "--json") {
		writeJSON(result)
		return
	}
	printBootstrap(result)
}

func bootstrapIntOption(args []string, name string, fallback int) (int, error) {
	value := optionValue(args, name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func bootstrapUintOption(args []string, name string) (uint64, error) {
	value := optionValue(args, name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("%s must be a positive task ID", name)
	}
	return parsed, nil
}

func printBootstrap(result bootstrapResult) {
	if result.Objective == nil {
		fmt.Println("Objective: none")
	} else {
		fmt.Printf("Objective: %d [%s] %s\n", result.Objective.ID, result.Objective.Status, result.Objective.Title)
		if result.Objective.BlockReason != "" {
			fmt.Printf("  blocked: %s\n", result.Objective.BlockReason)
		}
	}
	fmt.Printf(
		"Progress: %d/%d complete (%d%%); %d active, %d pending, %d blocked\n",
		result.Progress.Completed,
		result.Progress.Total,
		result.Progress.Percent,
		result.Progress.InProgress,
		result.Progress.Pending,
		result.Progress.Blocked,
	)
	fmt.Printf(
		"Worker: %s; %d owned; %d compatible ready",
		result.Worker.Actor,
		result.OwnedWork.Total,
		result.ReadyWork.Total,
	)
	if result.Worker.AtCapacity {
		fmt.Print("; at capacity")
	}
	fmt.Println()

	printBootstrapTasks("Owned work", result.OwnedWork)
	printBootstrapTasks("Ready work", result.ReadyWork)

	fmt.Printf(
		"Blockers: %d explicit, %d dependency-blocked",
		result.Blockers.ExplicitTotal,
		result.Blockers.DependencyBlockedTotal,
	)
	if len(result.Blockers.PrimaryDependencies) > 0 {
		fmt.Printf("; dependencies: %s", strings.Join(result.Blockers.PrimaryDependencies, ", "))
	}
	fmt.Println()
	for _, task := range result.Blockers.Explicit {
		fmt.Printf("  - %d: %s", task.ID, task.Title)
		if task.BlockReason != "" {
			fmt.Printf(" — %s", task.BlockReason)
		}
		fmt.Println()
	}

	fmt.Printf(
		"Capability demand: %d unclaimed (%d without requirements)\n",
		result.CapabilityDemand.UnclaimedTasks,
		result.CapabilityDemand.WithoutCaps,
	)
	for _, demand := range result.CapabilityDemand.Items {
		match := ""
		if !demand.Matched {
			match = " (unmatched)"
		}
		fmt.Printf("  - %s: %d%s\n", demand.Capability, demand.TaskCount, match)
	}

	fmt.Printf("Recent events: %d shown of %d\n", len(result.RecentEvents.Items), result.RecentEvents.Total)
	for _, event := range result.RecentEvents.Items {
		fmt.Printf("  - #%d %s task %d", event.ID, event.Type, event.TaskID)
		if event.Actor != "" {
			fmt.Printf(" by %s", event.Actor)
		}
		if event.Detail != "" {
			fmt.Printf(" (%s)", event.Detail)
		}
		fmt.Println()
	}
}

func printBootstrapTasks(label string, tasks bootstrapTaskList) {
	fmt.Printf("%s: %d shown of %d\n", label, len(tasks.Items), tasks.Total)
	for _, task := range tasks.Items {
		fmt.Printf("  - %d [%s] %s", task.ID, task.Status, task.Title)
		if task.Owner != "" {
			fmt.Printf(" — %s", task.Owner)
		}
		fmt.Println()
	}
}
