package main

import (
	"fmt"
	"os"
	"path/filepath"

	"terminal-todo/dag"
)

func cmdLink(args []string) {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Error: usage: todo link <alias> <project-path>")
		os.Exit(1)
	}
	alias := args[0]
	if alias == "local" {
		fmt.Fprintln(os.Stderr, "Error: repository alias local is reserved")
		os.Exit(1)
	}
	if _, _, err := dag.ParseTaskURI(fmt.Sprintf("todo://%s/1", alias)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	target, err := filepath.Abs(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving repository path: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(target, ".terminal-todo", "tasks.bin")); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s is not an initialized todo project\n", target)
		os.Exit(1)
	}
	currentInfo, currentErr := os.Stat(projectRoot)
	targetInfo, targetErr := os.Stat(target)
	if currentErr == nil && targetErr == nil && os.SameFile(currentInfo, targetInfo) {
		fmt.Fprintln(os.Stderr, "Error: cannot link a project to itself")
		os.Exit(1)
	}
	storedPath := target
	if relative, err := filepath.Rel(projectRoot, target); err == nil {
		storedPath = relative
	}

	registry, err := loadRepositoryRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading repository registry: %v\n", err)
		os.Exit(1)
	}
	registry.Repositories[alias] = storedPath
	if err := saveRepositoryRegistry(registry); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving repository registry: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Linked %s to %s\n", alias, storedPath)
}
