package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bharat94/terminal-todo/dag"
)

func cmdLink(args []string) {
	if len(args) != 2 {
		fail(ErrInvalidArgs, "usage: todo link <alias> <project-path>")
	}
	alias := args[0]
	if alias == "local" {
		fail(ErrInvalidArgs, "repository alias local is reserved")
	}
	if _, _, err := dag.ParseTaskURI(fmt.Sprintf("todo://%s/1", alias)); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	target, err := filepath.Abs(args[1])
	if err != nil {
		fail(ErrInvalidArgs, "resolving repository path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".terminal-todo", "tasks.bin")); err != nil {
		fail(ErrNotInitialized, "%s is not an initialized todo project", target)
	}
	currentInfo, currentErr := os.Stat(projectRoot)
	targetInfo, targetErr := os.Stat(target)
	if currentErr == nil && targetErr == nil && os.SameFile(currentInfo, targetInfo) {
		fail(ErrInvalidArgs, "cannot link a project to itself")
	}
	storedPath := target
	if relative, err := filepath.Rel(projectRoot, target); err == nil {
		storedPath = relative
	}

	if err := updateRepositoryRegistry(func(registry *repositoryRegistry) error {
		registry.Repositories[alias] = storedPath
		return nil
	}); err != nil {
		fail(ErrStoreCorrupted, "saving repository registry: %v", err)
	}
	fmt.Printf("Linked %s to %s\n", alias, storedPath)
}
