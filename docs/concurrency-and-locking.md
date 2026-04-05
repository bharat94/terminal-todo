# Concurrency, Locking, and Race Conditions

In a distributed multi-agent system, `terminal-todo` must guarantee state integrity despite concurrent CLI invocations from multiple autonomous agents. This document details the low-level and logic-level locking strategies.

## 1. Low-Level: Atomic File Access

To prevent binary corruption during concurrent writes, `terminal-todo` employs **Advisory File Locking** (`flock` on POSIX, `LockFileEx` on Windows).

### The Locking Protocol
- **Shared Lock (SH):** Acquired during `status`, `next`, `cat`, and `export`. Multiple agents can read the task graph simultaneously.
- **Exclusive Lock (EX):** Acquired during `add`, `done`, `claim`, `decompose`, and `rm`. This blocks all other readers and writers.
- **Timeout Strategy:** CLI calls will attempt to acquire the lock for a maximum of 500ms before failing with `Error: database locked by another agent`.

### Failure Atomicity
All writes are performed via **Rename-Swap**:
1. Write updated MessagePack data to `tasks.bin.tmp`.
2. Call `fsync()` to ensure data is on physical media.
3. `rename("tasks.bin.tmp", "tasks.bin")` (Atomic operation on most filesystems).

## 2. Logic-Level: The "Claim" Race Condition

A classic race condition occurs when two agents attempt to `claim` the same `Pending` task simultaneously. 

### Atomic Compare-And-Swap (CAS)
The `todo claim` command is implemented as a CAS operation within an exclusive file lock:

```go
func (s *TaskStore) ClaimTask(id uint64, agentID string, ttl duration) error {
    // 1. Enter EX lock (guarantees serializability)
    s.LockFile() 
    defer s.UnlockFile()

    task := s.Get(id)
    now := time.Now()

    // 2. Check current state (The "Compare" step)
    if task.Owner != "" && task.LeaseExpires > now {
        return fmt.Errorf("task already owned by %s", task.Owner)
    }

    // 3. Update state (The "Swap" step)
    task.Owner = agentID
    task.LeaseExpires = now.Add(ttl)
    
    return s.Save()
}
```

## 3. Distributed Resilience: The "Zombie Agent" Problem

If an agent claims a task and then crashes, the task would remain `In-Progress` forever if not for **Lease Expiration**.

- **Automatic Reclamation:** The `todo next` and `todo status` commands treat any task with `LeaseExpires < now` as `Pending`.
- **Preemption:** If an agent attempts to `claim` a task with an expired lease, the CLI transparently re-assigns the `Owner`.
- **Agent Heartbeats:** Agents executing long-running tasks (>15m) are responsible for calling `todo claim <id> --refresh` to extend their lease.

## 4. Cross-Repo Consistency

When Repo A depends on a task in Repo B:
- **Lock Propagation:** When checking the status of `todo://repo-b/50`, the Repo A CLI must acquire a **Shared Lock** on Repo B's `tasks.bin`.
- **Lazy Validation:** Repo A does not receive push notifications from Repo B. Instead, it re-validates the state of external dependencies only when `todo next` or `todo status` is called in the context of Repo A.

## 5. Distributed Deadlock Prevention

Cross-repo cycles (Repo A -> Repo B -> Repo A) can stall an entire swarm.
- **Inter-repo DFS:** The `todo link` command and any `todo add --after [URI]` call must perform a recursive DAG traversal across all linked repositories to ensure no global cycles are introduced.
- **Max Depth:** To prevent infinite recursion in misconfigured or malicious repo-links, the DFS is capped at a depth of 32 repos.

## 6. Summary of Safeguards

| Race Condition | Mitigation | Layer |
| :--- | :--- | :--- |
| **Binary Corruption** | `flock` + Atomic Rename | Filesystem |
| **Double Claim** | Exclusive Lock + CAS Logic | Application |
| **Agent Crash** | TTL-based Leases | Logic |
| **Stale Remote State** | Just-in-time Read-locking | Distributed |
| **Circular Dependencies** | Recursive DFS + Depth Limit | Topology |
