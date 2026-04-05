# Design: Distributed Task Orchestration System

## Storage Layer

### Location

Local State: `<project>/.terminal-todo/tasks.bin`
Global Config: `~/.config/terminal-todo/config.json`

### Format: Enhanced Binary (MessagePack)

We use an extensible MessagePack schema to support production-grade metadata without sacrificing performance.

**Agentic Task Schema:**
```go
type Task struct {
    ID           uint64            `msgpack:"id"`
    Title        string            `msgpack:"title"`
    Status       uint8             // 0=Pending, 1=In-Progress, 2=Completed, 3=Blocked
    Depends      []TaskURI         `msgpack:"depends"`     // Support for cross-repo URI
    Created      uint64            `msgpack:"created"`     // Unix ms
    Completed    uint64            `msgpack:"completed"`   // Unix ms
    
    // Agentic Metadata
    Capabilities []string          `msgpack:"caps"`        // e.g., ["go", "refactor"]
    Owner        string            `msgpack:"owner"`       // Agent ID or UUID
    LeaseExpires uint64            `msgpack:"lease_exp"`   // Expiration for ownership
    Priority     float32           `msgpack:"priority"`    // Task utility for allocation
    Lineage      string            `msgpack:"lineage"`     // Parent Goal ID
    Extra        map[string]string `msgpack:"extra"`       // Extensible KV for agents
}

type TaskURI string // Format: todo://[repo-id]/[task-id]
```

---

## Distributed Coordination Protocol

### Task URI & Cross-Repo Referencing

To coordinate across repositories, `terminal-todo` uses a URI-based scheme:
- `todo://local/101` -> Task 101 in the current repository.
- `todo://infra-repo/50` -> Task 50 in a known repository named `infra-repo`.

**Resolution Strategy:**
Agents resolve `infra-repo` by looking up a global registry in `~/.config/terminal-todo/repositories.json`.

### Inference-Time Ownership (Lease Management)

To prevent race conditions, agents must acquire a **Lease** before working on a task:
1. **Request:** `todo claim <id> --as <agent-name> --ttl 30m`
2. **Success:** `terminal-todo` updates `Owner` and `LeaseExpires`.
3. **Heartbeat:** Long-running agents must periodically refresh the lease.
4. **Failure/Timeout:** If the lease expires, the task returns to `Pending`.

### Submodular Allocation (Distributed Greedy)

When an agent is idle:
1. It queries `todo next --ready --capabilities go`.
2. It receives a list of tasks it is capable of performing, sorted by `Priority`.
3. It claims the top task.

---

## CLI Infrastructure (The "Orchestration Interface")

The CLI is designed for **deterministic agent interaction**.

### Orchestration Commands

| Command | Args | Description |
|---------|------|-------------|
| `todo claim` | `<id> --as <name>` | Secure an exclusive execution lease |
| `todo release` | `<id>` | Yield a lease back to the pool |
| `todo decompose` | `<id> --into "<json>"` | Split a task into sub-tasks (DAG injection) |
| `todo link` | `<repo-alias> <path>` | Register a remote repo for cross-repo deps |
| `todo sync` | | (Future) Propagate task state to peers |

### Advanced Queries for Agents

- `todo next --ready --capabilities [caps]` -> Returns JSON of actionable tasks matching agent skills.
- `todo lineage <goal-id>` -> Shows the progress of a specific high-level objective.

---

## Orchestration Flow

1. **Manager Initiation:** A "Manager" agent initializes the project objective and decomposes it into primary tasks (`todo add ...`).
2. **Worker Discovery:** Worker agents query `todo next` and filter by their specific capabilities.
3. **Execution Lease:** Workers `claim` tasks. Other agents now see the task as `In-Progress`.
4. **Dynamic Re-Planning:** If a worker discovers a blocker, it uses `todo decompose` to inject new sub-tasks into the DAG or updates `Extra` metadata with findings.
5. **Completion:** On `todo done`, the DAG automatically unblocks dependent tasks.

---

## Concurrency & Resilience

To support high-frequency interaction from multiple agents, `terminal-todo` implements a multi-layered safety architecture. See [Concurrency, Locking, and Race Conditions](concurrency-and-locking.md) for full details.

### Multi-Reader Single-Writer Locking
The system uses advisory file locking to allow concurrent read operations while ensuring write operations are strictly serialized. This prevents binary corruption when multiple agents query `todo next` while another agent is `claiming` a task.

### Atomic State Transitions
Task updates follow a Compare-And-Swap (CAS) pattern. A `claim` or `done` operation will only succeed if the underlying state matches the agent's expectation at the moment of the write lock.

### Fault Tolerance
- **Atomic Rename:** New state is written to a temporary file and renamed, ensuring a valid `tasks.bin` always exists even during power failures.
- **Lease Timeout:** Long-running agents must maintain "heartbeat" leases. If an agent crashes, its claimed tasks are automatically reclaimed by the pool after the TTL expires.

---

## Error Handling & Reliability

### State Consistency
- **Atomic Writes:** Uses file-level locking during binary updates to prevent corruption from concurrent agent CLI calls.
- **Cycle Detection:** Recursive DFS validation on every `add` or `link`.

### Distributed Edge Cases
- **Stale Leases:** Automatically detected on `todo next` or `todo status` calls.
- **Disconnected Repos:** Cross-repo dependencies are marked as "Unknown/Stale" if the target repo is inaccessible.

---

## Reference Implementation Details

- **Concurrency:** Go's `flock` or equivalent for file-level mutual exclusion.
- **Serialization:** `msgpack` with custom type resolvers for `TaskURI`.
- **Validation:** JSON Schema validation for the `Extra` metadata field to ensure agent interoperability.
