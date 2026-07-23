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
- `structuredContent` retains the typed machine result, which may be a compact
  receipt when the caller explicitly requests one.

The automated coordination-cycle test measures a full acquisition followed by
receipt-based heartbeat, update, and completion. It also compares a 200-task
status payload with its constant-size visible summary.

Run the measurement with:

```bash
go test . -run 'TestMCP(VisibleNoiseBudget|StatusSummary)' -count=1
```

## Responsibility by layer

| Layer | Responsibility |
|---|---|
| terminal-todo core | Correct DAG, ownership, leases, receipts, and history |
| MCP server | Compact display content plus typed full or receipt results |
| Integration skill | Use MCP first, request bounded views, and narrate outcomes or blockers only |
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
test. Routine mutation receipts also prevent accumulated logs and metadata
from being repeated through the structured channel.

This is a cardinality contract for receipts and event pages, not a universal
byte ceiling. Full acquisition, status, cat, lineage, export, and legacy event
results may include user-authored values and remain potentially large.

This metric does not count host-rendered tool names or arguments. A real-host
evaluation should record three values separately:

1. server-controlled text bytes;
2. host-rendered coordination rows or expanded panels; and
3. assistant messages that narrate bookkeeping rather than outcomes.

That separation makes regressions actionable: server payload regressions
belong here, narration regressions belong in the skill, and tool-chrome
regressions belong in the host integration.

## Real-host evaluation baseline

On 2026-07-16, the locally installed Codex CLI 0.144.5 and Claude Code 2.1.212
were inspected without changing user configuration or sending an agent
prompt:

| Surface | Project discovery | Direct MCP evidence | Host rows and narration |
|---|---|---|---|
| Codex | `codex mcp get terminal-todo` found the project entry and reported it enabled; the already-running thread did not expose terminal-todo tools | A fresh generated integration using an explicit absolute binary path completed initialization, listed 15 tools, resolved the exact project root, and returned a 33-byte ping summary | Not measured; doing so requires a new agent turn in the host |
| Claude Code | `claude mcp get terminal-todo` found the shared `.mcp.json` entry but reported it pending interactive approval | The same client-neutral live probe passed because it starts the configured server directly | Not measured; approval and an agent turn are required |

The checkout's generated entries used the command name `todo`, but that
command was not installed on the evaluator's inherited `PATH`; host discovery
alone did not detect that launch failure. The successful live probe used a
freshly built binary and an explicit absolute `--command`, demonstrating why
`todo integrate --check --live` and the host-discovery checks are complementary.

The installed client help exposes machine-readable agent-event modes
(`codex exec --json` and Claude Code `--output-format stream-json`). Those
streams now drive the opt-in `todo conformance --run` lifecycle evaluation.
Invoking them sends a prompt to an agent and therefore remained outside this
historical read-only baseline. The
official [Codex configuration reference][codex-config] fetched during the
evaluation covers MCP discovery, tool allowlists, approvals, and timeouts, but
no project setting for collapsing tool rows. The official
[Claude Code CLI reference][claude-cli] fetched during the evaluation,
consistent with the installed client's local help, likewise exposes
text/JSON/streaming and verbose output modes, not a project-controlled
collapsing policy.

The repeatable evaluation uses a disposable initialized project, captures the
documented event stream, and records:

1. UTF-8 bytes in the MCP result's text content;
2. the number of tool-use and tool-result events;
3. the number of tool rows or panels visible in the interactive UI; and
4. assistant sentences that merely narrate coordination.

The event stream can automate the first two counts. The interactive host UI
must provide or be observed for the third, and the fourth is checked against
the bundled skill's outcome-only narration rule. See
[Real-agent conformance](conformance.md) for the executable safety and scoring
contract.

[codex-config]: https://developers.openai.com/codex/config-reference
[claude-cli]: https://docs.anthropic.com/en/docs/claude-code/cli-usage
