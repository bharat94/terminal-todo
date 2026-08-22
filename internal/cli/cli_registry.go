package cli

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The command registry.
//
// Every fact about a command lives in exactly one place: its name, aliases,
// whether it runs outside an initialized project, which flags it accepts and
// whether each takes a value, its help text, and its handler.
//
// This replaces five parallel tables that had to be edited in lockstep — a
// known-commands set, a value-flag table, a boolean-flag table, the value
// options that argument scanning had to skip, and the skip list used when
// joining a title from positional words. Adding a flag to one and forgetting
// another produced a command that validated its input one way and read it
// another. Deriving all five from one declaration removes the possibility.

// flagKind distinguishes a flag that stands alone from one that consumes the
// argument after it. The distinction is what argument scanning needs in order
// to tell a flag's value apart from a positional argument.
type flagKind int

const (
	// flagBoolean is present or absent: --json.
	flagBoolean flagKind = iota
	// flagValue consumes the next argument: --as <actor>.
	flagValue
	// flagRepeatable consumes the next argument and may appear more than
	// once: --add-dep <ref> --add-dep <ref>.
	flagRepeatable
)

// takesValue reports whether the argument after this flag belongs to it.
func (k flagKind) takesValue() bool {
	return k == flagValue || k == flagRepeatable
}

type flagSpec struct {
	Name string
	Kind flagKind
	Help string
}

type command struct {
	Name    string
	Aliases []string
	// Usage is the invocation form shown in help, without the leading "todo".
	Usage string
	// Summary is the one-line description shown in help.
	Summary string
	// Group orders the command within the help listing.
	Group string
	// AllowsUninitializedProject marks commands that must work before, or
	// without, a .terminal-todo directory.
	AllowsUninitializedProject bool
	// Hidden keeps a command out of help without removing it. Aliases retained
	// for compatibility use this.
	Hidden bool
	Flags  []flagSpec
	Run    func(args []string)
}

// valueFlags returns the flags whose following argument is a value rather than
// a positional argument.
func (c *command) valueFlags() map[string]bool {
	values := make(map[string]bool, len(c.Flags))
	for _, flag := range c.Flags {
		if flag.Kind.takesValue() {
			values[flag.Name] = true
		}
	}
	return values
}

// accepts reports whether the command declares this flag.
func (c *command) accepts(name string) (flagSpec, bool) {
	for _, flag := range c.Flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return flagSpec{}, false
}

// helpGroups is the display order of the help listing.
var helpGroups = []string{
	"Task Management",
	"Agent Operations",
	"DAG & Dependency",
	"Reactivity",
	"Integrations",
	"Project",
}

// Shared flag declarations. Lifecycle mutations offer the same structured
// output opt-ins, and repeating the help text at each site invites drift.
var (
	jsonFlag    = flagSpec{"--json", flagBoolean, "Emit a versioned JSON envelope"}
	receiptFlag = flagSpec{"--receipt", flagBoolean, "Emit a compact mutation receipt"}
	actorFlag   = flagSpec{"--as", flagValue, "Actor identity to attribute the operation to"}
	ttlFlag     = flagSpec{"--ttl", flagValue, "Lease duration, such as 30m"}
)

func mutationFlags(extra ...flagSpec) []flagSpec {
	return append(append([]flagSpec{}, extra...), jsonFlag, receiptFlag)
}

