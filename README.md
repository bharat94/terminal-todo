# terminal-todo

**A durable coordination layer for humans and AI agents.**

[![CI](https://github.com/bharat94/terminal-todo/actions/workflows/ci.yml/badge.svg)](https://github.com/bharat94/terminal-todo/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`terminal-todo` turns a project-local task list into a dependency graph that
people, coding agents, and scripts can safely share. It remembers work across
sessions, prevents two agents from taking the same task, and exposes the same
state through a friendly CLI, versioned JSON, and JSON-RPC over stdio.

No account. No daemon. No hosted control plane. One small binary and one
`.terminal-todo/` directory.

```text
                  ┌──────────────────────────┐
 human / agent ──▶│  CLI · JSON · JSON-RPC  │
                  └────────────┬─────────────┘
                               │
                    atomic state transitions
                               │
                  ┌────────────▼─────────────┐
                  │   project task graph     │
                  │ DAG · leases · events    │
                  └────────────┬─────────────┘
                               │
                     linked repositories
                               │
                  ┌────────────▼─────────────┐
                  │ workspace-wide progress │
                  └──────────────────────────┘
```

## Why terminal-todo?

AI-assisted work breaks down when coordination lives only inside a context
window:

- a new session cannot tell what the previous one learned;
- parallel agents race toward the same ready task;
- flat checklists hide dependencies and blockers;
- handoffs become prose scattered across chat logs and scratch files;
- work spanning repositories has no shared source of truth.

`terminal-todo` gives every participant the same persistent model:

- **Tasks form a DAG.** Ready work is derived from dependency state.
- **Claims are leases.** Ownership expires after a configurable TTL, so a
  crashed worker cannot hold work forever.
- **Acquisition is atomic.** Selecting and claiming the best compatible task is
  one locked transaction.
- **Agent retries are safe.** Durable request IDs make `acquire` idempotent.
- **State is observable.** Structured metadata, logs, and an append-only event
  stream preserve findings and transitions.
- **Repositories can be linked.** A task can depend on work in another local
  checkout using a stable URI.

## Quick start

### 1. Build and install

`terminal-todo` currently installs from source and requires Go 1.26 or newer.

```bash
git clone https://github.com/bharat94/terminal-todo.git
cd terminal-todo
make build
sudo make install
```

To install without writing to `/usr/local/bin`, build the binary and copy it
somewhere already on your `PATH`:

```bash
go build -o todo .
mkdir -p "$HOME/.local/bin"
install -m 755 todo "$HOME/.local/bin/todo"
```

### 2. Initialize a project

Run this at the root of the project whose work you want to track:

```bash
todo init
```

### 3. Create a dependency graph

```bash
todo add "Design the authentication flow" --priority 0.9 --caps architecture
todo add "Implement token validation" --after 1 --caps go,security
todo add "Add authentication tests" --after 2 --caps go,testing
```

### 4. See what can move now

```bash
todo status
todo next
todo graph
```

### 5. Claim and finish work

```bash
todo claim 1 --as bharat --ttl 30m
todo update 1 --as bharat --set finding="Use short-lived access tokens"
todo done 1 --as bharat
```

Completing task `1` makes task `2` eligible automatically.

## The agent loop

For autonomous workers, use `acquire` instead of a separate `next` followed by
`claim`. The selection and lease happen atomically, so concurrent workers
cannot both receive the same task.

```bash
todo agent-card \
  --as go-worker-1 \
  --caps go,testing,concurrency \
  --desc "Go implementation and test specialist" \
  --max-load 1

todo acquire \
  --as go-worker-1 \
  --request-id 01JZQ4N6BK7R8T9W0XYZ123456 \
  --wait 30s \
  --json
```

A successful response contains the selected task and its lease:

```json
{
  "schema_version": "1",
  "request_id": "01JZQ4N6BK7R8T9W0XYZ123456",
  "replayed": false,
  "task": {
    "id": 7,
    "title": "Protect task acquisition from races",
    "status": "in_progress",
    "depends": [],
    "created": "2026-07-12T20:00:00Z",
    "metadata": {
      "capabilities": ["go", "concurrency"],
      "owner": "go-worker-1",
      "lease_expires": "2026-07-12T21:00:00Z",
      "priority": 0.9,
      "tags": [],
      "retry_count": 0,
      "log": [],
      "extra": {}
    }
  }
}
```

Use a new request ID for each new allocation attempt. If delivery is uncertain,
retry the same request with the same parameters: `terminal-todo` returns the
original result without claiming twice or extending its lease.

`--wait` keeps the CLI call open for a bounded period when no compatible task
is ready. Each retry still performs one atomic selection-and-claim transition;
capacity and idempotency errors return immediately.

When work finishes:

```bash
todo done 7 --as go-worker-1 --json
```

For work that runs longer than its lease, renew ownership before expiry:

```bash
todo heartbeat 7 --as go-worker-1 --ttl 30m --json
```

When work should return to the queue:

```bash
todo release 7 --as go-worker-1 --error "upstream API is unavailable" --json
```

Machine clients should treat these acquisition errors as scheduler outcomes:

| Error | Exit | Meaning |
|---|---:|---|
| `NO_WORK` | 6 | No compatible ready task exists; wait or back off. |
| `AGENT_AT_CAPACITY` | 7 | Finish or release active work before acquiring more. |
| `IDEMPOTENCY_CONFLICT` | 8 | The request ID was reused with different parameters. |

## How work is represented

A task is more than a title. It can carry:

| Field | Purpose |
|---|---|
| `depends` | Local or cross-repository prerequisites |
| `status` | `pending`, `in_progress`, `completed`, or `blocked` |
| `capabilities` | Skills a compatible worker should advertise |
| `owner` / `lease_expires` | Exclusive, recoverable execution ownership |
| `priority` | Allocation utility from `0.0` to `1.0` |
| `lineage` | Parent objective used during decomposition |
| `retry_count` / `last_error` | Failure and retry context |
| `block_reason` | A durable manual blocker |
| `log` / `extra` | Audit notes and structured handoff context |

Dependencies use task URIs:

```text
todo://local/12
todo://api-service/42
```

The first refers to task `12` in the current project. The second refers to task
`42` in a linked repository named `api-service`.

## Dynamic planning and handoffs

Plans change as agents learn. The graph can change with them.

Split an objective into independently claimable work:

```bash
todo decompose 10 --as planner --into '{
  "subtasks": [
    {"title": "Reproduce the race", "caps": ["go", "testing"]},
    {"title": "Design the locking fix", "caps": ["go", "concurrency"]},
    {"title": "Document the invariant", "caps": ["documentation"]}
  ]
}'
```

Decomposition releases any lease on the parent. The parent returns to
`pending`, blocked by its new subtasks, so workers can acquire the finer-grained
work independently.

Record a finding where the next session will see it:

```bash
todo update 11 --as investigator \
  --set finding="rename invalidates locks held on tasks.bin" \
  --set file=store/store.go \
  --priority 1.0 \
  --caps go,concurrency
```

Add or remove dependencies without recreating the task:

```bash
todo update 13 --add-dep todo://local/12
todo update 13 --remove-dep todo://local/11
```

Inspect the objective and its recursive progress:

```bash
todo lineage 10
todo lineage 10 --json
todo what-if 12 --json
```

## Coordinate across repositories

Suppose a frontend task cannot start until an API task in a neighboring
checkout is complete:

```bash
# From web-app/
todo link api ../api-service
todo add "Integrate profile endpoint" --after todo://api/42
```

`todo next` keeps that task blocked until `api-service` task `42` is complete.
Linked stores are read under shared locks, and the repository path is stored
relative to the current project when possible.

Managers can inspect the entire linked workspace:

```bash
todo status --all
todo status --all --json
todo caps --all --json
```

Unavailable checkouts are reported in-band; one missing repository does not
hide the rest of the workspace.

## Integrate with any agent runtime

### Versioned CLI JSON

Add `--json` to queries and core lifecycle mutations:

```bash
todo next --capabilities go,testing --json
todo status --as go-worker-1 --json
todo events 120 --json
todo graph --json
```

Every structured response includes:

```json
{"schema_version": "1"}
```

Errors use stable identifiers:

```json
{
  "schema_version": "1",
  "error": {
    "code": "NOT_OWNER",
    "message": "task 7 is claimed by go-worker-1"
  }
}
```

### JSON-RPC 2.0 over stdio

Long-lived integrations can avoid process-per-command overhead:

```bash
todo serve --stdio
```

Requests and responses are newline-delimited JSON-RPC 2.0:

```json
{"jsonrpc":"2.0","id":1,"method":"todo.next","params":{"capabilities":["go"]}}
{"jsonrpc":"2.0","id":2,"method":"todo.acquire","params":{"actor":"go-worker-1","requestId":"alloc-42"}}
{"jsonrpc":"2.0","id":3,"method":"todo.events","params":{"since":120}}
```

The server supports the complete task, graph, project, diagnostics, and agent
card surfaces. Parameters are decoded strictly, notifications are supported,
and stdin/stdout remain clean for embedding. See the
[Agent Protocol](docs/agent-protocol.md) for method schemas and error mappings.

## Concurrency and recovery guarantees

Coordination state is useful only if workers can trust it. `terminal-todo`
therefore treats every mutation as a complete read-modify-write transaction:

1. acquire a stable sidecar lock;
2. load the latest MessagePack store;
3. reclaim expired leases;
4. validate and apply the transition;
5. write and `fsync` a temporary file;
6. atomically rename it into place;
7. `fsync` the containing directory.

This design provides:

- serialized writers and concurrent readers;
- no lost updates between competing processes;
- one winner for concurrent claims and acquisitions;
- durable, observable lease expiration;
- power-loss-safe store replacement;
- schema migration on load;
- cycle detection before dependency changes commit.

For the invariants and platform details, read
[Concurrency and Locking](docs/concurrency-and-locking.md).

## Command map

Run `todo help` for the concise built-in reference.

| Area | Commands |
|---|---|
| Tasks | `add`, `status`, `cat`, `update`, `done`, `rm`, `prune`, `search` |
| Scheduling | `next`, `claim`, `acquire`, `heartbeat`, `release`, `my` |
| Dependencies | `depends`, `dependents`, `decompose`, `lineage`, `graph`, `what-if` |
| Coordination | `block`, `unblock`, `log`, `events`, `watch` |
| Agents | `agent-card`, `caps` |
| Projects | `init`, `link`, `unlink`, `config` |
| Operations | `serve`, `export`, `backup`, `restore`, `doctor` |

Common configuration:

```bash
todo config default_ttl=45m
todo config default_priority=0.5
todo config default_caps=go,testing
todo config
```

## Project files

Project state lives under `.terminal-todo/`. Files appear as the corresponding
features are used:

```text
.terminal-todo/
├── tasks.bin          # versioned MessagePack task graph and event stream
├── tasks.bin.lock     # stable advisory lock sidecar
├── agents.json        # advertised agent capabilities and load limits
├── agents.json.lock   # agent registry lock sidecar
├── repositories.json  # linked repository aliases
└── config.json        # project defaults
```

`.terminal-todo/` is ignored by this repository because it is live,
project-specific state. Choose whether to ignore, share, or synchronize it
according to your own workflow.

## Development

```bash
git clone https://github.com/bharat94/terminal-todo.git
cd terminal-todo

make build
make test
make test-race
go vet ./...
```

The test suite includes CLI integration tests, concurrent process tests,
storage migration coverage, cross-platform locking code, DAG semantics, and
JSON-RPC protocol tests. CI runs builds, race-enabled tests, and `go vet` on
Linux and macOS.

Before opening a pull request:

```bash
gofmt -w *.go dag/*.go lock/*.go store/*.go
go test ./... -race -count=1 -timeout 120s
go vet ./...
```

Small, focused commits are easiest to review. Changes to JSON fields, error
identifiers, exit codes, event types, or JSON-RPC methods should also update
the [Agent Protocol](docs/agent-protocol.md).

## Documentation

- [Vision](docs/vision.md) — the distributed multi-agent orchestration goal
- [Problem statement](docs/problem_statement.md) — why persistent shared state matters
- [System design](docs/design.md) — storage, allocation, and orchestration model
- [Agent protocol](docs/agent-protocol.md) — stable JSON and JSON-RPC contract
- [Concurrency and locking](docs/concurrency-and-locking.md) — safety invariants
- [Examples](docs/examples.md) — human and multi-agent workflows

## Direction

The current system delivers the local and multi-repository coordination core:
DAG planning, agent metadata, ownership leases, atomic allocation, dynamic
re-graphing, events, and a transport-neutral protocol.

The larger vision is **Distributed Multi-Agent Task Orchestration (DMATO)**:
a decentralized shared memory layer that can coordinate heterogeneous agents
across repositories, machines, and inference runtimes. The next frontier is
reducing polling overhead further and eventually supporting synchronization
beyond a shared filesystem.

See [Vision](docs/vision.md) for the principles and roadmap.

## License

[MIT](LICENSE) © Bharat V.
