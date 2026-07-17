# Problem Statement

## The Problem

Agent-native goals and memories help one agent retain intent. They do not form
a shared execution control plane for multiple humans, agents, scripts,
sessions, worktrees, and repositories.

Once work crosses that boundary, coordination becomes implicit:

1. **No conflict-free allocation** — Independent workers can select the same
   task or make incompatible changes without knowing about each other.
2. **No dependency model** — Flat lists and chat plans do not reliably express
   what is ready, blocked, or downstream.
3. **No portable ownership** — A worker can disappear while work remains
   ambiguously in progress, or another worker can unknowingly duplicate it.
4. **Opaque handoffs** — Findings, retries, and blockers are scattered across
   private contexts and vendor-specific transcripts.
5. **No cross-runtime source of truth** — Claude, Codex, humans, and scripts do
   not share a neutral operational record.
6. **No user-controlled audit history** — Coordination state is often trapped
   inside a vendor's thread, memory, or hosted task system.

> Goals tell an agent what outcome to pursue. `terminal-todo` tells a fleet who
> should do what, in what order, and what has already happened.

## Why It Matters

A high-level goal can own the outcome while `terminal-todo` operationalizes it
as an explicit execution graph:

- DAG dependencies determine ready work.
- Atomic acquisition prevents duplicate assignment.
- Ownership leases, heartbeats, retries, and expiration support recovery.
- Structured metadata and events make handoffs inspectable.
- Project-local state remains durable, portable, and controlled by the user.
- The execution record survives context resets, process exits, and agent
  restarts.
- URI dependencies coordinate work across repositories.

For one agent in one thread, a goal or ordinary plan may be sufficient.
`terminal-todo` becomes useful when work must be shared across workers, tools,
sessions, or repositories.

## Current Alternatives

- **Agent goals and memories** — Preserve individual intent and context, but do
  not coordinate a heterogeneous fleet.
- **Planning files** — Capture prose, but do not provide atomic ownership,
  leases, or executable dependency semantics.
- **Issue trackers** — Provide team visibility, but are networked,
  human-oriented, and rarely safe for inference-time acquisition loops.
- **Taskwarrior and todo tools** — Track work, but are not designed as
  machine-stable multi-agent coordination protocols.
- **In-context task tools** — Are convenient within one run, but are not a
  portable cross-agent source of truth.

## Target Users

- Developers coordinating multiple coding agents
- Teams mixing Claude, Codex, other vendors, scripts, and humans
- Agent supervisors distributing work across isolated worktrees
- Multi-repository projects with explicit delivery dependencies
- Users who want coordination state independent of an agent provider

## Success Criteria

- Any authorized participant can read and mutate the same task graph.
- Ready work is derived deterministically from DAG and blocker state.
- Concurrent workers cannot acquire the same task.
- Crashed workers relinquish ownership through lease expiration.
- Another worker can resume from structured findings and retry history.
- Cross-repository dependencies remain observable when checkouts are missing.
- CLI, JSON, and integration protocols preserve stable machine semantics.
- The complete coordination record remains portable and user-controlled.