// newCommandRegistry is the single declaration of the CLI surface.
func newCommandRegistry() []*command {
	return []*command{
		// Task management.
		{
			Name: "init", Usage: "init", Group: "Task Management",
			Summary:                    "Initialize .terminal-todo/ in the current directory",
			AllowsUninitializedProject: true,
			Run:                        cmdInit,
		},
		{
			Name: "add", Usage: `add "<title>"`, Group: "Task Management",
			Summary: "Add a new task",
			Flags: mutationFlags(
				flagSpec{"--after", flagRepeatable, "Depend on a task ID or todo:// reference"},
				flagSpec{"--priority", flagValue, "Priority between 0 and 1"},
				flagSpec{"--caps", flagValue, "Comma-separated required capabilities"},
				flagSpec{"--tag", flagRepeatable, "Tag to attach"},
			),
			Run: cmdAdd,
		},
		{
			Name: "done", Usage: "done <id>...", Group: "Task Management",
			Summary: "Mark work complete",
			Flags:   mutationFlags(actorFlag),
			Run:     cmdDone,
		},
		{
			Name: "status", Usage: "status", Group: "Task Management",
			Summary: "Show tasks",
			Flags: []flagSpec{
				{"--tag", flagValue, "Filter by tag"},
				actorFlag,
				jsonFlag,
				{"--all", flagBoolean, "Include completed tasks"},
			},
			Run: cmdStatus,
		},
		{
			Name: "cat", Usage: "cat <id>", Group: "Task Management",
			Summary: "Show task detail",
			Flags:   []flagSpec{jsonFlag},
			Run:     cmdCat,
		},
		{
			Name: "rm", Usage: "rm <id>...", Group: "Task Management",
			Summary: "Remove a task",
			Flags:   mutationFlags(),
			Run:     cmdRm,
		},
		{
			Name: "update", Usage: "update <id>", Group: "Task Management",
			Summary: "Update task metadata",
			Flags: mutationFlags(
				flagSpec{"--title", flagValue, "New title"},
				flagSpec{"--priority", flagValue, "New priority between 0 and 1"},
				flagSpec{"--caps", flagValue, "Replacement comma-separated capabilities"},
				flagSpec{"--set", flagRepeatable, "Structured metadata as key=value"},
				actorFlag,
				flagSpec{"--add-dep", flagRepeatable, "Add a dependency reference"},
				flagSpec{"--remove-dep", flagRepeatable, "Remove a dependency reference"},
			),
			Run: cmdUpdate,
		},
		{
			Name: "log", Usage: "log <id>", Group: "Task Management",
			Summary: "Append to a task's audit trail",
			Flags:   mutationFlags(flagSpec{"--msg", flagValue, "Message to record"}, actorFlag),
			Run:     cmdLog,
		},
		{
			Name: "next", Usage: "next", Group: "Task Management",
			Summary: "Show tasks ready to work",
			Flags: []flagSpec{
				{"--capabilities", flagValue, "Filter by comma-separated capabilities"},
				jsonFlag,
				{"--ready", flagBoolean, "Show only ready tasks"},
			},
			Run: cmdNext,
		},
		{
			Name: "search", Usage: "search <query>", Group: "Task Management",
			Summary: "Search tasks by title or tag",
			Flags:   []flagSpec{jsonFlag},
			Run:     cmdSearch,
		},

		// Agent operations.
		{
			Name: "claim", Usage: "claim <id> --as <actor>", Group: "Agent Operations",
			Summary: "Secure an exclusive execution lease on a specific task",
			Flags:   mutationFlags(actorFlag, ttlFlag),
			Run:     cmdClaim,
		},
		{
			Name: "acquire", Usage: "acquire --as <actor> --request-id <id>", Group: "Agent Operations",
			Summary: "Atomically select and claim the next compatible ready task",
			Flags: mutationFlags(
				actorFlag,
				flagSpec{"--request-id", flagValue, "Idempotency key; retry with the same value"},
				ttlFlag,
				flagSpec{"--capabilities", flagValue, "Override the registered capability set"},
				flagSpec{"--wait", flagValue, "Poll for work up to this duration"},
			),
			Run: cmdAcquire,
		},
		{
			Name: "heartbeat", Usage: "heartbeat <id> --as <actor>", Group: "Agent Operations",
			Summary: "Renew an active owned lease",
			Flags:   mutationFlags(actorFlag, ttlFlag),
			Run:     cmdHeartbeat,
		},
		{
			Name: "handoff", Usage: "handoff <id> --as <actor>", Group: "Agent Operations",
			Summary: "Persist successor context and yield the lease atomically",
			Flags: mutationFlags(
				actorFlag,
				flagSpec{"--set", flagRepeatable, "Handoff context as key=value"},
			),
			Run: cmdHandoff,
		},
		{
			Name: "release", Usage: "release <id> --as <actor>", Group: "Agent Operations",
			Summary: "Yield an owned lease back to the pool",
			Flags: mutationFlags(
				actorFlag,
				flagSpec{"--error", flagValue, "Failure to record for the next worker"},
			),
			Run: cmdRelease,
		},
		{
			Name: "my", Usage: "my --as <actor>", Group: "Agent Operations",
			Summary: "Show tasks currently leased by an actor",
			Flags:   []flagSpec{actorFlag, jsonFlag},
			Run:     cmdMy,
		},
		{
			Name: "bootstrap", Usage: "bootstrap --as <actor>", Group: "Agent Operations",
			Summary: "Get a bounded worker session brief",
			Flags: []flagSpec{
				actorFlag,
				{"--capabilities", flagValue, "Capabilities to match ready work against"},
				{"--objective", flagValue, "Task ID to treat as the objective"},
				{"--limit", flagValue, "Maximum tasks per section"},
				{"--events", flagValue, "Maximum recent events"},
				jsonFlag,
			},
			Run: cmdBootstrap,
		},
		{
			Name: "agent-card", Usage: "agent-card [--as <actor>]", Group: "Agent Operations",
			Summary: "Register or query agent identity",
			Flags: []flagSpec{
				actorFlag,
				{"--caps", flagValue, "Comma-separated capabilities to register"},
				{"--desc", flagValue, "Human-readable description"},
				{"--max-load", flagValue, "Maximum concurrent leases"},
				jsonFlag,
			},
			Run: cmdAgentCard,
		},
		{
			Name: "caps", Usage: "caps", Group: "Agent Operations",
			Summary: "Show capability demand across tasks",
			Flags: []flagSpec{
				actorFlag,
				jsonFlag,
				{"--all", flagBoolean, "Include capabilities with no demand"},
			},
			Run: cmdCaps,
		},
		{
			Name: "block", Usage: "block <id> --reason <text>", Group: "Agent Operations",
			Summary: "Record a blocker and release ownership",
			Flags: mutationFlags(
				flagSpec{"--reason", flagValue, "Why the task is blocked"},
				actorFlag,
			),
			Run: cmdBlock,
		},
		{
			Name: "unblock", Usage: "unblock <id>", Group: "Agent Operations",
			Summary: "Return blocked work to the pending pool",
			Flags:   mutationFlags(actorFlag),
			Run:     cmdUnblock,
		},

		// DAG and dependencies.
		{
			Name: "depends", Usage: "depends <id>", Group: "DAG & Dependency",
			Summary: "Show what this task depends on",
			Flags:   []flagSpec{jsonFlag},
			Run:     cmdDepends,
		},
		{
			Name: "dependents", Usage: "dependents <id>", Group: "DAG & Dependency",
			Summary: "Show tasks that depend on this one",
			Flags:   []flagSpec{jsonFlag},
			Run:     cmdDependents,
		},
		{
			Name: "decompose", Usage: "decompose <id> --into <json>", Group: "DAG & Dependency",
			Summary: "Split a task into sub-tasks",
			Flags: mutationFlags(
				flagSpec{"--into", flagValue, `Subtask document: {"subtasks":[{"title":"…","caps":["go"]}]}`},
				actorFlag,
			),
			Run: cmdDecompose,
		},
		{
			Name: "lineage", Usage: "lineage <id>", Group: "DAG & Dependency",
			Summary: "Show the recursive decomposition tree",
			Flags:   []flagSpec{jsonFlag},
			Run:     cmdLineage,
		},
		{
			Name: "what-if", Aliases: []string{"whatif"}, Usage: "what-if <id>", Group: "DAG & Dependency",
			Summary: "Simulate completing, claiming, or blocking a task",
			Flags: []flagSpec{
				{"--done", flagBoolean, "Simulate completion"},
				{"--claim", flagBoolean, "Simulate claiming"},
				{"--block", flagBoolean, "Simulate blocking"},
				jsonFlag,
			},
			Run: cmdWhatIf,
		},
		{
			Name: "graph", Usage: "graph", Group: "DAG & Dependency",
			Summary: "Visualize the DAG topology",
			Flags: []flagSpec{
				{"--dot", flagBoolean, "Emit Graphviz DOT"},
				jsonFlag,
			},
			Run: cmdGraph,
		},

		// Reactivity.
		{
			Name: "watch", Usage: "watch [<id>]", Group: "Reactivity",
			Summary: "Live-refresh the task dashboard",
			Flags: []flagSpec{
				{"--poll", flagValue, "Refresh interval (e.g. 500ms, 2s)"},
				{"--plain", flagBoolean, "Disable alt-screen and ANSI chrome (for pipes/tests)"},
			},
			Run: cmdWatch,
		},
		{
			Name: "events", Usage: "events [<since>]", Group: "Reactivity",
			Summary: "Show the event log",
			Flags: []flagSpec{
				{"--limit", flagValue, "Maximum events per page"},
				jsonFlag,
			},
			Run: cmdEvents,
		},

		// Integrations.
		{
			Name: "mcp", Usage: "mcp --stdio", Group: "Integrations",
			Summary:                    "Start the Model Context Protocol server",
			AllowsUninitializedProject: true,
			Flags:                      []flagSpec{{"--stdio", flagBoolean, "Serve over stdio"}},
			Run:                        cmdMCP,
		},
		{
			Name: "serve", Usage: "serve --stdio", Group: "Integrations",
			Summary:                    "Start the native JSON-RPC server",
			AllowsUninitializedProject: true,
			Flags:                      []flagSpec{{"--stdio", flagBoolean, "Serve over stdio"}},
			Run:                        cmdServe,
		},
		{
			Name: "integrate", Usage: "integrate [target]", Group: "Integrations",
			Summary:                    "Install Codex or Claude Code project integration",
			AllowsUninitializedProject: true,
			Flags: []flagSpec{
				{"--command", flagValue, "Path to the todo binary to register"},
				{"--force", flagBoolean, "Overwrite existing configuration"},
				{"--check", flagBoolean, "Report integration status without writing"},
				{"--live", flagBoolean, "Perform a live MCP handshake check"},
			},
			Run: cmdIntegrate,
		},
		{
			Name: "conformance", Usage: "conformance", Group: "Integrations",
			Summary:                    "Probe real-agent hosts; --run performs the opt-in suite",
			AllowsUninitializedProject: true,
			Flags: []flagSpec{
				{"--host", flagValue, "Host to evaluate"},
				{"--timeout", flagValue, "Per-scenario timeout"},
				{"--model", flagValue, "Explicit model ID"},
				{"--suite", flagValue, "Suite version"},
				{"--run", flagBoolean, "Execute the suite; may consume paid usage"},
				jsonFlag,
				{"--include-events", flagBoolean, "Include raw host events"},
				{"--keep-workspace", flagBoolean, "Retain disposable workspaces"},
			},
			Run: cmdConformance,
		},

		// Project.
		{
			Name: "config", Usage: "config [key=value]", Group: "Project",
			Summary: "View or set project configuration",
			Run:     cmdConfig,
		},
		{
			Name: "export", Usage: "export", Group: "Project",
			Summary: "Export tasks",
			Flags:   []flagSpec{{"--markdown", flagBoolean, "Emit Markdown"}},
			Run:     cmdExport,
		},
		{
			Name: "prune", Usage: "prune", Group: "Project",
			Summary: "Remove all completed tasks",
			Flags:   mutationFlags(),
			Run:     cmdPrune,
		},
		{
			Name: "backup", Usage: "backup", Group: "Project",
			Summary: "Snapshot the task store",
			Flags:   []flagSpec{{"--output", flagValue, "Destination path"}},
			Run:     cmdBackup,
		},
		{
			Name: "restore", Usage: "restore <path>", Group: "Project",
			Summary: "Restore tasks from a backup",
			Run:     cmdRestore,
		},
		{
			Name: "compact", Usage: "compact", Group: "Project",
			Summary: "Apply the audit and receipt retention policy",
			Flags: []flagSpec{
				{"--keep-events", flagValue, "Events to retain"},
				{"--receipts-before", flagValue, "Drop receipts older than this"},
				{"--dry-run", flagBoolean, "Report without writing"},
				jsonFlag,
			},
			Run: cmdCompact,
		},
		{
			Name: "doctor", Usage: "doctor", Group: "Project",
			Summary: "Diagnose project health",
			Flags:   []flagSpec{{"--fix", flagBoolean, "Repair what can be repaired"}},
			Run:     cmdDoctor,
		},
		{
			Name: "link", Usage: "link <alias> <path>", Group: "Project",
			Summary: "Register a linked repository",
			Run:     cmdLink,
		},
		{
			Name: "unlink", Usage: "unlink <alias>", Group: "Project",
			Summary: "Remove a linked repository alias",
			Run:     cmdUnlink,
		},
	}
}

