# Concurrency, Locking, and Race Conditions

In a distributed multi-agent system, `terminal-todo` must guarantee state integrity despite concurrent CLI invocations from multiple autonomous agents. This document details the low-level and logic-level locking strategies.

## 1. Low-Level: Atomic File Access

To prevent binary corruption during concurrent writes, `terminal-todo` employs **Advisory File Locking** (`flock` on POSIX, `LockFileEx` on Windows).

### The Locking Protocol
- **Shared Lock (SH):** Normally acquired during `status`, `next`, `cat`, and
  `export`. Multiple agents can read the task graph simultaneously. A query
  that discovers expired ownership can enter the exclusive update path to
  persist lease reclamation safely.
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

## 2. Logic-Level: Ownership Races

A classic race occurs when two agents attempt to own the same pending task.
`claim` validates and updates a chosen task under one exclusive lock. For
worker scheduling, `acquire` is stronger: it selects the next compatible ready
task and creates its lease in the same transaction.

Within that transaction the store reclaims expired leases, checks readiness
and ownership, mutates the selected task, appends its event, records the
idempotency receipt, and atomically replaces the affected files. A second
writer observes the committed state rather than the pre-mutation candidate.

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
- **Lock propagation:** When checking `todo://repo-b/50`, Repo A normally
  acquires a shared lock on Repo B. If Repo B contains an expired lease that
  must be reclaimed, its store may safely enter an exclusive update.
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
| **Local circular dependencies** | Cycle detection before commit | Topology |
