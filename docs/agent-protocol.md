# Multi-Agent Coordination Protocol (MACP)

This document defines the JSON interface for agent-to-agent and agent-to-CLI communication in `terminal-todo`. Adherence to this protocol ensures that heterogeneous agents can collaborate without semantic drift.

## 1. Task Representation (JSON)

When an agent queries `todo cat <id> --json` or `todo status --json`, the following schema is used.

```json
{
  "id": 101,
  "title": "Implement auth middleware",
  "status": "in_progress",
  "depends": ["todo://local/100"],
  "metadata": {
    "capabilities": ["go", "security"],
    "owner": "agent-alpha-4",
    "lease_expires": "2026-04-04T14:30:00Z",
    "priority": 0.85,
    "lineage": "goal-auth-v1",
    "extra": {
      "github_issue": "42",
      "suggested_approach": "Use JWT with RS256"
    }
  }
}
```

## 2. Discovery Protocol (`todo next`)

Agents use this to find work. The CLI filters tasks based on the agent's reported capabilities.

**Request:**
`todo next --ready --capabilities go,security --json`

**Response:**
```json
{
  "available_tasks": [
    {
      "id": 101,
      "title": "Implement auth middleware",
      "priority": 0.85,
      "reason": "Ready: Dependency todo://local/100 completed"
    }
  ],
  "blocked_summary": {
    "count": 5,
    "primary_blockers": ["todo://local/99"]
  }
}
```

## 3. Decomposition Protocol (`todo decompose`)

When an agent breaks a task into sub-tasks, it must provide a structured decomposition plan.

**Request:**
`todo decompose 101 --into '{"subtasks": [{"title": "Write unit tests", "caps": ["go"]}, {"title": "Update docs"}]}'`

**Effect:**
1. Task 101 is marked as `Blocked` (or remains `In-Progress` as a parent).
2. New tasks are created.
3. Dependencies are automatically set so that the new tasks must be completed before Task 101 can be marked `Done`.

## 4. Lease & Ownership Flow

To ensure exclusive execution in distributed inference:

1. **State: Pending** -> Any capable agent can see it.
2. **Action: Claim** -> Agent calls `todo claim 101 --as agent-alpha-4 --ttl 15m`.
3. **State: In-Progress** -> Task is locked to `agent-alpha-4`.
4. **Heartbeat:** Agent must call `todo claim 101` again before 15m elapses to retain the lock.
5. **Release/Done:** Agent calls `todo done 101` or `todo release 101`.

## 5. Agent Best Practices

- **Atomic Decomposition:** Always decompose large tasks before starting execution.
- **Semantic Tagging:** Use standard capability tags (e.g., `lang:go`, `tool:docker`) to help the allocator.
- **Context Sharing:** Use the `extra` field to pass findings (e.g., "Found bug in line 42") to the next agent in the DAG.
