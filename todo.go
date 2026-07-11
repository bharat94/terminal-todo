package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	if err := validateCommandArgs(command, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

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
	case "claim":
		cmdClaim(args)
	case "release":
		cmdRelease(args)
	case "decompose":
		cmdDecompose(args)
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
  add "<title>"       Add a new task (--priority 0..1, --caps go,testing)
  add "<title>" --after <id> Add task with dependency
  done <id>           Mark complete (--as owner for claimed tasks)
  status              Show all tasks
  status --json       Show all tasks in JSON format
  cat <id>            Show task details
  rm <id>             Remove a task
  depends <id>        Show what this task depends on
  dependents <id>     Show tasks that depend on this
  next                Show tasks ready to work
  next --json         Show ready tasks in JSON format
  claim <id> --as <n> Secure an exclusive execution lease
  release <id> --as <n> Yield an owned lease back to the pool
  decompose <id> --into <json> Split a task into sub-tasks
  export              Export tasks to JSON
  export --markdown  Export tasks to Markdown
  prune               Remove all completed tasks
  help                Show this help

Examples:
  todo add "Implement auth"
  todo add "Fix bug" --after 1
  todo claim 1 --as agent-alpha
  todo decompose 1 --into '{"subtasks":[{"title":"Sub1"}]}'
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

func updateStore(mutate func(*store.TaskStore) error) *store.TaskStore {
	s, err := store.Update(tasksBinPath(), mutate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return s
}

func parseIDs(args []string) []uint64 {
	var ids []uint64
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if id, err := strconv.ParseUint(arg, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func validateCommandArgs(command string, args []string) error {
	valueFlags := map[string]map[string]bool{
		"add":       {"--after": true, "--priority": true, "--caps": true},
		"claim":     {"--as": true, "--ttl": true},
		"decompose": {"--into": true},
		"done":      {"--as": true},
		"next":      {"--capabilities": true},
		"release":   {"--as": true},
	}
	booleanFlags := map[string]map[string]bool{
		"cat":    {"--json": true},
		"status": {"--json": true},
		"next":   {"--json": true, "--ready": true},
		"export": {"--markdown": true},
	}
	knownCommands := map[string]bool{
		"init": true, "add": true, "done": true, "status": true,
		"cat": true, "rm": true, "depends": true, "dependents": true,
		"next": true, "export": true, "prune": true, "claim": true,
		"release": true, "decompose": true,
	}
	if !knownCommands[command] {
		return nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if valueFlags[command][arg] {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return fmt.Errorf("%s requires a value", arg)
			}
			i++
			continue
		}
		if booleanFlags[command][arg] {
			continue
		}
		return fmt.Errorf("unknown option %s for %s", arg, command)
	}
	return nil
}

func extractAfterIDs(args []string) []string {
	var ids []string
	for i, arg := range args {
		if arg == "--after" && i+1 < len(args) {
			ids = append(ids, args[i+1])
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

func optionValue(args []string, option string) string {
	for i, arg := range args {
		if arg == option && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func extractTitle(args []string) string {
	var titleParts []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--after" || arg == "--as" || arg == "--ttl" || arg == "--capabilities" || arg == "--caps" || arg == "--priority" || arg == "--into" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		titleParts = append(titleParts, arg)
	}
	return strings.Join(titleParts, " ")
}
