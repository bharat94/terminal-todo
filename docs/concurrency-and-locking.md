# Concurrency, Locking, and Race Conditions

In a distributed multi-agent system, `terminal-todo` must guarantee state integrity despite concurrent CLI invocations from multiple autonomous agents. This document details the low-level and logic-level locking strategies.

## 1. Low-Level: Atomic File Access

To prevent binary corruption during concurrent writes, `terminal-todo` employs **Advisory File Locking** (`flock` on POSIX, `LockFileEx` on Windows).

### The Locking Protocol
- **Shared Lock (SH):** Acquired during `status`, `next`, `cat`, and `export`. Multiple agents can read the task graph simultaneously.
- **Exclusive Lock (EX):** Acquired during `add`, `done`, `claim`, `decompose`, and `rm`. This blocks all other readers and writers.
- **Contention Strategy:** CLI calls retry the non-blocking OS primitive until
  they acquire the lock. Internal callers can supply a bounded timeout.

### Failure Atomicity
All writes are performed via **Rename-Swap**:
1. Write updated MessagePack data to a private temporary file.
2. Call `fsync()` on the complete temporary file.
3. Atomically rename it over `tasks.bin`.
4. Call `fsync()` on the containing directory on platforms that expose the
   operation.

The stable lock is `tasks.bin.lock`, not the replaceable data inode. Writers
therefore cannot invalidate mutual exclusion by renaming `tasks.bin`.
Windows flushes the complete temporary file before the atomic replace but
cannot explicitly flush a directory through Go; see the
[compatibility contract](compatibility.md) for the resulting boundary.

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
- **Agent Heartbeats:** Agents executing long-running tasks renew the active
  owned lease with `todo heartbeat <id> --as <owner> --ttl <duration>`.
  Expired leases cannot be revived by a delayed heartbeat.

## 4. Cross-Repo Consistency

Linked repositories are resolved lazily when readiness or lifecycle transitions
are evaluated. Repo A depending on Repo B uses:
- **Lock Propagation:** When checking the status of `todo://repo-b/50`, the Repo A CLI must acquire a **Shared Lock** on Repo B's `tasks.bin`.
- **Lazy Validation:** Repo A does not receive push notifications from Repo B. Instead, it re-validates the state of external dependencies only when `todo next` or `todo status` is called in the context of Repo A.

## 5. Cross-repository cycle boundaries

Local DAG changes are rejected before commit when they create a local cycle.
Cross-repository dependencies are resolved lazily. A global cycle across
separately managed stores can therefore remain permanently unready and must be
diagnosed at the workspace level; terminal-todo does not claim a distributed
transaction or global-cycle guarantee.

## 6. Summary of Safeguards

| Race Condition | Mitigation | Layer |
| :--- | :--- | :--- |
| **Binary Corruption** | `flock` + Atomic Rename | Filesystem |
| **Double Claim** | Exclusive Lock + CAS Logic | Application |
| **Agent Crash** | TTL-based Leases | Logic |
| **Stale Remote State** | Just-in-time Read-locking | Distributed |
| **Circular Dependencies** | Recursive DFS + Depth Limit | Topology |
