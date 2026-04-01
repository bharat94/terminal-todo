# Problem Statement

## The Problem

AI agents lose task state when context resets. Current todo implementations are flat lists without dependency tracking. There is no standardized way for multiple agents or terminal sessions to share project progress.

### Why It Matters

1. **Context Loss** — Agents can't retain what previous sessions completed
2. **No Dependency Tracking** — Flat lists don't capture "blocked by" or "depends on" relationships
3. **No Shared State** — Different agents (Claude, Cursor, etc.) can't see each other's progress
4. **Repetitive Work** — Without persistence, agents redo work that's already done

## Current Alternatives

- **planning-with-files** — Handles the "what" (plan/findings/progress files) but not task dependencies
- **taskwarrior** — Full-featured but human-centric, not designed for agent CLI usage
- **TodoWrite tool** — In-context only, lost on reset

## Target Users

- AI agents working on code projects
- Developers using multiple AI tools on the same codebase
- Teams wanting shared project memory across sessions

## Success Criteria

- Any AI agent can read/write tasks via CLI
- Survives context resets (persisted to disk)
- Supports DAG operations: add, depends, status, done, next
- Binary storage under 1KB for typical projects
- Zero configuration, project-scoped only