// The registry is built once. Rebuilding it per lookup would hand out fresh
// pointers each time, so a command and its alias would not be the same value.
var (
	registryOnce  sync.Once
	registry      []*command
	registryIndex map[string]*command
)

func loadRegistry() {
	registryOnce.Do(func() {
		registry = newCommandRegistry()
		registryIndex = make(map[string]*command, len(registry))
		for _, cmd := range registry {
			registryIndex[cmd.Name] = cmd
			for _, alias := range cmd.Aliases {
				registryIndex[alias] = cmd
			}
		}
	})
}

// commandRegistry returns the resolved command set.
func commandRegistry() []*command {
	loadRegistry()
	return registry
}

// lookupCommand resolves a name or alias.
func lookupCommand(name string) (*command, bool) {
	loadRegistry()
	cmd, ok := registryIndex[name]
	return cmd, ok
}

// validateArgs rejects unknown flags and flags missing a required value.
//
// A value flag followed by another flag is treated as missing its value. That
// is deliberate: `--as --json` is far more likely to be a forgotten actor than
// an actor literally named "--json".
func (c *command) validateArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flag, declared := c.accepts(arg)
		if !declared {
			return fmt.Errorf("unknown option %s for %s", arg, c.Name)
		}
		if !flag.Kind.takesValue() {
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return fmt.Errorf("%s requires a value", arg)
		}
		i++
	}
	return nil
}

