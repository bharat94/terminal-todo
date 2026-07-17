# Examples

These examples use human-readable output for setup and atomic acquisition for
workers. Integrations should normally use MCP or `--json` and stable request
IDs; see the [agent protocol](agent-protocol.md).

## Human and agent handoff

Create an objective and decompose it:

```bash
todo init
todo add "Implement user authentication"
todo decompose 1 --into '{
  "subtasks": [
    {"title": "Set up token validation", "caps": ["go"]},
    {"title": "Create login endpoint", "caps": ["go"]}
  ]
}'
```

Decomposed siblings are parallel by default. If the endpoint must follow token
validation, make that ordering explicit:

```bash
todo update 3 --add-dep 2
```

A Go-capable worker atomically selects and leases the first ready task:

```bash
todo acquire --as agent-alpha --capabilities go \
  --ttl 30m --request-id session-42-acquire-1
```

During long work it heartbeats. At completion it records a portable handoff:

```bash
todo heartbeat 2 --as agent-alpha --ttl 30m
todo update 2 --as agent-alpha \
  --set finding="Token validation implemented; tests cover expiry and bad signatures"
todo done 2 --as agent-alpha
```

Task 3 then becomes ready.

## Parallel specialists

Create independent work with capability requirements:

```bash
todo add "Fix injection risk in search" --priority 0.9 --caps security
todo add "Reduce search latency" --priority 0.6 --caps performance
```

Each specialist acquires only compatible work:

```bash
todo acquire --as security-agent --capabilities security \
  --request-id security-run-7
todo acquire --as perf-agent --capabilities performance \
  --request-id perf-run-3
```

Selection and lease creation happen in one transaction, so concurrent workers
cannot both acquire the same task.

## Block, hand off, and recover

A worker that needs an external decision can block its task and leave context:

```bash
todo update 8 --as agent-alpha \
  --set finding="Implementation supports either 15m or 60m without schema changes"
todo block 8 --as agent-alpha \
  --reason "Need the identity team's token lifetime decision"
```

Blocking releases the ownership lease. After the decision:

```bash
todo unblock 8
todo acquire --as agent-beta --capabilities go \
  --request-id agent-beta-acquire-1
```

If a worker crashes instead, its in-progress lease becomes reclaimable after
the TTL. A delayed heartbeat cannot revive an expired lease.

## Cross-repository ordering

In `api-service`, create the prerequisite:

```bash
todo add "Create /v1/users/profile endpoint"
```

In the sibling `web-app`, register the repository and depend on its task:

```bash
todo link api-service ../api-service
todo add "Build profile page" --after todo://api-service/42
```

The frontend task remains unready until task 42 is complete in the linked
store. Both repositories must be visible through the same lock-capable
filesystem; linking is not network replication.

## Priority and deterministic allocation

```bash
todo add "Fix production crash" --priority 1
todo add "Refactor logging" --priority 0.3
```

Among compatible ready tasks, `acquire` orders priority descending and then ID
ascending. Priority affects selection; it does not preempt a task that another
worker already owns.

## Failed attempt and caller-controlled backoff

When work fails transiently, release it with the error:

```bash
todo release 12 --as deploy-agent \
  --error "Registry returned HTTP 503 after upload began"
```

The task becomes pending again, with its attempt count and history available
to the next worker. A caller that needs backoff should wait before its next
`acquire`; terminal-todo does not currently schedule a retry timestamp.

## Retention and audit history

```bash
todo status
todo prune
todo compact
todo doctor
```

`prune` removes eligible completed task records. `compact` applies configured
retention to events and idempotency receipts. File size depends on retained
history and metadata; these commands intentionally do not promise a fixed
size.
