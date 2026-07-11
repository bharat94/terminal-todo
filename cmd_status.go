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

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

func cmdStatus(args []string) {
	if hasFlag(args, "--all") {
		cmdStatusAll(args)
		return
	}
	s := loadStore()
	tasks := s.GetAllTasks()

	filterTag := optionValue(args, "--tag")
	filterAgent := optionValue(args, "--as")

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(tasks))
		for _, task := range tasks {
			if filterTag != "" && !hasTag(task.Tags, filterTag) {
				continue
			}
			if filterAgent != "" && task.Owner != filterAgent {
				continue
			}
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

	grouped := map[store.TaskStatus][]*store.Task{
		store.StatusInProgress: {},
		store.StatusPending:    {},
		store.StatusBlocked:    {},
		store.StatusCompleted:  {},
	}
	for _, t := range tasks {
		if filterTag != "" && !hasTag(t.Tags, filterTag) {
			continue
		}
		if filterAgent != "" && t.Owner != filterAgent {
			continue
		}
		grouped[t.Status] = append(grouped[t.Status], t)
	}

	sectionHeader := func(label string) {
		fmt.Printf("\n%s%s%s\n", colorCyan, label, colorReset)
	}

	statusIcon := func(t *store.Task) string {
		switch t.Status {
		case store.StatusInProgress:
			return colorYellow + " ◐" + colorReset
		case store.StatusPending:
			return " ○"
		case store.StatusBlocked:
			return colorRed + " ⊗" + colorReset
		case store.StatusCompleted:
			return colorGreen + " ●" + colorReset
		default:
			return " ?"
		}
	}

	tagStr := func(tags []string) string {
		if len(tags) == 0 {
			return ""
		}
		colored := make([]string, len(tags))
		for i, tag := range tags {
			colored[i] = colorGray + tag + colorReset
		}
		return " [" + strings.Join(colored, " ") + "]"
	}

	if len(grouped[store.StatusInProgress]) > 0 {
		sectionHeader("In Progress")
		for _, t := range grouped[store.StatusInProgress] {
			fmt.Printf("  %s %d %-40s %s%s\n", statusIcon(t), t.ID, t.Title, colorYellow+t.Owner+colorReset, tagStr(t.Tags))
		}
	}
	if len(grouped[store.StatusPending]) > 0 {
		sectionHeader("Pending")
		for _, t := range grouped[store.StatusPending] {
			fmt.Printf("  %s %d %-40s%s\n", statusIcon(t), t.ID, t.Title, tagStr(t.Tags))
		}
	}
	if len(grouped[store.StatusBlocked]) > 0 {
		sectionHeader("Blocked")
		for _, t := range grouped[store.StatusBlocked] {
			fmt.Printf("  %s %d %-40s%s\n", statusIcon(t), t.ID, t.Title, tagStr(t.Tags))
		}
	}
	if len(grouped[store.StatusCompleted]) > 0 {
		sectionHeader("Completed")
		for _, t := range grouped[store.StatusCompleted] {
			fmt.Printf("  %s %d %-40s%s\n", statusIcon(t), t.ID, t.Title, tagStr(t.Tags))
		}
	}
}

func hasTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
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
