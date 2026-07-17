---
name: terminal-todo
description: Coordinate and persist project work with the terminal-todo CLI. Use when an agent needs to plan or decompose a durable objective, resume work from an existing .terminal-todo graph, distribute ready tasks across multiple workers, claim work without races, maintain ownership leases, record findings for handoff, or complete, block, and release tasks. Also use when the user explicitly mentions terminal-todo, shared agent tasks, durable task state, task DAGs, or multi-agent coordination.
---

# Terminal Todo

Use `terminal-todo` as the source of truth for project work that must survive
the current conversation or be shared with another worker. Keep ordinary
one-step work out of the task graph unless the user asks to track it.

## Establish the session

1. Run `command -v todo` before relying on the CLI. If it is unavailable,
   stop and explain that terminal-todo must be installed.
2. Work from the project root. Check for `.terminal-todo/tasks.bin`.
3. Run `todo init` only when durable coordination is intended and the project
   is not initialized. Do not initialize for a read-only question.
4. Choose one unique actor name for this agent session and reuse it in every
   ownership-sensitive command. Prefer a user-provided identity; otherwise use
   a recognizable name with a short random suffix, such as `codex-a1b2c3d4`.
5. Inspect existing state before adding or acquiring anything:

```bash
todo status --json
todo events --json
```

If resuming a known actor, also run:

```bash
todo my --as <actor> --json
```

Never recreate work already represented in the graph.

## Plan durable objectives

Create tasks with actionable titles, capability requirements, and explicit
dependencies:

```bash
todo add "Implement token validation" --priority 0.8 --caps go,security
todo add "Add authentication tests" --after 1 --caps go,testing
```

Use `decompose` when an objective is too broad or new findings change the plan:

```bash
todo decompose <parent-id> --as <actor> --into '{
  "subtasks": [
    {"title": "Reproduce the failure", "caps": ["testing"]},
    {"title": "Implement the fix", "caps": ["go"]}
  ]
}'
```

Decomposition releases the parent lease and makes its new subtasks
prerequisites. Reacquire a ready subtask rather than continuing to act as the
parent owner.

## Acquire work safely

Register capabilities when they are known:

```bash
todo agent-card --as <actor> --caps go,testing --max-load 1
```

For autonomous allocation, always use the atomic allocator:

```bash
todo acquire \
  --as <actor> \
  --request-id <unique-request-id> \
  --capabilities go,testing \
  --wait 30s \
  --json
```

Do not implement allocation as `todo next` followed by `todo claim`; another
worker can win between those commands.

Generate a new opaque request ID for each new allocation attempt. If the
result may have been lost, retry the identical request ID with identical actor,
TTL mode, and capability mode. Never reuse a request ID with changed
parameters.

Treat allocator errors as control flow:

- `NO_WORK`: wait, inspect blockers, or finish the session.
- `AGENT_AT_CAPACITY`: finish or release currently owned work.
- `IDEMPOTENCY_CONFLICT`: generate a new request ID for the new parameters.

Use `claim` only when the user or a manager explicitly selected a task ID.

## Maintain and communicate ownership

Choose a TTL longer than the expected interval between checkpoints. Renew an
active lease before it expires:

```bash
todo heartbeat <id> --as <actor> --ttl 30m --json
```

Heartbeat before and after long-running commands or extended reasoning. A
stale lease cannot be revived; if renewal returns `LEASE_NOT_ACTIVE`, inspect
the graph and reacquire instead of assuming ownership.

Record durable context as soon as it becomes useful to another session:

```bash
todo update <id> --as <actor> \
  --set finding="rename invalidates the original lock inode" \
  --set file=store/store.go

todo log <id> --as <actor> --msg "Race reproduced under concurrent writers"
```

Do not store secrets, credentials, tokens, or unnecessary personal data in
task metadata.

## Finish or hand off

Verify the work before completion, then use the same actor identity:

```bash
todo done <id> --as <actor> --json
```

If the work should return to the queue:

```bash
todo release <id> --as <actor> --error "concise retry context" --json
```

If progress requires an external condition that no worker can currently
resolve:

```bash
todo block <id> --as <actor> --reason "waiting for API credentials"
```

Prefer `release` for retryable execution failures and `block` for durable
external blockers. Never leave owned work silently in progress when ending a
session.

## Preserve invariants

- Treat `.terminal-todo/` as application state; never edit its files directly.
- Keep one stable actor identity per worker session.
- Mutate claimed tasks only as their current owner.
- Use dependencies for ordering and manual blocks for external conditions.
- Record findings before context can be lost.
- Use JSON output for machine decisions; do not scrape human-readable tables.
- Inspect `todo status --json` after ambiguous failures before retrying a
  mutation.
