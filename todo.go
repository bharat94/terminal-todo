package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"terminal-todo/store"
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

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	root, err := findProjectRoot()
	if err != nil {
		if os.Args[1] == "init" || os.Args[1] == "serve" {
			projectRoot, _ = os.Getwd()
		} else {
			fail(ErrNotInitialized, "%s", err)
		}
	} else {
		projectRoot = root
	}

	command := os.Args[1]
	args := os.Args[2:]
	if err := validateCommandArgs(command, args); err != nil {
		fail(ErrInvalidArgs, "%v", err)
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
	case "acquire":
		cmdAcquire(args)
	case "release":
		cmdRelease(args)
	case "decompose":
		cmdDecompose(args)
	case "lineage":
		cmdLineage(args)
	case "update":
		cmdUpdate(args)
	case "block":
		cmdBlock(args)
	case "unblock":
		cmdUnblock(args)
	case "log":
		cmdLog(args)
	case "what-if", "whatif":
		cmdWhatIf(args)
	case "events":
		cmdEvents(args)
	case "watch":
		cmdWatch(args)
	case "my":
		cmdMy(args)
	case "agent-card":
		cmdAgentCard(args)
	case "caps":
		cmdCaps(args)
	case "graph":
		cmdGraph(args)
	case "search":
		cmdSearch(args)
	case "serve":
		cmdServe(args)
	case "backup":
		cmdBackup(args)
	case "restore":
		cmdRestore(args)
	case "doctor":
		cmdDoctor(args)
	case "config":
		cmdConfig(args)
	case "link":
		cmdLink(args)
	case "unlink":
		cmdUnlink(args)
	case "--version", "-v":
		fmt.Printf("todo v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fail(ErrInvalidArgs, "unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Printf("todo v%s - Distributed Multi-Agent Task Orchestration\n\n", version)
	fmt.Print(`Usage: todo <command> [options]

Task Management:
  init                Initialize .terminal-todo/ in current directory
  add "<title>"       Add a new task (--priority, --caps, --tag, --after)
  done <id>           Mark complete (--as owner for claimed tasks)
  status              Show all tasks (--json, --all, --as, --tag)
  cat <id>            Show task details
  rm <id>             Remove a task
  update <id>         Update metadata (--set, --title, --priority, --caps)
  log <id>            Append to task audit trail (--msg, --as)
  next                Show tasks ready to work (--json, --capabilities)
  search <query>      Search tasks by title or tag

Agent Operations:
  claim <id> --as <n>  Secure an exclusive execution lease (--ttl)
  acquire --as <n> --request-id <id>
                       Atomically select and claim ready work
  release <id> --as <n> Yield an owned lease back to the pool (--error)
  my --as <owner>      Show tasks claimed by you
  agent-card [--as <n>] Register or query agent identity (--caps, --desc, --max-load)
  caps [--all]          Show capability demand across all tasks
  block <id>           Mark a task as blocked (--reason, --as)
  unblock <id>         Unblock a task (--as)

DAG & Dependency:
  depends <id>        Show what this task depends on
  dependents <id>     Show tasks that depend on this
  decompose <id>      Split a task into sub-tasks (--into, --as)
  lineage <id>        Show recursive decomposition tree (--json)
  what-if <id>        Simulate completing/blocking a task
  graph [--dot]       Visualize the DAG topology (DOT/JSON/text)

Reactivity:
  watch [<id>]        Live-refresh task dashboard (--poll)
  events [<since>]    Show the event log (--json)

Project:
  config [key=value]  View or set project configuration
  export              Export tasks (--markdown)
  prune               Remove all completed tasks
  backup [--output]   Snapshot the task store
  restore <path>      Restore tasks from a backup
  doctor [--fix]      Diagnose project health and repair issues
  link <alias> <path> Register a linked repository
  unlink <alias>      Remove a linked repository alias
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
		fail(ErrStoreCorrupted, "%v", err)
	}
	return s
}

func parseIDs(args []string) []uint64 {
	var ids []uint64
	valueOptions := map[string]bool{
		"--after": true, "--as": true, "--ttl": true,
		"--capabilities": true, "--caps": true, "--priority": true,
		"--into": true, "--title": true, "--set": true,
		"--reason": true, "--msg": true, "--tag": true,
		"--add-dep": true, "--remove-dep": true,
		"--error": true, "--poll": true, "--output": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if valueOptions[arg] {
			i++
			continue
		}
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
		"add":        {"--after": true, "--priority": true, "--caps": true, "--tag": true},
		"backup":     {"--output": true},
		"block":      {"--reason": true, "--as": true},
		"claim":      {"--as": true, "--ttl": true},
		"acquire":    {"--as": true, "--request-id": true, "--ttl": true, "--capabilities": true},
		"decompose":  {"--into": true},
		"done":       {"--as": true},
		"log":        {"--msg": true, "--as": true},
		"next":       {"--capabilities": true},
		"release":    {"--as": true, "--error": true},
		"unblock":    {"--as": true},
		"update":     {"--title": true, "--priority": true, "--caps": true, "--set": true, "--as": true, "--add-dep": true, "--remove-dep": true},
		"status":     {"--tag": true, "--as": true},
		"watch":      {"--poll": true},
		"my":         {"--as": true},
		"agent-card": {"--as": true, "--caps": true, "--desc": true, "--max-load": true},
		"caps":       {"--as": true},
	}
	booleanFlags := map[string]map[string]bool{
		"add":        {"--json": true},
		"claim":      {"--json": true},
		"acquire":    {"--json": true},
		"done":       {"--json": true},
		"release":    {"--json": true},
		"cat":        {"--json": true},
		"status":     {"--json": true, "--all": true},
		"next":       {"--json": true, "--ready": true},
		"lineage":    {"--json": true},
		"update":     {"--json": true},
		"export":     {"--markdown": true},
		"graph":      {"--dot": true, "--json": true},
		"events":     {"--json": true},
		"doctor":     {"--fix": true},
		"what-if":    {"--done": true, "--claim": true, "--block": true, "--json": true},
		"whatif":     {"--done": true, "--claim": true, "--block": true, "--json": true},
		"depends":    {"--json": true},
		"dependents": {"--json": true},
		"search":     {"--json": true},
		"my":         {"--json": true},
		"agent-card": {"--json": true},
		"caps":       {"--json": true, "--all": true},
		"serve":      {"--stdio": true},
	}
	knownCommands := map[string]bool{
		"init": true, "add": true, "done": true, "status": true,
		"cat": true, "rm": true, "depends": true, "dependents": true,
		"next": true, "export": true, "prune": true, "claim": true, "acquire": true,
		"release": true, "decompose": true, "lineage": true, "update": true,
		"config": true, "link": true, "unlink": true, "block": true, "unblock": true,
		"log": true, "search": true, "doctor": true, "backup": true,
		"restore": true, "what-if": true, "whatif": true, "events": true,
		"watch": true, "my": true, "graph": true, "serve": true,
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
		if arg == "--after" || arg == "--as" || arg == "--ttl" || arg == "--capabilities" || arg == "--caps" || arg == "--priority" || arg == "--into" || arg == "--reason" || arg == "--msg" || arg == "--tag" || arg == "--add-dep" || arg == "--remove-dep" || arg == "--set" || arg == "--output" || arg == "--error" || arg == "--poll" {
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
