# Vision: Distributed Multi-Agent Task Orchestration (DMATO)

## Core Vision

`terminal-todo` is not just a todo list; it is a **Distributed Multi-Agent Task Orchestration (DMATO)** layer. It provides a decentralized, DAG-based shared memory for autonomous agents to coordinate, decompose, and execute complex objectives across heterogeneous environments and repositories. 

In the era of distributed inference, where multiple specialized agents (LLMs, SLMs, and traditional scripts) collaborate in real-time, `terminal-todo` acts as the **source of truth for progress, ownership, and state**.

## Key Principles

1. **Decentralized Coordination** — No central server; task state is shared via the filesystem (and eventually peer-to-peer protocols), enabling cross-repo and cross-machine collaboration.
2. **Inference-Time Shared State** — Agents can observe and update the global task graph during their inference cycles, allowing for dynamic re-planning and emergent coordination.
3. **Agentic Task Autonomy** — Every task is an autonomous unit with its own metadata, retry logic, and ownership, adhering to a global DAG structure.
4. **Submodular Optimization** — Task allocation uses distributed greedy algorithms to ensure conflict-free execution with minimal computational overhead.
5. **Universal Cognition Interface** — A standardized CLI and binary protocol that allows any agentic system to leverage shared coordination knowledge.

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

### 1. The Context Reset Barrier
Agents often lose track of long-running objectives across session boundaries. `terminal-todo` provides **Permanent Objective Memory**.

### 2. Multi-Agent Race Conditions
In a shared environment, multiple agents might attempt the same task. `terminal-todo` provides **Lease-based Concurrency Control**.

### 3. Decomposition Blindness
Complex goals require breaking down into atomic, dependent steps. `terminal-todo` enforces **DAG-based Structural Planning**.

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
