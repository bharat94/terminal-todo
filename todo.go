package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bharat94/terminal-todo/store"
)

var (
	projectRoot string
	version     = "dev"
)

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

// activeCommand is the command resolved for this invocation. Argument scanning
// consults it to tell a flag's value apart from a positional argument, which is
// what lets those helpers stay correct without a second copy of the flag table.
var activeCommand *command

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	name := os.Args[1]
	args := os.Args[2:]

	switch name {
	case "--version", "-v":
		fmt.Printf("todo v%s\n", version)
		return
	case "help", "--help", "-h":
		if len(args) > 0 {
			if cmd, ok := lookupCommand(args[0]); ok {
				fmt.Print(renderCommandHelp(cmd))
				return
			}
			fail(ErrInvalidArgs, "unknown command: %s", args[0])
		}
		printUsage()
		return
	}

	cmd, known := lookupCommand(name)
	if !known {
		fail(ErrInvalidArgs, "unknown command: %s", name)
	}
	activeCommand = cmd

	root, err := findProjectRoot()
	if err != nil {
		if cmd.AllowsUninitializedProject {
			projectRoot, _ = os.Getwd()
		} else {
			fail(ErrNotInitialized, "%s", err)
		}
	} else {
		projectRoot = root
	}

	if err := cmd.validateArgs(args); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	cmd.Run(args)
}

func printUsage() {
	fmt.Print(renderUsage())
}

func loadStore() *store.TaskStore {
	s, err := store.LoadCurrent(tasksBinPath())
	if err != nil {
		fail(ErrStoreCorrupted, "loading store: %v", err)
	}
	return s
}

func saveStore(s *store.TaskStore) {
	if err := s.Save(tasksBinPath()); err != nil {
		fail(ErrStoreCorrupted, "saving store: %v", err)
	}
}

func updateStore(mutate func(*store.TaskStore) error) *store.TaskStore {
	s, err := store.Update(tasksBinPath(), mutate)
	if err != nil {
		if isPersistedInputFailure(err) {
			fail(ErrInvalidArgs, "%v", err)
		}
		if code, ok := classifyCoordinationError(err); ok {
			fail(code, "%v", err)
		}
		fail(ErrStoreCorrupted, "%v", err)
	}
	return s
}

// parseIDs extracts positional task IDs, skipping the values that belong to
// flags. The skip set comes from the running command's own declaration, so a
// newly added value flag cannot be mistaken for a task ID.
func parseIDs(args []string) []uint64 {
	return parseIDsWithValueFlags(args, activeCommandValueFlags())
}

func activeCommandValueFlags() map[string]bool {
	if activeCommand == nil {
		return map[string]bool{}
	}
	return activeCommand.valueFlags()
}

func parseIDsWithValueFlags(args []string, valueFlags map[string]bool) []uint64 {
	var ids []uint64
	seen := make(map[uint64]struct{})
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if valueFlags[arg] {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if id, err := strconv.ParseUint(arg, 10, 64); err == nil && id > 0 {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
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

// extractTitle joins the positional words of a command into one title,
// skipping flags and the values that belong to them. Like parseIDs, the skip
// set comes from the running command's declaration.
func extractTitle(args []string) string {
	valueFlags := activeCommandValueFlags()
	var titleParts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if valueFlags[arg] {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		titleParts = append(titleParts, arg)
	}
	return strings.Join(titleParts, " ")
}
