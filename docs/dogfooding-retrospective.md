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

### Bootstrap reduced resume cost

After the first implementation pass, one bounded bootstrap replaced separate
status, ownership, and event-history reads. Against this project's live graph
it summarized an objective at 83% completion, the one explicit publication
gate, current capability demand, and three recent events out of 55. It also
surfaced and durably recovered an expired integration-task lease. That was the
first point where resuming coordination felt like reading a brief instead of
querying a task database.

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
6. Allocation failures now explain exact dependency, capability, ownership,
   retry-history, and capacity conditions in structured data.
7. A bounded bootstrap gives a newly arrived worker the smallest useful
   objective, ownership, ready-work, blocker, capability, and event brief.
8. Remaining lifecycle mutations now support versioned CLI JSON and
   structured errors without changing human output.
9. Every MCP tool advertises explicit title and behavior annotations, and the
   installer can perform a live handshake, tool-list, ping, and root check.
10. A test-backed noise budget measures server-visible bytes independently
    from host-rendered tool chrome and assistant narration.

## Systematic improvement plan

### P0: quiet, structured agent transport

Status: implemented in this run.

- Keep MCP as the default agent surface and CLI as the operator/fallback
  surface.
- Keep visible MCP text bounded and semantic.
- Preserve full typed data separately.
- Teach agents to narrate outcomes, not coordination calls.
- Enforce the server-controlled [coordination noise
  budget](coordination-noise.md) independently from host-rendered tool chrome.

### P1: explain allocation decisions

Status: implemented in this run.

- Structured `NO_WORK` diagnostics distinguish no ready work,
  dependency blocking, capability mismatch, and ownership/capacity.
- Responses include deterministic missing capabilities and blocker task
  references.
- Visible errors stay compact while full detail lives in structured error data.

### P1: first-class session bootstrap

Status: bounded session brief implemented; identity and request-ID ergonomics
remain.

- `bootstrap` provides one bounded view of objective progress, current
  ownership, compatible ready work, blockers, capability demand, and recent
  events.
- The caller still chooses the stable actor and request IDs; this preserves
  portable identity and retry semantics across vendors.
- MCP and CLI summaries remain compact while structured content carries the
  complete brief.

### P1: incremental reads

Status: protocol support exists; workflow refinement remains.

- Prefer event cursors and actor-filtered status after initial discovery.
- Avoid repeatedly transferring the full graph during long sessions.
- Add resource or subscription support only when host interoperability is
  proven; do not add protocol surface for novelty.

### P2: host-aware onboarding

Status: live integration check implemented; restart detection remains planned.

- Detect and explain when integration was installed but requires a host
  restart.
- A live integration health check verifies binary discovery, MCP
  initialization, tool listing, and project-root resolution.
- Keep setup output concise by default and offer detail on request.

Host-side tool collapsing remains outside the MCP server's control. Measure
server text, host-rendered tool rows, and assistant narration separately so a
presentation regression is assigned to the layer that can fix it.

The second dogfood pass confirmed the intended split: bootstrap made discovery
small, allocation diagnostics made empty work actionable, and MCP summaries
made routine transitions terse. A third pass addressed the remaining
mutation/event amplification: routine lifecycle calls can now return compact
receipts, and event consumers can opt into cursor-based bounded pages with
retention-gap detection. Legacy full results remain available for operators
and compatible version 1 clients.

This bounds receipt cardinality, not arbitrary user-authored value sizes in
full detail reads. `status`, `cat`, `lineage`, `export`, acquisition results,
and legacy unpaged events remain deliberate potentially-large operations.

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
