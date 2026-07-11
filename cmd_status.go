package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"terminal-todo/store"
)

func cmdStatus(args []string) {
	if hasFlag(args, "--all") {
		cmdStatusAll(args)
		return
	}
	s := loadStore()
	tasks := s.GetAllTasks()

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(tasks))
		for _, task := range tasks {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		output, err := json.MarshalIndent(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
		return
	}

	fmt.Printf("%-4s %-12s %-30s %-20s %s\n", "ID", "STATUS", "TITLE", "OWNER", "DEPENDS")
	for _, t := range tasks {
		statusStr := "[ ]"
		switch t.Status {
		case store.StatusInProgress:
			statusStr = "[/]"
		case store.StatusCompleted:
			statusStr = "[x]"
		case store.StatusBlocked:
			statusStr = "[B]"
		}

		owner := t.Owner
		if owner == "" {
			owner = "-"
		}

		deps := strings.Join(t.Depends, ", ")
		if deps == "" {
			deps = "-"
		}

		fmt.Printf("%-4d %-12s %-30s %-20s %s\n", t.ID, statusStr, t.Title, owner, deps)
	}
}

type projectStatus struct {
	Alias     string         `json:"alias"`
	Path      string         `json:"path"`
	Available bool           `json:"available"`
	Error     string         `json:"error,omitempty"`
	Tasks     []protocolTask `json:"tasks"`
}

type projectsEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Projects      []projectStatus `json:"projects"`
}

func cmdStatusAll(args []string) {
	projects := []projectStatus{projectStatusFromStore("local", ".", loadStore())}
	registry, err := loadRepositoryRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading repository registry: %v\n", err)
		os.Exit(1)
	}
	aliases := make([]string, 0, len(registry.Repositories))
	for alias := range registry.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		path := registry.Repositories[alias]
		resolvedPath := path
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(projectRoot, resolvedPath)
		}
		s, err := store.Load(filepath.Join(filepath.Clean(resolvedPath), ".terminal-todo", "tasks.bin"))
		if err != nil {
			projects = append(projects, projectStatus{Alias: alias, Path: path, Available: false, Error: err.Error(), Tasks: []protocolTask{}})
			continue
		}
		projects = append(projects, projectStatusFromStore(alias, path, s))
	}

	if hasFlag(args, "--json") {
		output, err := json.MarshalIndent(projectsEnvelope{SchemaVersion: protocolVersion, Projects: projects}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
		return
	}
	for _, project := range projects {
		fmt.Printf("\n[%s] %s\n", project.Alias, project.Path)
		if !project.Available {
			fmt.Printf("  unavailable: %s\n", project.Error)
			continue
		}
		if len(project.Tasks) == 0 {
			fmt.Println("  No tasks.")
			continue
		}
		for _, task := range project.Tasks {
			fmt.Printf("  %d [%s] %s\n", task.ID, task.Status, task.Title)
		}
	}
}

func projectStatusFromStore(alias, path string, s *store.TaskStore) projectStatus {
	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	protocolTasks := make([]protocolTask, 0, len(tasks))
	for _, task := range tasks {
		protocolTasks = append(protocolTasks, newProtocolTask(task))
	}
	return projectStatus{Alias: alias, Path: path, Available: true, Tasks: protocolTasks}
}
