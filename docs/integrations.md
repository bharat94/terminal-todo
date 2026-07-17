# Agent integrations

terminal-todo separates coordination mechanics from agent behavior:

- the CLI and MCP server enforce DAG, lease, ownership, and durability rules;
- the bundled skill teaches an agent how to use those rules as a reliable
  worker;
- client configuration makes the MCP tools available without runtime-specific
  glue code.

Goals tell an agent what outcome to pursue. terminal-todo tells a fleet who
should do what, in what order, and what has already happened.

## Project installation

From the project you want agents to coordinate:

```bash
todo init
todo integrate
todo integrate --check
```

The default target is `all`. Use `codex` or `claude` to install one runtime:

```bash
todo integrate codex
todo integrate claude
```

The installer embeds the version-matched canonical skill in the binary. It
does not need the terminal-todo source checkout at runtime.

## Generated files

Codex:

```text
.agents/skills/terminal-todo/SKILL.md
.agents/skills/terminal-todo/agents/openai.yaml
.codex/config.toml
```

Claude Code:

```text
.claude/skills/terminal-todo/SKILL.md
.claude/skills/terminal-todo/agents/openai.yaml
.mcp.json
```

The MCP entry launches:

```bash
todo mcp --stdio
```

Use `--command` when `todo` is not available on the client's inherited
`PATH`:

```bash
todo integrate --command "$HOME/bin/todo"
```

For a shared repository, prefer a command name or a team-standard stable path.
Avoid committing a machine-specific absolute path.

## Safety and repeatability

`todo integrate` is idempotent. It computes every output before writing any
file, preserves unrelated keys and sections in existing client configuration,
and writes replacements through a temporary file.

If an existing terminal-todo MCP entry or bundled skill differs, installation
stops without modifying files. Review the local customization before using:

```bash
todo integrate --force
```

`--force` replaces only terminal-todo's skill files and MCP entry. It does not
discard unrelated MCP servers or client preferences.

Use the read-only check in repository validation:

```bash
todo integrate --check
```

It exits non-zero when an expected file is missing or does not match the
version bundled with the binary.

## Runtime lifecycle

Start Codex or Claude Code from the initialized project directory. The MCP
host launches terminal-todo over stdio, completes MCP initialization, and
discovers the curated tools. A worker should then:

1. inspect `terminal_todo_status`;
2. identify itself with a stable actor name;
3. call `terminal_todo_acquire` with a unique request ID;
4. heartbeat before the lease expires;
5. record findings with `terminal_todo_update` or `terminal_todo_log`;
6. complete, block, decompose, or release the task explicitly.

The underlying `.terminal-todo/` directory remains user-controlled portable
state. It can stay local, be backed up, or be shared through storage chosen by
the user.

Routine coordination should remain background bookkeeping. MCP tool results
put a concise trace in `content` and the complete result in
`structuredContent`; the bundled skill tells workers to report meaningful
work outcomes and blockers instead of narrating every task-manager call. Host
UIs still control whether tool calls themselves are visible or collapsed.

The design and remaining priorities come from a full production-readiness
dogfood run. See the [dogfooding retrospective](dogfooding-retrospective.md).

## Manual registration

The installer writes project-scoped configuration. For user-scoped
registration, client CLIs can configure the same server:

```bash
codex mcp add terminal-todo -- todo mcp --stdio
claude mcp add --transport stdio --scope user terminal-todo -- todo mcp --stdio
```

The canonical skill may likewise be copied to `$HOME/.agents/skills/` or
`$HOME/.claude/skills/`.

## Troubleshooting

If tools report that the project is not initialized, either start the client
from the intended repository, run `todo init` there, or call
`terminal_todo_init`.

If the MCP process cannot start, verify:

```bash
command -v todo
todo --version
todo mcp --stdio
```

The last command waits for MCP messages on stdin; exiting on EOF is normal.
Run `todo integrate --check` after upgrading the binary so skill and client
configuration drift is visible.
