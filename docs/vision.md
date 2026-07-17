# Vision

`terminal-todo` is a user-owned coordination control plane for a fleet of
humans, agents, scripts, sessions, and repositories.

> Goals tell an agent what outcome to pursue. `terminal-todo` tells a fleet who
> should do what, in what order, and what has already happened.

Goals, memories, and thread persistence are complementary. A goal can preserve
one worker's overarching intent while `terminal-todo` operationalizes that
intent as shared work. One agent in one thread may need nothing more. The
coordination graph becomes valuable when work crosses workers, vendors, tools,
sessions, worktrees, or repositories.

Persistence is part of the product: the graph, leases, findings, handoffs, and
audit history survive individual processes and conversations. Its larger
differentiator is that this durable state is portable, structured, and shared
across a fleet rather than owned by one agent runtime.

## Principles

1. **User-owned state.** The user chooses where coordination state lives and
   can inspect, back up, export, and move it.
2. **Vendor-neutral coordination.** A human, Codex, Claude, another MCP host,
   or a script can participate without becoming the system of record.
3. **Explicit execution graphs.** Dependencies and lineage are data, not
   assumptions buried in a transcript.
4. **Conflict-free acquisition.** Selection and leasing happen atomically, so
   two workers do not receive the same work.
5. **Recoverable ownership.** Leases, heartbeats, retries, blocking, and
   release make crashes and handoffs normal lifecycle events.
6. **Structured history.** Findings, receipts, and events explain what
   happened without replaying private conversations.
7. **Honest boundaries.** Filesystem coordination is not a distributed
   consensus protocol. Guarantees are documented and tested at their real
   boundary.

## What exists now

The current product provides:

- a durable local DAG with local and linked-repository dependencies;
- atomic `acquire`, explicit `claim`, leases, heartbeats, retries, and recovery;
- capability-aware deterministic allocation and per-agent load limits;
- structured findings, lifecycle events, mutation receipts, backup, export,
  doctor, prune, and compaction;
- human CLI output, stable CLI JSON, native JSON-RPC, and an MCP server;
- reusable Codex and Claude integration material; and
- cross-platform builds and an auditable release pipeline.

All live participants must operate on the same lock-capable filesystem.
Cross-repository aliases point to other stores visible from that workspace.

## What comes next

Near-term work should make the existing control plane easier to operate:

- explain precisely why no work was allocated;
- provide a compact session/bootstrap view for a newly arrived worker;
- expose richer health and invariant diagnostics;
- improve host UI treatment of background heartbeats and coordination calls;
- validate the first public prerelease and installation paths on real hosts;
- expand integration and crash-recovery testing; and
- document patterns for worktrees and multi-repository workspaces.

## Later possibilities

An optional service-backed coordination layer could support workers that cannot
share a filesystem. That would require authentication, authorization,
network-partition semantics, conflict resolution, migrations, and operational
ownership. It should extend the local model rather than blur its guarantees.

Likewise, richer scheduling may eventually consider fairness, cost, affinity,
or deadlines. Today allocation intentionally stays understandable:
capability-compatible ready tasks are ordered by priority and task ID.

## Success looks like

- Two independent workers cannot atomically acquire the same task.
- A crashed worker's task becomes recoverable after its lease expires.
- A replacement worker can understand the handoff from structured state.
- Dependencies make execution order visible across linked repositories.
- A user can move or archive the coordination record without a vendor export.
- Normal coordination remains quiet in the conversation while full structured
  detail remains available to tools and operators.
- Every production claim has a test, a documented boundary, or an explicit
  roadmap item.
