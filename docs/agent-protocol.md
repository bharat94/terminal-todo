# terminal-todo Agent Protocol

**Protocol Version:** `1` (stable)
**Last Updated:** 2026-07-11

This document defines the stable CLI/JSON interface for agent-to-agent and
agent-to-CLI communication via `terminal-todo`. Every JSON response includes
`"schema_version": "1"` at the top level.

## Table of Contents

1. [Task Representation](#1-task-representation)
2. [Envelopes](#2-envelopes)
3. [Error Format](#3-error-format)
4. [Query Commands (--json)](#4-query-commands---json)
5. [Mutation Commands](#5-mutation-commands)
6. [Agent Cards & Capabilities](#6-agent-cards--capabilities)
7. [Events & Logs](#7-events--logs)
8. [Graph & Lineage](#8-graph--lineage)
9. [JSON-RPC Transport (serve --stdio)](#9-json-rpc-transport-serve---stdio)
10. [Timestamp Format](#10-timestamp-format)
11. [Versioning Policy](#11-versioning-policy)

---

## 1. Task Representation

Every task in the system has the following JSON shape. Timestamps are
RFC3339Nano strings. See section 10 for details.

```json
{
  "id": 101,
  "title": "Implement auth middleware",
  "status": "in_progress",
  "depends": ["todo://local/100"],
  "created": "2026-04-04T14:00:00Z",
  "completed": null,
  "metadata": {
    "capabilities": ["go", "security"],
    "owner": "agent-alpha-4",
    "lease_expires": "2026-04-04T14:30:00Z",
    "priority": 0.85,
    "lineage": "todo://local/50",
    "tags": ["backend"],
    "retry_count": 0,
    "last_error": "",
    "log": [
      {
        "timestamp": "2026-04-04T14:05:00Z",
        "agent": "agent-alpha-4",
        "message": "Started work on auth middleware"
      }
    ],
    "extra": {
      "github_issue": "42",
      "finding": "Rate limiter needs config"
    }
  }
}
```

**Status values:** `"pending"`, `"in_progress"`, `"completed"`, `"blocked"`

**Dependency URIs:** `todo://local/<id>` for local tasks, `todo://<alias>/<id>`
for tasks in linked repositories.

**Metadata fields:**
- `capabilities`: what the task requires (matching via `todo next --capabilities`)
- `owner`: which agent holds the lease (empty if unclaimed)
- `lease_expires`: when the lease expires (null if unclaimed)
- `priority`: 0.0–1.0 float, higher = more important
- `lineage`: parent task URI if this is a subtask
- `tags`: arbitrary classification tags
- `retry_count`: how many times the task has been released with an error
- `last_error`: the most recent error message (empty if none)
- `block_reason`: structured reason for a manual block (empty unless blocked)
- `log`: ordered audit trail of agent actions
- `extra`: free-form key-value store for context sharing between agents

---

## 2. Envelopes

All JSON responses wrap data in a versioned envelope. The envelope type
depends on the command.

### Task Envelope (single task)

Used by: `cat --json`, `update --json`

```json
{
  "schema_version": "1",
  "task": { ... }
}
```

### Tasks Envelope (multiple tasks)

Used by: `status --json`, `search --json`, `my --json`, `export`

```json
{
  "schema_version": "1",
  "tasks": [ ... ]
}
```

### Next Envelope

Used by: `next --json`

```json
{
  "schema_version": "1",
  "available_tasks": [
    {
      "id": 101,
      "title": "Implement auth middleware",
      "priority": 0.85,
      "capabilities": ["go"],
      "reason": "ready: all dependencies completed"
    }
  ],
  "blocked_summary": {
    "count": 5,
    "primary_blockers": ["todo://local/99"]
  }
}
```

### Projects Envelope

Used by: `status --all --json`

```json
{
  "schema_version": "1",
  "projects": [
    {
      "alias": "local",
      "path": ".",
      "available": true,
      "tasks": [ ... ]
    },
    {
      "alias": "infra",
      "path": "../infra",
      "available": false,
      "error": "not found"
    }
  ]
}
```

### Lineage Envelope

Used by: `lineage --json`

```json
{
  "schema_version": "1",
  "root": { ... },
  "descendants": [ ... ],
  "progress": {
    "total": 10,
    "pending": 3,
    "in_progress": 2,
    "completed": 4,
    "blocked": 1,
    "percent_complete": 40.0
  }
}
```

### Depends Envelope

Used by: `depends --json`

```json
{
  "schema_version": "1",
  "task_id": 101,
  "task_title": "Implement auth middleware",
  "depends": [
    {"id": 100, "title": "Design schema", "uri": "todo://local/100"}
  ]
}
```

Remote dependencies that cannot be resolved locally appear as:
```json
{"uri": "todo://infra/5"}
```
with no `id` or `title` fields.

### Dependents Envelope

Used by: `dependents --json`

```json
{
  "schema_version": "1",
  "task_id": 101,
  "task_title": "Implement auth middleware",
  "dependents": [
    {"id": 102, "title": "Add login page"}
  ]
}
```

### What-If Envelope

Used by: `what-if --json`

```json
{
  "schema_version": "1",
  "task_id": 101,
  "title": "Implement auth middleware",
  "if_done": {
    "would_unblock": [
      {"id": 102, "title": "Add login page"}
    ],
    "still_blocked": 2
  },
  "if_blocked": {
    "would_block": 4
  }
}
```

The `if_done` section is present when no filter is specified or `--done` is
used. The `if_blocked` section is present when no filter is specified or
`--block` is used.

### Events Envelope

Used by: `events --json`

```json
{
  "schema_version": "1",
  "events": [
    {
      "id": 1,
      "timestamp": "2026-04-04T14:00:00Z",
      "type": "created",
      "task_id": 101,
      "actor": "",
      "data": {"title": "Implement auth middleware"}
    }
  ]
}
```

Event types: `created`, `completed`, `claimed`, `released`, `lease_expired`,
`blocked`, `unblocked`, `updated`, `decomposed`, `removed`, `dep_added`,
`dep_removed`.

### Graph Envelope

Used by: `graph --json`

```json
{
  "schema_version": "1",
  "nodes": [
    {"id": 100, "title": "Design schema", "status": "completed"},
    {"id": 101, "title": "Implement auth", "status": "in_progress"}
  ],
  "edges": [
    {"from": 100, "to": 101}
  ]
}
```

### Agent Card Envelope

Used by: `agent-card --json`

Single agent:
```json
{
  "schema_version": "1",
  "agent": {
    "name": "agent-alpha",
    "capabilities": ["go", "docker"],
    "description": "Handles Go backend tasks",
    "max_load": 5,
    "current_load": 3,
    "created_at": "2026-07-11T12:00:00Z",
    "last_seen": "2026-07-11T14:30:00Z"
  }
}
```

All agents:
```json
{
  "schema_version": "1",
  "agents": {
    "agent-alpha": { ... },
    "agent-beta": { ... }
  }
}
```

`current_load` is computed dynamically from the task store (tasks claimed by
this agent with active leases).

### Capabilities Envelope

Used by: `caps --json`

```json
{
  "schema_version": "1",
  "required_capabilities": [
    {"capability": "go", "task_count": 5},
    {"capability": "docker", "task_count": 3}
  ],
  "total_unclaimed_tasks": 12,
  "tasks_without_caps": 2,
  "registered_agents": 3,
  "unmatched_capabilities": ["java"]
}
```

`unmatched_capabilities` lists capabilities required by tasks but not provided
by any registered agent.

---

## 3. Error Format

All errors output to stderr when `--json` is present anywhere in CLI args.
The error envelope is:

```json
{
  "schema_version": "1",
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "task 999 not found",
    "details": ""
  }
}
```

**Error codes:**

| Code | Meaning | Exit Code |
|------|---------|-----------|
| `TASK_NOT_FOUND` | Referenced task does not exist | 1 |
| `INVALID_ARGS` | Bad flags, missing values, wrong format | 1 |
| `NOT_OWNER` | Action requires the claiming agent's identity | 1 |
| `DEPENDENCY_ERROR` | Dependency constraint violated | 1 |
| `ALREADY_CLAIMED` | Task is already claimed by another agent | 5 |
| `CYCLE_DETECTED` | Dependency graph would form a cycle | 4 |
| `NOT_INITIALIZED` | No `.terminal-todo/` found | 2 |
| `STORE_CORRUPTED` | Task store or config file is corrupted | 2 |
| `LOCK_CONTENTION` | Another process holds the file lock | 3 |
| `SCHEMA_VERSION` | Store was created by a newer binary | 2 |

---

## 4. Query Commands (`--json`)

All query commands support `--json` for structured output.

| Command | Envelope | Description |
|---------|----------|-------------|
| `status [--json]` | `tasksEnvelope` | All tasks with optional `--tag`/`--as` filter |
| `status --all --json` | `projectsEnvelope` | Aggregate local + linked repos |
| `cat <id> --json` | `taskEnvelope` | Single task detail |
| `next --json` | `nextEnvelope` | Tasks ready to work, with blocked summary |
| `next --json --capabilities go,test` | `nextEnvelope` | Filtered by agent capabilities |
| `lineage <id> --json` | `lineageEnvelope` | Recursive decomposition with progress |
| `depends <id> --json` | `dependsEnvelope` | Structured dependency list |
| `dependents <id> --json` | `dependentsEnvelope` | Structured dependent list |
| `search <query> --json` | `tasksEnvelope` | Tasks matching title or tags |
| `my --as <owner> --json` | `tasksEnvelope` | Tasks claimed by a specific agent |
| `what-if <id> --json` | `whatifEnvelope` | Simulation of completion/blocking |
| `graph --json` | `graphEnvelope` | DAG topology as nodes + edges |
| `events [<since>] --json` | `eventsEnvelope` | Append-only event log since ID |
| `update <id> --json` | `taskEnvelope` | Updated task after mutation |
| `agent-card --json` | `agentCardEnvelope` | Agent identity and computed load |
| `caps --json` | `capsEnvelope` | Capability demand and gaps |
| `export` | `tasksEnvelope` | Full task export (always JSON, no flag needed) |

---

## 5. Mutation Commands

Mutation commands output human-readable text by default. Core lifecycle
mutations (`add`, `claim`, `release`, and `done`) accept `--json` and return a
versioned task or tasks envelope. All errors are structured via the error
envelope when `--json` is present.

| Command | Args | Effect |
|---------|------|--------|
| `add "<title>" [--priority N] [--caps a,b] [--tag x,y] [--after <id>]` | Create task | Emits `created` event |
| `done <id> [--as <owner>]` | Mark complete | Makes pending dependents eligible and emits `completed`; manual blocks remain until `unblock` |
| `rm <id>` | Remove task | Emits `removed` event |
| `update <id> [--title] [--priority] [--caps] [--set k=v] [--add-dep] [--remove-dep] [--as]` | Modify task | Emits `updated`/`dep_added`/`dep_removed` events |
| `claim <id> --as <owner> [--ttl <duration>]` | Acquire lease | Emits `claimed` event |
| `acquire --as <owner> [--capabilities a,b] [--ttl <duration>]` | Atomically select and claim the highest-priority compatible task | Enforces agent `max_load`, emits `claimed` event |
| `release <id> --as <owner> [--error <msg>]` | Yield lease | Increments retry_count, emits `released` event |
| `block <id> --reason <text> [--as <owner>]` | Mark blocked | Emits `blocked` event |
| `unblock <id> [--as <owner>]` | Mark pending | Emits `unblocked` event |
| `log <id> --msg <text> --as <owner>` | Append to trail | Appends to task.Log |
| `decompose <id> --into <json> [--as]` | Split into subtasks | Emits `decomposed` event |
| `prune` | Remove completed tasks | Rewrites dependency lists |

---

## 6. Agent Cards & Capabilities

Agents register their identity and capabilities so the system can match work
to workers and identify capability gaps.

### Registering an Agent

```bash
todo agent-card --as agent-alpha --caps go,docker --desc "Backend specialist" --max-load 5
```

This creates or updates `.terminal-todo/agents.json`. Registration is also
automatic on first `claim`.

### Querying Agents

```bash
todo agent-card --as agent-alpha           # Single agent with computed load
todo agent-card                            # All known agents
```

### Capability Demand

```bash
todo caps                                  # What the project needs
todo caps --as agent-alpha                 # Exclude tasks already owned by agent
todo caps --all                            # Include linked repositories
```

The `unmatched_capabilities` field in the response tells you which
capabilities no registered agent provides.

---

## 7. Events & Logs

The event log is an append-only sequence of structured events. Events are
numbered sequentially and never modified or deleted.

**Query:**
```bash
todo events              # All events
todo events 42           # Events since ID 42
todo events --json       # Structured output
```

Events are used by the `watch` command internally. Agents can poll for new
events to detect state changes without polling the full task set.

The task-level `log` field is distinct from events — it's an agent-authored
audit trail stored on each individual task.

---

## 8. Graph & Lineage

The DAG visualization commands give agents topological awareness.

### Topology

```bash
todo graph                 # Text overview
todo graph --dot           # Graphviz DOT format
todo graph --json          # Structured nodes + edges
```

### Decomposition Tree

```bash
todo lineage <id>          # Recursive tree
todo lineage <id> --json   # Structured with progress %, root, and descendants
```

---

## 9. JSON-RPC Transport (`serve --stdio`)

`todo serve --stdio` starts a JSON-RPC 2.0 server over stdin/stdout. This
enables MCP (Model Context Protocol) compatible transport without any external
dependency.

### Connection Model

- Persistent session: reads newline-delimited JSON-RPC requests from stdin
- Writes newline-delimited JSON-RPC responses to stdout
- Stderr is reserved for diagnostics
- Process exits when stdin reaches EOF

### Methods

All methods are namespaced `todo.<command>`. Params are named objects.

| Method | Params | Result |
|--------|--------|--------|
| `todo.ping` | `{}` | `{version, project, initialized, capabilities}` |
| `todo.init` | `{}` | `{path}` |
| `todo.add` | `{title, after?, priority?, capabilities?, tags?}` | `{id, title}` |
| `todo.done` | `{ids, actor?}` | `{completed, unblocked}` |
| `todo.status` | `{tag?, actor?, all?}` | `tasksEnvelope` or `projectsEnvelope` |
| `todo.cat` | `{id}` | `protocolTask` |
| `todo.update` | `{id, title?, priority?, capabilities?, actor?, extra?, addDeps?, removeDeps?}` | `protocolTask` |
| `todo.claim` | `{id, actor, ttl?}` | `{id, owner, expires, retryCount, lastError}` |
| `todo.acquire` | `{actor, ttl?, capabilities?}` | Versioned task envelope for the atomically selected task |
| `todo.release` | `{id, actor, error?}` | `{id, status}` |
| `todo.block` | `{id, reason, actor?}` | `{id, status}` |
| `todo.unblock` | `{id, actor?}` | `{id, status}` |
| `todo.next` | `{capabilities?}` | `nextEnvelope` |
| `todo.log` | `{id, message, actor?}` | `{id}` |
| `todo.my` | `{actor}` | `tasksEnvelope` |
| `todo.search` | `{query}` | `tasksEnvelope` |
| `todo.depends` | `{id}` | `dependsEnvelope` |
| `todo.dependents` | `{id}` | `dependentsEnvelope` |
| `todo.decompose` | `{id, subtasks, actor?}` | `{parent, subtasks}` |
| `todo.lineage` | `{id}` | `lineageEnvelope` |
| `todo.events` | `{since?}` | `{events}` |
| `todo.whatIf` | `{id, scenario?}` | `whatifEnvelope` |
| `todo.graph` | `{format?}` | `graphEnvelope` |
| `todo.config.get` | `{key?}` | `{config}` |
| `todo.config.set` | `{key, value}` | `{key, value}` |
| `todo.prune` | `{}` | `{removedCount}` |
| `todo.export` | `{format?}` | `tasksEnvelope` |
| `todo.link` | `{alias, path}` | `{alias, path}` |
| `todo.unlink` | `{alias}` | `{alias}` |
| `todo.backup` | `{output?}` | `{path, taskCount}` |
| `todo.restore` | `{path}` | `{taskCount}` |
| `todo.doctor` | `{fix?}` | diagnostic map |
| `todo.agentCard` | `{actor?, caps?, desc?, maxLoad?}` | `agentCardEnvelope` |
| `todo.caps` | `{actor?, all?}` | `capsEnvelope` |

### JSON-RPC Error Codes

| Todo Error | JSON-RPC Code |
|-----------|---------------|
| Parse error | -32700 |
| Invalid Request | -32600 |
| Method not found | -32601 |
| Invalid params | -32602 |
| Internal error | -32603 |
| `TASK_NOT_FOUND` | -32001 |
| `NOT_INITIALIZED` | -32002 |
| `CYCLE_DETECTED` | -32003 |
| `ALREADY_CLAIMED` | -32004 |
| `NOT_OWNER` | -32005 |
| `DEPENDENCY_ERROR` | -32006 |
| `STORE_CORRUPTED` | -32007 |
| `LOCK_CONTENTION` | -32008 |
| `SCHEMA_VERSION` | -32009 |

---

## 10. Timestamp Format

All timestamps in JSON output use **RFC3339Nano** format:

```
2026-04-04T14:00:00Z
2026-04-04T14:00:00.123Z
```

This applies to:
- `task.created`
- `task.completed`
- `task.metadata.lease_expires`
- `event.timestamp`
- `log[].timestamp` (inside task and event)
- `agent-card` timestamps (`created_at`, `last_seen`)

Internal storage uses Unix epoch milliseconds (`uint64`). The protocol layer
converts to RFC3339Nano on JSON output.

---

## 11. Versioning Policy

The protocol version is a single string field (`"schema_version"`) present in
every JSON response.

- **`"1"`**: Initial stable release. All fields documented above are stable.
  Fields will not be removed; new fields may be added as optional.
- **Future versions**: A new `"schema_version"` value indicates breaking
  changes. Old consumers that don't recognize the version should treat the
  response as opaque.

The protocol version is independent of the internal store schema version
(currently `2`) and the CLI tool version. They evolve independently.

To discover the protocol version at runtime:
- Parse `schema_version` from any JSON response
- `todo agent-card --json` returns a versioned envelope
- `todo serve --stdio`'s `todo.ping` includes version info