// renderUsage builds the help listing from the registry, so a command can
// never exist without appearing in help or appear in help without existing.
func renderUsage() string {
	var out strings.Builder
	fmt.Fprintf(&out, "todo v%s - Distributed Multi-Agent Task Orchestration\n\n", version)
	out.WriteString("Usage: todo <command> [options]\n")

	grouped := make(map[string][]*command)
	for _, cmd := range commandRegistry() {
		if cmd.Hidden {
			continue
		}
		grouped[cmd.Group] = append(grouped[cmd.Group], cmd)
	}

	width := 0
	for _, cmds := range grouped {
		for _, cmd := range cmds {
			if len(cmd.Usage) > width {
				width = len(cmd.Usage)
			}
		}
	}

	for _, group := range helpGroups {
		cmds := grouped[group]
		if len(cmds) == 0 {
			continue
		}
		sort.SliceStable(cmds, func(i, j int) bool { return false })
		fmt.Fprintf(&out, "\n%s:\n", group)
		for _, cmd := range cmds {
			fmt.Fprintf(&out, "  %-*s  %s\n", width, cmd.Usage, cmd.Summary)
		}
	}

	fmt.Fprintf(&out, "\n%s:\n", "Help")
	fmt.Fprintf(&out, "  %-*s  %s\n", width, "help", "Show this help")
	fmt.Fprintf(&out, "  %-*s  %s\n", width, "--version", "Show the installed version")
	out.WriteString(`
Examples:
  todo add "Implement auth"
  todo add "Fix bug" --after 1
  todo claim 1 --as agent-alpha
  todo decompose 1 --into '{"subtasks":[{"title":"Sub1"}]}'
  todo done 1
  todo status --json

Run 'todo help <command>' for the flags a command accepts.
`)
	return out.String()
}

// renderCommandHelp describes one command and its flags.
func renderCommandHelp(cmd *command) string {
	var out strings.Builder
	fmt.Fprintf(&out, "todo %s\n\n%s\n", cmd.Usage, cmd.Summary)
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(&out, "\nAliases: %s\n", strings.Join(cmd.Aliases, ", "))
	}
	if len(cmd.Flags) == 0 {
		return out.String()
	}

	width := 0
	for _, flag := range cmd.Flags {
		name := flagDisplay(flag)
		if len(name) > width {
			width = len(name)
		}
	}
	out.WriteString("\nOptions:\n")
	for _, flag := range cmd.Flags {
		fmt.Fprintf(&out, "  %-*s  %s\n", width, flagDisplay(flag), flag.Help)
	}
	return out.String()
}

func flagDisplay(flag flagSpec) string {
	switch flag.Kind {
	case flagValue:
		return flag.Name + " <value>"
	case flagRepeatable:
		return flag.Name + " <value>"
	default:
		return flag.Name
	}
}
