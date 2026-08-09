---
name: terminal-todo
description: Coordinate and persist project work with terminal-todo through its MCP tools or CLI. Use when an agent needs to plan or decompose a durable objective, resume work from an existing .terminal-todo graph, distribute ready tasks across multiple workers, claim work without races, maintain ownership leases, record findings for handoff, or complete, block, and release tasks. Also use when the user explicitly mentions terminal-todo, shared agent tasks, durable task state, task DAGs, or multi-agent coordination.
---

# Terminal Todo

Use terminal-todo as the source of truth for project work that must survive
the current conversation or be shared with another worker. Keep ordinary
one-step work out of the task graph unless the user asks to track it.

## Keep coordination quiet

Treat routine coordination as background bookkeeping:

- Prefer the `terminal_todo_*` MCP tools when available. Use the CLI as a
  fallback; do not launch the MCP server manually.
- Do not echo raw commands, JSON payloads, actor IDs, request IDs, heartbeats,
  or routine tool results to the user.
- Do not narrate every coordination call. Mention only outcomes that affect
  the user's understanding: selected work, a changed plan, a blocker, a
  meaningful handoff, or completion.
- Keep progress updates about product work, not task-manager mechanics.
- Read decisions from structured MCP content. The short text block is for a
  compact trace, not the complete result.
- Request compact mutation receipts for routine acknowledgements. Fetch full
  task detail only when the next decision needs it.

## Establish the session

1. Work from the project root. If MCP tools are available, call
   `terminal_todo_ping`; otherwise run `command -v todo` before relying on the
   CLI. If neither transport is available, explain that terminal-todo must be
   installed.
2. Initialize only when durable coordination is intended and the project is
   not initialized. Use `terminal_todo_init` or `todo init`. Do not initialize
   for a read-only question.
3. Choose one unique actor name for this agent session and reuse it in every
   ownership-sensitive command. Prefer a user-provided identity; otherwise use
   a recognizable name with a short random suffix, such as `codex-a1b2c3d4`.
4. When the project is initialized, call `terminal_todo_bootstrap` with that
   actor. It returns a bounded brief covering the objective, progress, owned
   work, compatible ready work, blockers, capability demand, and recent
   events. The brief does not register the actor or acquire work, though
   inspecting coordination state may persist recovery of expired leases. On
   the CLI fallback, run:

```bash
todo bootstrap --as <actor> --capabilities go,testing --json
```

If bootstrap is unavailable in an older installation, fall back to
`terminal_todo_status` plus `terminal_todo_events`; on the CLI use:

```bash
todo status --json
todo events --limit 20 --json
todo my --as <actor> --json
```

Over MCP, call `terminal_todo_events` with `{"page":true,"limit":20}` and
continue from `cursor.next_since` while `cursor.has_more` is true. If
`cursor.history_gap` is true, retained event history is incomplete; resync
from current status instead of assuming the missing prefix.

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
}' --receipt
```

Decomposition releases the parent lease and makes its new subtasks
prerequisites. One call accepts at most 20 children. Reacquire a ready subtask
rather than continuing to act as the parent owner. Over MCP, request
`receipt:true`.

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

Over MCP, call `terminal_todo_acquire` with the same actor, request ID,
capabilities, and TTL fields.

Keep acquisition's full result: the selected task content is the input to the
next work decision. Compact receipts are for later lifecycle mutations, not
the initial allocation.

Do not implement allocation as `todo next` followed by `todo claim`; another
worker can win between those commands.

Generate a new opaque request ID for each new allocation attempt. If the
result may have been lost, retry the identical request ID with identical actor,
TTL mode, and capability mode. Never reuse a request ID with changed
parameters.

Treat allocator errors as control flow:

- `NO_WORK`: inspect structured `data.reason` and its blockers or missing
  capabilities; wait only when the reported state can change.
- `AGENT_AT_CAPACITY`: inspect current/max load, then finish or release
  currently owned work.
- `IDEMPOTENCY_CONFLICT`: generate a new request ID for the new parameters.

Use `claim` only when the user or a manager explicitly selected a task ID.

## Maintain and communicate ownership

Choose a TTL longer than the expected interval between checkpoints. Renew an
active lease before it expires:

```bash
todo heartbeat <id> --as <actor> --ttl 30m --receipt
```

Prefer the `terminal_todo_heartbeat` MCP tool over the CLI and send
`receipt:true`.

Heartbeat before and after long-running commands or extended reasoning. A
stale lease cannot be revived; if renewal returns `LEASE_NOT_ACTIVE`, inspect
the graph and reacquire instead of assuming ownership.

Record durable context as soon as it becomes useful to another session:

```bash
todo update <id> --as <actor> \
  --set finding="rename invalidates the original lock inode" \
  --set file=store/store.go \
  --receipt

todo log <id> --as <actor> --msg "Race reproduced under concurrent writers" \
  --receipt
```

Send `receipt:true` for the corresponding MCP update and log calls. Receipts
return bounded acknowledgement fields and never echo large log messages or
task metadata. Use the receipt's `detail_follow_up` only when full detail is
needed.

Do not store secrets, credentials, tokens, or unnecessary personal data in
task metadata.

## Finish or hand off

Verify the work before completion, then use the same actor identity:

```bash
todo done <id> --as <actor> --receipt
```

If the work should return to the queue:

```bash
todo release <id> --as <actor> --error "concise retry context" --receipt
```

If progress requires an external condition that no worker can currently
resolve:

```bash
todo block <id> --as <actor> \
  --reason "waiting for API credentials" --receipt
```

Prefer `release` for retryable execution failures and `block` for durable
external blockers. Blocking releases the active lease so another worker can
unblock and reacquire the task later. Never leave owned work silently in
progress when ending a session.

Over MCP, send `receipt:true` for completion, release, block, and other routine
mutations. Compact receipts acknowledge what changed; they do not replace
detail reads.

## Preserve invariants

- Treat `.terminal-todo/` as application state; never edit its files directly.
- Keep one stable actor identity per worker session.
- Mutate claimed tasks only as their current owner.
- Use dependencies for ordering and manual blocks for external conditions.
- Record findings before context can be lost.
- Use MCP structured content or CLI JSON for machine decisions; do not scrape
  human-readable tables.
- Treat `status`, `cat`, `lineage`, `export`, legacy unpaged `events`, and
  acquisition results as potentially large reads. Request them deliberately,
  and prefer bootstrap, event pages, or mutation receipts when those bounded
  views answer the decision.
- Inspect `todo status --json` after ambiguous failures before retrying a
  mutation.
