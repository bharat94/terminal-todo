# Dogfooding retrospective

This retrospective records a production-readiness run in which terminal-todo
coordinated its own development. It covers the product experience, not just
whether commands returned the correct result.

## Executive finding

The coordination model worked. The interaction layer was too visible.

The DAG prevented work from starting before its prerequisites. Atomic
acquisition, ownership, max-load enforcement, heartbeats, and explicit
completion kept the execution state honest. Durable task metadata preserved
commits, test evidence, and findings across a long development run.

The weak point was the agent-facing transport. Using the CLI through a shell
made bookkeeping look like product work: commands were visible, JSON results
were verbose, and routine heartbeats created user-facing noise. MCP is the
right default surface for agents, but MCP alone does not solve the problem.
The server must return compact display content, the skill must teach quiet
coordination, and the host decides whether tool calls are visible or
collapsed.

## What felt good

### The graph changed behavior

Dependencies were operational, not decorative. Final audit work stayed
unavailable while release, security, compatibility, and retrospective tasks
were incomplete. Completing prerequisites made downstream work ready without
manually reconstructing the plan.

### Acquisition and leases created real ownership

The allocator selected one ready task atomically. A max load of one prevented
the same actor from silently accumulating work. Heartbeats made long-running
CI and release checks explicit, while completion released ownership cleanly.

### Durable handoffs were genuinely useful

Task metadata carried commit IDs, CI run evidence, and platform findings.
That was more useful than storing only a high-level objective: the graph
remembered what had happened and why, while the thread-level goal retained the
overall outcome.

### Failures exposed product gaps

Native Windows CI found an unsupported directory-sync assumption. The task
graph preserved that finding and the verification run that closed it. The
dogfood loop therefore improved both the product and its operating contract.

## What felt rough

### Shell transport leaked mechanics

Every CLI call traveled through a visible shell tool. Even concise one-line
results exposed actor names, request IDs, lease renewals, and other details
that rarely help the end user. JSON was correct for machine decisions but
especially noisy when rendered in a conversation.

### MCP originally duplicated the full payload

The first MCP implementation returned the complete JSON value in both a text
content block and `structuredContent`. Hosts commonly render text content,
which turned a machine-readable result into visible chatter and doubled the
payload.

### The skill optimized correctness but not presentation

The original skill correctly required atomic acquisition, stable actors,
idempotency keys, and heartbeats. It did not say that these operations should
stay in the background. An agent could follow every invariant and still
narrate every bookkeeping step.

### Capability mismatch was underexplained

An acquisition attempt advertised `product,ux,mcp,docs,go` while the ready
task also required `integration`. The allocator correctly returned no work,
but “no compatible tasks ready” did not identify the missing capability. A
task detail lookup was needed to diagnose it.

### Session identity remained manual

The actor name and acquisition request IDs had to be chosen and reused by the
agent. That is safe but repetitive. A typo can look like another worker, and a
request ID must be retried exactly to preserve idempotency.

### Installation does not hot-load a running host

`todo integrate` correctly installed Codex and Claude configuration, but the
already-running session could not discover a newly registered MCP server. The
rest of that session had to continue through the CLI. This is a host lifecycle
constraint and should be clear during onboarding.

### Portability is not the same as remote consensus

Project-local state was easy to inspect, back up, and control. Agents sharing
one filesystem could coordinate immediately. Agents on different machines
still need user-chosen shared storage with correct locking semantics or a
future network service; copying state is transfer, not live coordination.

## Changes made from this run

1. MCP text content is now a concise operation summary capped at 240 bytes.
   The complete result remains available in `structuredContent`.
2. MCP initialization tells agents to keep routine coordination in the
   background and report only meaningful outcomes and blockers.
3. The canonical Codex and Claude skill prefers MCP, uses CLI as a fallback,
   and explicitly forbids echoing raw commands, JSON, actor IDs, request IDs,
   and routine heartbeats to users.
4. Integration and protocol documentation distinguish compact display content
   from complete structured data.
5. MCP tests enforce the compact-output contract while proving structured
   content remains complete.

## Systematic improvement plan

### P0: quiet, structured agent transport

Status: implemented in this run.

- Keep MCP as the default agent surface and CLI as the operator/fallback
  surface.
- Keep visible MCP text bounded and semantic.
- Preserve full typed data separately.
- Teach agents to narrate outcomes, not coordination calls.

### P1: explain allocation decisions

Status: next.

- Add structured `NO_WORK` diagnostics that distinguish no ready work,
  dependency blocking, capability mismatch, and ownership/capacity.
- Return the smallest useful set of missing capabilities and blocker task IDs.
- Keep the visible error one line; put detail in structured error data.

### P1: first-class session bootstrap

Status: next.

- Provide a session bootstrap operation that establishes or resumes a stable
  actor identity.
- Make request-ID generation easy while preserving caller-controlled retry
  identity.
- Expose current ownership and recommended heartbeat timing in one compact
  result.

### P1: incremental reads

Status: protocol support exists; workflow refinement remains.

- Prefer event cursors and actor-filtered status after initial discovery.
- Avoid repeatedly transferring the full graph during long sessions.
- Add resource or subscription support only when host interoperability is
  proven; do not add protocol surface for novelty.

### P2: host-aware onboarding

Status: planned.

- Detect and explain when integration was installed but requires a host
  restart.
- Add a minimal integration health check that verifies binary discovery, MCP
  initialization, tool listing, and project-root resolution.
- Keep setup output concise by default and offer detail on request.

### P2: remote coordination boundary

Status: future architecture decision.

- Preserve the file-backed local mode as the simple, user-owned default.
- Define a separate service-backed mode only if cross-machine coordination
  demand justifies consensus, authentication, and availability costs.
- Do not imply that ordinary network filesystems provide a distributed
  database.

## Product principle

Goals tell an agent what outcome to pursue. terminal-todo tells a fleet who
should do what, in what order, and what has already happened.

Persistence remains a feature, but the product becomes distinctive when
durable state is combined with cross-agent allocation, explicit dependencies,
leases, recovery, handoffs, and audit history. The ideal user experience is
therefore paradoxical in a useful way: coordination should be rigorous in the
system and nearly invisible in the conversation.
