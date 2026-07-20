# Design

`terminal-todo` is a user-owned coordination control plane for humans, agents,
and scripts. It stores an explicit execution graph, allocates ready work
atomically, and records lifecycle history without requiring a hosted service.

> Goals tell an agent what outcome to pursue. `terminal-todo` tells a fleet who
> should do what, in what order, and what has already happened.

## Product boundary

The current coordination boundary is a filesystem shared by cooperating
processes. Every participant must see the same files through a filesystem that
preserves advisory locks and atomic replacement. Copying or asynchronously
syncing `.terminal-todo` transfers state; it does not provide live consensus.

The project does not currently provide a network service, peer-to-peer
replication, distributed transactions, or conflict resolution between
independently modified copies.

## Surfaces

The same coordination model is available through:

- a human-readable CLI;
- stable CLI JSON envelopes for scripts;
- an MCP server for Codex, Claude, and other MCP hosts; and
- newline-delimited JSON-RPC for native process integrations.

The CLI, MCP server, and native protocol all operate on the same local store.
MCP and JSON-RPC integrations keep machine-oriented coordination out of normal
agent narration while preserving structured results for the host.

## State model

Project state lives under `.terminal-todo/`:

- `tasks.bin` contains the MessagePack task collection, ordered audit events,
  and idempotent acquisition receipts;
- `agents.json` stores reusable agent profiles;
- `config.json` stores project defaults;
- `repositories.json` maps cross-repository aliases to paths; and
- sibling `.lock` files provide stable lock identities across atomic replaces.

Backups and compacted state use the same schemas. State is deliberately
portable and controlled by the user.

Event IDs append monotonically and are never reused. Events are append-only
within the retained window; explicit compaction may remove an old prefix.
Agent-facing mutations can opt into compact acknowledgements that omit
unbounded task history, while legacy full results remain available.

A task contains an ID, title, status, dependencies, timestamps, capability
requirements, priority, lineage, findings, retry metadata, and optional
ownership lease. Local dependencies use numeric IDs. Cross-repository
dependencies use `todo://<alias>/<id>`, where the alias is resolved through
`repositories.json`.

## Transactions and durability

Mutations run under an exclusive advisory lock and commit through a temporary
file, file flush, atomic replacement, and directory flush where the operating
system exposes one. Reads normally use shared locks. A read that discovers an
expired lease may upgrade through the store's exclusive update path to reclaim
it safely.

This provides serializable state transitions between cooperating local
processes and crash-safe replacement within the documented filesystem and
platform boundary. See [Concurrency and locking](concurrency-and-locking.md)
and [Compatibility](compatibility.md).

## DAG semantics

A task is ready when it is pending and all local and resolvable
cross-repository dependencies are complete. Local graph mutations reject
cycles before commit. Cross-repository dependencies are
resolved lazily, so a cycle spanning independent stores can remain unready and
must be diagnosed at the workspace level.

Decomposition creates child tasks and makes the parent depend on them. Sibling
children are parallel unless dependencies are added between them explicitly.

## Work allocation

`acquire` is the safe worker primitive. Under one exclusive transaction it:

1. reclaims expired leases;
2. filters pending, ready work by the worker's capabilities;
3. orders candidates by priority descending, then task ID ascending;
4. checks the worker's active-load limit;
5. marks the selected task in progress with an ownership lease; and
6. records the result against an idempotency request ID.

This avoids the race inherent in querying `next` and then separately calling
`claim`. `next` remains useful for inspection; `claim` remains useful when a
specific task was deliberately assigned.

Capabilities and load limits can be supplied per request or registered in an
agent card. Allocation is deterministic priority ordering, not an optimization
or fairness algorithm.

When allocation cannot proceed, the same deterministic read model explains
whether the queue is empty, dependencies are incomplete, capabilities are
missing, work is owned elsewhere, or the worker is at capacity. Concrete
dependency references and missing capabilities remain in structured output;
human and MCP display text stays compact.

`bootstrap` is the bounded, non-allocating session view. Given a
caller-supplied actor, it combines objective progress, owned and compatible
ready work, blockers, capability demand, and recent events without claiming
work, registering identity, or dumping the full graph. Like other current
state reads, it may persist recovery of an expired lease.

## Lifecycle

- `claim` leases a specific ready task.
- `acquire` selects and leases the next compatible ready task atomically.
- `heartbeat` renews an active lease owned by the caller.
- `release` yields work, optionally recording an error and incrementing the
  attempt count. Callers that need backoff wait before acquiring again.
- `block` records a blocker and releases ownership so recovery is not tied to
  a dead worker.
- `unblock` returns blocked work to pending and repairs legacy stale ownership.
- `done` completes owned work; findings can be attached first through task
  metadata or the task log.
- `decompose` injects child work while preserving lineage.

Expired in-progress leases are reclaimed during store updates and relevant
queries. Mutation receipts make retried calls safe when clients lose a
response.

## History and retention

Task state answers what is true now. Events answer what happened. Receipts
answer whether a mutation already committed. Handoffs and findings stay with
the task graph rather than a vendor-specific chat transcript.

`prune` removes eligible completed task records. `compact` applies the explicit
retention policy to history and receipts. Neither command promises a fixed file
size; retention is intentional and inspectable.

## Trust model

All writers are trusted collaborators with filesystem access. Advisory locks
coordinate well-behaved clients but are not an authorization boundary.
`terminal-todo` validates lifecycle transitions, dependency topology, finite
priorities, ownership, leases, and protocol inputs; it does not sandbox workers
or authenticate local users.
