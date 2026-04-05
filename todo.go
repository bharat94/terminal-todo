package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"terminal-todo/store"
)

var projectRoot string

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		ttDir := filepath.Join(dir, ".terminal-todo")
		if _, err := os.Stat(ttDir); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not in a project (no .terminal-todo/ found)")
}

func tasksBinPath() string {
	return filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	root, err := findProjectRoot()
	if err != nil {
		if os.Args[1] == "init" {
			projectRoot, _ = os.Getwd()
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
	} else {
		projectRoot = root
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		cmdInit(args)
	case "add":
		cmdAdd(args)
	case "done":
		cmdDone(args)
	case "status":
		cmdStatus(args)
	case "cat":
		cmdCat(args)
	case "rm":
		cmdRm(args)
	case "depends":
		cmdDepends(args)
	case "dependents":
		cmdDependents(args)
	case "next":
		cmdNext(args)
	case "export":
		cmdExport(args)
	case "prune":
		cmdPrune(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`Usage: todo <command> [options]

Commands:
  init                Initialize .terminal-todo/ in current directory
  add "<title>"       Add a new task
  add "<title>" --after <id> Add task with dependency
  done <id>           Mark task as complete
  status              Show all tasks
  status --json       Show all tasks in JSON format
  cat <id>            Show task details
  rm <id>             Remove a task
  depends <id>        Show what this task depends on
  dependents <id>     Show tasks that depend on this
  next                Show tasks ready to work
  next --json         Show ready tasks in JSON format
  export              Export tasks to JSON
  export --markdown  Export tasks to Markdown
  prune               Remove all completed tasks
  help                Show this help

Examples:
  todo add "Implement auth"
  todo add "Fix bug" --after 1
  todo done 1
  todo status --json
`)
}

func loadStore() *store.TaskStore {
	s, err := store.Load(tasksBinPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading store: %v\n", err)
		os.Exit(1)
	}
	return s
}

func saveStore(s *store.TaskStore) {
	if err := s.Save(tasksBinPath()); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving store: %v\n", err)
		os.Exit(1)
	}
}

func parseIDs(args []string) []uint64 {
	var ids []uint64
	for _, arg := range args {
		if arg == "--json" || arg == "--pretty" || arg == "--markdown" {
			continue
		}
		var id uint64
		if _, err := fmt.Sscanf(arg, "%d", &id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func extractTitle(args []string) string {
	var titleParts []string
	inTitle := false
	for _, arg := range args {
		if arg == "--after" || strings.HasPrefix(arg, "--") {
			inTitle = false
			continue
		}
		if !inTitle && !strings.HasPrefix(arg, "-") {
			inTitle = true
		}
		if inTitle {
			titleParts = append(titleParts, arg)
		}
	}
	return strings.Join(titleParts, " ")
}
