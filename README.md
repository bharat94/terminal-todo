# terminal-todo

**A durable coordination layer for humans and AI agents.**

[![CI](https://github.com/bharat94/terminal-todo/actions/workflows/ci.yml/badge.svg)](https://github.com/bharat94/terminal-todo/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`terminal-todo` is a user-owned execution graph that people, coding agents, and
scripts can safely share. It coordinates who should do what, in what order,
and what has already happened across vendors, sessions, and repositories. The
same state is available through a friendly CLI, versioned JSON, MCP, and the
complete native JSON-RPC API over stdio.

No account. No daemon. No hosted control plane. One small binary and one
`.terminal-todo/` directory.

```text
                  ┌──────────────────────────┐
 human / agent ──▶│ CLI · JSON · MCP · RPC  │
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

> Goals tell an agent what outcome to pursue. `terminal-todo` tells a fleet who
> should do what, in what order, and what has already happened.

Agent-native goals, memories, and planning features are useful for maintaining
one worker's intent. For one agent in one thread, they may be enough.
`terminal-todo` becomes valuable when execution crosses a coordination
boundary:

- several agents or humans need conflict-free work allocation;
- Claude, Codex, scripts, and other runtimes need one shared contract;
- dependencies determine which work is actually ready;
- ownership must survive crashes without remaining stuck forever;
- findings and retries must be handed to a different worker;
- work spans sessions, worktrees, or linked repositories;
- the user needs portable state that no agent vendor owns.

`terminal-todo` gives every participant the same persistent model:

- **Tasks form a DAG.** Ready work is derived from dependency state.
- **Claims are leases.** Ownership expires after a configurable TTL, so a
  crashed worker cannot hold work forever.
- **Acquisition is atomic.** Selecting and claiming the best compatible task is
  one locked transaction.
- **Agent retries are safe.** Durable request IDs make `acquire` idempotent.
- **Handoffs are structured.** Metadata, retries, logs, and an append-only
  event stream preserve findings and transitions.
- **Execution state persists.** The graph, ownership history, and handoff
  context survive process exits, context resets, and agent restarts.
- **Repositories can be linked.** A task can depend on work in another local
  checkout using a stable URI.
- **State belongs to the user.** The coordination record remains portable,
  inspectable, and independent of any agent runtime.

## Quick start

### 1. Install

For tagged versions, download the archive for your platform from
[GitHub Releases](https://github.com/bharat94/terminal-todo/releases), verify
it against `checksums.txt`, and place `todo` on your `PATH`. If no tagged
release is available yet, build the current release candidate from source.

To build from source, install Go 1.26.1 or newer:

```bash
git clone https://github.com/bharat94/terminal-todo.git
cd terminal-todo
make build
sudo install -m 755 todo /usr/local/bin/todo
```

To install without writing to `/usr/local/bin`, build the binary and copy it
somewhere already on your `PATH`:

```bash
go build -o todo .
mkdir -p "$HOME/.local/bin"
install -m 755 todo "$HOME/.local/bin/todo"
```

The release workflow is configured for Linux, macOS, and Windows on amd64 and
arm64, with SHA-256 checksums, SPDX SBOMs, and GitHub provenance attestations.
See [Releasing](docs/releasing.md) for the validation status, verification,
and maintainer procedures.
Release archives are the canonical binary distribution for the beta. The Go
module has a public import path, but a supported `go install .../cmd/todo`
layout is future work.

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

todo bootstrap --as go-worker-1 --json

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
todo done 7 --as go-worker-1 --receipt
```

For work that runs longer than its lease, renew ownership before expiry:

```bash
todo heartbeat 7 --as go-worker-1 --ttl 30m --receipt
```

When work should return to the queue:

```bash
todo release 7 --as go-worker-1 --error "upstream API is unavailable" --receipt
```

`--receipt` returns a bounded acknowledgement—operation, affected IDs,
lifecycle state, and an explicit detail follow-up—without repeating task logs,
metadata, error text, or request IDs. Legacy `--json` results remain available
when the next decision needs the complete task.

Machine clients should treat these acquisition errors as scheduler outcomes:

| Error | Exit | Meaning |
|---|---:|---|
| `NO_WORK` | 6 | Inspect `error.data.reason` to distinguish an empty queue, dependencies, capability gaps, and work owned by others. |
| `AGENT_AT_CAPACITY` | 7 | Inspect current/max load, then finish or release active work before acquiring more. |
| `IDEMPOTENCY_CONFLICT` | 8 | The request ID was reused with different parameters. |

`NO_WORK` and `AGENT_AT_CAPACITY` keep compact messages while returning a
structured allocation diagnostic in CLI JSON, native JSON-RPC error data, and
MCP structured content. It includes deterministic dependency blocker
references, missing capability names, ownership/load counts, and the number of
pending tasks that have previously been retried.

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
Linked stores are normally read under shared locks. A read that finds an
expired lease may enter an exclusive update to persist reclamation safely.
Repository paths are stored relative to the current project when possible.

Managers can inspect the entire linked workspace:

```bash
todo status --all
todo status --all --json
todo caps --all --json
```

Unavailable checkouts are reported in-band; one missing repository does not
hide the rest of the workspace.

## Integrate with any agent runtime

Install project-scoped Codex and Claude Code integrations in one command:

```bash
todo integrate
```

This installs the bundled skill and MCP configuration while preserving
unrelated client settings:

| Client | Skill | MCP configuration |
|--------|-------|-------------------|
| Codex | `.agents/skills/terminal-todo/` | `.codex/config.toml` |
| Claude Code | `.claude/skills/terminal-todo/` | `.mcp.json` |

Target one client with `todo integrate codex` or `todo integrate claude`.
`todo integrate --check` is a read-only drift check suitable for CI.
`todo integrate --check --live` additionally launches the configured binary,
negotiates MCP, lists tools, and verifies project-root resolution.
Existing terminal-todo settings or modified skill files are never overwritten
silently; inspect them, then use `--force` if replacement is intentional.
If the binary is not named `todo`, set the MCP launch command explicitly:

```bash
todo integrate --command /absolute/path/to/todo
```

See [Agent integrations](docs/integrations.md) for lifecycle details,
generated files, and troubleshooting.

### Verify real-agent behavior

`todo conformance` checks which supported agent hosts are locally available
without contacting a model. Real execution requires an explicit flag:

```bash
todo conformance
todo conformance --run --host codex --json
```

The opt-in run creates a disposable synthetic project and grades the persisted
task state and audit trail after a real Codex or Claude Code turn. It measures
bounded bootstrap, atomic acquisition, structured handoff data, ownership
cleanup, and quiet outcome narration. Authentication or MCP approval problems
are reported separately as infrastructure skips. The controlled prompt may
consume host usage; repository source is not placed in the fixture.

See [Real-agent conformance](docs/conformance.md) for the safety model, current
lifecycle smoke evaluation, and the nine-scenario vendor-neutral contract.

### Reusable agent skill

The repository includes a canonical
[`terminal-todo` skill](integrations/skills/terminal-todo/SKILL.md) for agents
that support the open `SKILL.md` format. It teaches the complete coordination
workflow: inspect existing state, acquire work atomically, maintain leases,
record durable findings, decompose discoveries, and complete or release work
without abandoning ownership.

For local authoring, copy or symlink the skill into the location used by your
agent runtime:

```bash
# Codex, available to all repositories
mkdir -p "$HOME/.agents/skills"
cp -R integrations/skills/terminal-todo "$HOME/.agents/skills/"

# Claude Code, available to all repositories
mkdir -p "$HOME/.claude/skills"
cp -R integrations/skills/terminal-todo "$HOME/.claude/skills/"
```

The automated project integration above is recommended for teams. These
global copies are useful for agents that should discover the workflow in every
repository. The CLI remains the source of truth; the skill supplies the
reliable operating procedure.

### Model Context Protocol

Codex, Claude Code, and any MCP client can use terminal-todo as native tools:

```bash
todo mcp --stdio
```

The server implements the MCP `2025-06-18` stdio lifecycle and exposes a
curated coordination surface: discovery, initialization, bounded worker
bootstrap, status, task detail, creation, atomic acquisition, heartbeats,
updates, logs, decomposition, blocking, release, completion, and events. Every
tool advertises an MCP title and explicit read-only, destructive, idempotent,
and open-world hints. Tool calls return both text and structured JSON: the
text is a compact human trace, while `structuredContent` contains the
machine result. The bundled skill requests compact receipts for routine
mutations and bounded event pages, then fetches full detail only for decisions
that require it. Coordination stays in the background so user-facing updates
remain focused on the work.

Register it in Codex:

```bash
codex mcp add terminal-todo -- todo mcp --stdio
```

Or in Claude Code for the current project:

```bash
claude mcp add --transport stdio --scope project terminal-todo -- todo mcp --stdio
```

Start the client from an initialized project directory so the server discovers
the correct `.terminal-todo/` state. The MCP server can also initialize a new
project through `terminal_todo_init`.

See the [dogfooding retrospective](docs/dogfooding-retrospective.md) for the
interaction principles and prioritized UX follow-ups behind this design.

### Versioned CLI JSON

Add `--json` to queries and core lifecycle mutations:

```bash
todo next --capabilities go,testing --json
todo status --as go-worker-1 --json
todo block 7 --as go-worker-1 --reason "waiting for credentials" --json
todo decompose 8 --as go-worker-1 --into '{"subtasks":[{"title":"Reproduce"}]}' --json
todo heartbeat 7 --as go-worker-1 --ttl 30m --receipt
todo events 120 --limit 100 --json
todo graph --json
```

Every structured response includes:

```json
{"schema_version": "1"}
```

Mutation receipts are explicit and additive; without `--receipt`, existing
version 1 result shapes are unchanged. Event pagination is likewise opt-in
with `--limit`. Continue from `cursor.next_since`; if
`cursor.history_gap` is true, resynchronize from current status because the
user has compacted an older event prefix.

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
{"jsonrpc":"2.0","id":0,"method":"todo.ping","params":{}}
{"jsonrpc":"2.0","id":1,"method":"todo.bootstrap","params":{"actor":"go-worker-1"}}
{"jsonrpc":"2.0","id":2,"method":"todo.next","params":{"capabilities":["go"]}}
{"jsonrpc":"2.0","id":3,"method":"todo.acquire","params":{"actor":"go-worker-1","requestId":"alloc-42"}}
{"jsonrpc":"2.0","id":4,"method":"todo.events","params":{"since":120,"page":true,"limit":100}}
```

This native API supports the complete task, graph, project, diagnostics, and
agent-card surfaces. Parameters are decoded strictly, notifications are
supported, and stdin/stdout remain clean for embedding. `todo.ping` advertises
the protocol version and supported coordination features before
initialization. For standards-based agent integration, use `todo mcp --stdio`.
See the
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
7. `fsync` the containing directory where the operating system exposes that
   operation.

This design provides:

- serialized writers and concurrent readers;
- no lost updates between competing processes;
- one winner for concurrent claims and acquisitions;
- durable, observable lease expiration;
- complete-file store replacement, with the strongest flush semantics each
  supported operating system exposes;
- schema migration on load;
- cycle detection before dependency changes commit.

For the invariants and platform details, read
[Concurrency and Locking](docs/concurrency-and-locking.md).

## Security, privacy, and retention

terminal-todo is a local coordination control plane, not an authentication
boundary. New state is private to the current OS user: `.terminal-todo/` is
created with mode `0700`, and state, backups, and lock files use `0600`.
`todo doctor --fix` detects and repairs broader legacy permissions.

Task titles, logs, errors, actor names, and metadata are persisted in cleartext.
Do not store secrets. Anyone who can run a configured MCP server with your OS
account has the same project authority as the CLI; the server is stdio-only
and should not be exposed as an unauthenticated network service.

Audit events and successful acquisition receipts are durable by default.
Retention is explicit and previewable:

```bash
todo compact --keep-events 10000 --receipts-before 2160h --dry-run
todo backup
todo compact --keep-events 10000 --receipts-before 2160h
```

Receipts are removed only when their task is completed or no longer present.
After a receipt is compacted, its request ID no longer has replay protection,
so autonomous workers should continue generating globally unique IDs.
Backups and restores cover the complete coordination store, including events
and idempotency receipts. See
[Security and data lifecycle](docs/security-and-data.md).

## Command map

Run `todo help` for the concise built-in reference.

| Area | Commands |
|---|---|
| Tasks | `add`, `status`, `cat`, `update`, `done`, `rm`, `prune`, `search` |
| Scheduling | `next`, `claim`, `acquire`, `heartbeat`, `release`, `my` |
| Dependencies | `depends`, `dependents`, `decompose`, `lineage`, `graph`, `what-if` |
| Coordination | `block`, `unblock`, `log`, `events`, `watch` |
| Agents | `bootstrap`, `agent-card`, `caps` |
| Projects | `init`, `link`, `unlink`, `config` |
| Operations | `serve`, `mcp`, `integrate`, `conformance`, `export`, `backup`, `restore`, `compact`, `doctor` |

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
├── tasks.bin          # task graph, events, and idempotency receipts
├── tasks.bin.lock     # stable advisory lock sidecar
├── agents.json        # advertised agent capabilities and load limits
├── agents.json.lock   # agent registry lock sidecar
├── repositories.json  # linked repository aliases
├── repositories.json.lock
├── config.json        # project defaults
├── config.json.lock
└── backup-*.bin       # snapshots created by `todo backup`
```

`.terminal-todo/` is ignored by this repository because it is live,
project-specific state. Live sharing requires one filesystem that preserves
the documented advisory-lock and atomic-replace semantics for every worker.
Copying or asynchronously synchronizing the directory is useful for transfer
or backup, but is not live consensus. The directory is private to the creating
OS user by default.

## Development

Read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a substantial change.
Security reports follow the private process in [SECURITY.md](SECURITY.md).

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
MCP/JSON-RPC protocol tests. The conformance package additionally checks the
real-host adapter contract, bounded event capture, redaction, persisted-state
grading, and all nine catalog fixtures without contacting a model. CI runs
trimmed builds, race-enabled tests, process-level MCP and integration smoke
tests, and `go vet` on Linux, macOS, and Windows. It also validates release
configuration and scans reachable code against the Go vulnerability database.

Before opening a pull request:

```bash
gofmt -w .
go test ./... -race -count=1 -timeout 300s
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
- [Compatibility contract](docs/compatibility.md) — supported platforms, filesystems, schemas, and CI evidence
- [Security and data lifecycle](docs/security-and-data.md) — trust boundary, permissions, retention, and recovery
- [Agent integrations](docs/integrations.md) — Codex, Claude Code, skills, and MCP
- [Real-agent conformance](docs/conformance.md) — opt-in host evaluation and behavioral contract
- [Releasing](docs/releasing.md) — verified artifacts and maintainer workflow
- [Production readiness](docs/production-readiness.md) — evidence, release gate, and known boundaries
- [Dogfooding retrospective](docs/dogfooding-retrospective.md) — observed UX friction and improvement plan
- [Coordination noise budget](docs/coordination-noise.md) — measurable MCP output contract and host boundary
- [Examples](docs/examples.md) — human and multi-agent workflows

## Community

- [Contributing](CONTRIBUTING.md)
- [Support](SUPPORT.md)
- [Security policy](SECURITY.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)

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
