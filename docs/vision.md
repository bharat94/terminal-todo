# Vision

## Core Vision

A standalone CLI that manages a DAG of tasks with dependencies, persisted to disk for cross-session and cross-agent sharing. It provides a shared project memory layer that any AI agent can interact with via command line.

## Key Principles

1. **Binary-first storage** — Compact, smallest on-disk footprint
2. **Optional JSON export** — Human-readable format on demand
3. **Project-scoped** — Lives in `.terminal-todo/` in project root
4. **DAG semantics** — Tasks have explicit dependencies with cycle detection
5. **Agent-first** — Every interaction is CLI-invokable with JSON output
6. **Minimalist** — No fluff, just task tracking with dependencies

## Design Decisions

### Why Binary Storage?

- Smallest disk footprint
- Fast read/write for CLI operations
- Forces agents through CLI (consistent interface)
- Optional JSON export for debugging/inspection

### Why Project-Scoped?

- Each project has independent task state
- Git-ignorable (add `.terminal-todo/` to `.gitignore`)
- No global state pollution

### Why DAG over Flat List?

- Captures natural task relationships (subtasks, prerequisites)
- Shows "blocked" vs "ready-to-work" states
- Enables parallel execution planning
- More accurately reflects how agents break down work

## What It Is NOT

- Not a full project management tool (no time tracking, no extensive tagging)
- Not a human-only tool (designed for AI agent CLI usage)
- Not tied to one AI platform (works with any CLI-capable agent)
- Not a replacement for planning-with-files (complementary)

## Success Criteria

- Any AI agent can read/write via CLI invocation
- Survives context resets (persisted to project directory)
- Supports: add, depends, done, status, next, rm, cat, export, prune
- Binary storage compact (<1KB typical project)
- Cycle detection on dependency add
- JSON output flag for all read commands

## Integration Points

### With AI Agents

```bash
# Get next ready task
NEXT=$(todo next --json)

# Add task with dependency
todo add "implement auth" --after 1

# Mark complete
todo done 2
```

### With planning-with-files

- terminal-todo tracks **tasks and dependencies** (the "what" and "in what order")
- planning-with-files tracks **plan, findings, progress** (the "how" and "learnings")
- Use both in parallel: terminal-todo for task queue, planning-with-files for execution details

## Roadmap

| Phase | Scope |
|-------|-------|
| 1 | Binary storage, init, add, done, status |
| 2 | Dependencies, cycle detection, next, prune |
| 3 | Export (JSON/Markdown), config, dependents |
| 4 | Tests, edge cases, polish |
