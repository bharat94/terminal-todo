# Vision: Distributed Multi-Agent Task Orchestration (DMATO)

## Core Vision

`terminal-todo` is not just a todo list; it is a **Distributed Multi-Agent Task
Orchestration (DMATO)** layer. It provides a decentralized, user-owned
execution graph for autonomous agents, humans, and scripts to coordinate,
decompose, and execute complex objectives across heterogeneous environments
and repositories.

In the era of distributed inference, where multiple specialized agents (LLMs, SLMs, and traditional scripts) collaborate in real-time, `terminal-todo` acts as the **source of truth for progress, ownership, and state**.

> Goals tell an agent what outcome to pursue. `terminal-todo` tells a fleet who
> should do what, in what order, and what has already happened.

Agent-native goals, memories, and thread persistence are complementary. A goal
can own the overarching objective while `terminal-todo` operationalizes it as
a shared execution DAG. For one agent in one thread, a goal may be sufficient.
`terminal-todo` is designed for the point where multiple workers, vendors,
tools, sessions, worktrees, or repositories must coordinate.

## Key Principles

1. **Decentralized Coordination** — No central server; task state is shared via the filesystem (and eventually peer-to-peer protocols), enabling cross-repo and cross-machine collaboration.
2. **Portable, User-Owned State** — Agents can observe and update the shared
   task graph without making any vendor's thread or memory system the
   coordination authority.
3. **Durable Execution History** — The graph, ownership transitions, findings,
   and audit trail persist across agent restarts, process crashes, and context
   resets.
4. **Agentic Task Autonomy** — Every task is an autonomous unit with its own metadata, retry logic, and ownership, adhering to a global DAG structure.
5. **Submodular Optimization** — Task allocation uses distributed greedy algorithms to ensure conflict-free execution with minimal computational overhead.
6. **Universal Cognition Interface** — A standardized CLI and binary protocol that allows any agentic system to leverage shared coordination knowledge.

## High-Level Architecture

### The "Agentic Task" Model
Beyond simple titles, tasks are rich objects containing:
- **Capabilities Required:** Semantic tags describing the skills needed (e.g., `golang`, `k8s-deploy`).
- **Owner/Lease:** Identifiers for the agent currently executing the task to prevent duplicate work.
- **Priority & Utility:** Dynamic values used for submodular task allocation.
- **Lineage:** Proof of decomposition from a higher-level objective.

### Distributed DAG Semantics
- **Cross-Repo Dependencies:** A task in Repo A can depend on a task in Repo B via URI-based referencing (`todo://repo-b/task-42`).
- **Dynamic Re-graphing:** Agents can inject, prune, or split tasks as the objective evolves during execution.

## The Problem It Solves

### 1. The Fleet Coordination Boundary
Agent goals preserve intent for an individual worker. They do not assign
conflict-free work across independent agents, vendors, humans, scripts,
worktrees, and repositories. `terminal-todo` provides the shared execution
control plane.

### 2. Multi-Agent Race Conditions
In a shared environment, multiple agents might attempt the same task. `terminal-todo` provides **Lease-based Concurrency Control**.

### 3. Decomposition Blindness
Complex goals require breaking down into atomic, dependent steps. `terminal-todo` enforces **DAG-based Structural Planning**.

### 4. Opaque Handoffs
Chat transcripts and private memories do not provide a portable operational
record. `terminal-todo` preserves structured findings, ownership transitions,
retries, blockers, and audit history in state controlled by the user.

## Success Criteria

- **Autonomous Handoff:** Agent A completes Task 1, and Agent B automatically picks up Task 2 based on capability match.
- **Global Visibility:** A "manager" agent can visualize the entire multi-repo progress by aggregating local DAGs.
- **Resilience:** The task graph survives agent crashes, context window expirations, and network partitions.

## Roadmap: Toward Distributed Inference

| Phase | Goal | Key Innovation |
|-------|------|----------------|
| **Current** | Local DAG | MessagePack storage, cycle detection, basic CLI. |
| **Orchestration** | Agentic Metadata | Ownership leases, capability tags, priority scoring. |
| **Distributed** | Cross-Repo Linking | URI-based dependencies, multi-root aggregation. |
| **Autonomous** | Dynamic Planning | Agents re-architecting the DAG in-flight. |
| **Production** | Global Sync | Gossip-based synchronization or shared ledger backends. |

## References & Inspiration

- **DeMAC (2025):** Dynamic DAGs for multi-agent feedback loops.
- **Flint Engine:** Task-as-an-agent distributed execution.
- **DGBA Algorithm:** Distributed Greedy Bundles for conflict-free allocation.
- **MDI-LLM:** Model-distributed inference architectures.
