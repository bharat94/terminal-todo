# Coordination noise budget

Agent coordination should be rigorous in the system and nearly invisible in
the user's conversation. This document separates the parts terminal-todo can
control from the parts owned by an MCP host.

## What counts as visible noise

The server-controlled visible-noise metric is the total UTF-8 bytes in MCP
`content` text blocks for routine coordination calls. It excludes
`structuredContent`, because that data is intended for machine decisions
rather than narration.

The contract is:

- each MCP tool result contains one semantic text sentence;
- each sentence is at most 240 bytes and valid UTF-8;
- routine summaries do not include actor names, idempotency keys, raw JSON, or
  multiline command output;
- graph-size growth affects structured content, not visible summary size; and
- the complete typed result remains in `structuredContent`.

The automated coordination-cycle test measures acquire, heartbeat, update,
and completion together. It also compares a 200-task status payload with its
constant-size visible summary.

Run the measurement with:

```bash
go test . -run 'TestMCP(VisibleNoiseBudget|StatusSummary)' -count=1
```

## Responsibility by layer

| Layer | Responsibility |
|---|---|
| terminal-todo core | Correct DAG, ownership, leases, receipts, and history |
| MCP server | Compact display content plus complete structured results |
| Integration skill | Use MCP first and narrate outcomes or blockers only |
| Agent host | Decide whether tool calls, arguments, and result chrome are expanded, collapsed, or hidden |

MCP is necessary because it gives terminal-todo separate display and
structured channels. It is not sufficient to make calls invisible: the MCP
protocol does not let this server dictate how a host renders its own tool-call
UI. Host-side collapsing must therefore be treated as a host capability, not
as a terminal-todo guarantee.

## Dogfood baseline

The CLI fallback remains intentionally operator-readable. When invoked through
a visible shell tool, even `--json` output can dominate the conversation.
During the production-readiness run, a heartbeat, metadata update, completion,
and full status query rendered pages of JSON. The equivalent MCP cycle now has
a server-controlled visible budget of at most 240 bytes total in its automated
test, while preserving substantially more structured data for the agent.

This metric does not count host-rendered tool names or arguments. A real-host
evaluation should record three values separately:

1. server-controlled text bytes;
2. host-rendered coordination rows or expanded panels; and
3. assistant messages that narrate bookkeeping rather than outcomes.

That separation makes regressions actionable: server payload regressions
belong here, narration regressions belong in the skill, and tool-chrome
regressions belong in the host integration.
