# Real-agent conformance

Unit tests can prove that terminal-todo serializes mutations, enforces leases,
and serves a stable protocol. They cannot prove that a real coding agent will
discover the integration, acquire work atomically, preserve a handoff, or keep
coordination noise out of the user conversation.

The real-agent conformance suite measures that final boundary.

## Safety model

The default command is a local preflight:

```bash
todo conformance
todo conformance --host codex --json
```

It finds the selected host executables and records their versions. It does not
send a prompt, contact a model, or consume inference usage.

Real execution is deliberately opt-in:

```bash
todo conformance --run --host codex
todo conformance --run --host all --json
```

`--run` sends one controlled prompt to each selected host and may consume paid
or rate-limited usage. The host uses its existing local authentication. The
evaluation does not read or transmit repository source: it creates a
disposable project containing only one synthetic terminal-todo task and
project-scoped MCP configuration. The prompt restricts the agent to
coordination tools and forbids shell commands and file edits.

Use `--keep-workspace` only for debugging. Otherwise the fixture is removed
after normalization. Captured output is bounded, stdout and stderr remain
separate, and the report redacts the disposable path, the evaluation prompt,
explicit secrets, and host session/thread identifiers.

## What executes today

The executable `lifecycle_smoke` scenario asks one real agent to:

1. resume through one bounded bootstrap;
2. acquire the synthetic task atomically with a stable request ID;
3. persist a unique structured marker;
4. complete the task as its lease owner; and
5. return one concise outcome sentence without raw coordination payloads.

Correctness is graded from terminal-todo's persisted store and audit events,
not from the agent claiming that it succeeded. Host JSONL is used only for
process telemetry and the final assistant message.

Authentication, MCP trust/approval, missing executables, timeouts, output
limits, and launch errors are infrastructure results. An unauthenticated or
unapproved host is `skipped`, not behaviorally failed. A host that runs but
leaves the synthetic task in the wrong state is `failed`.

The adapters use the documented non-interactive event streams:

- Codex runs `codex exec --json --ephemeral` with the fixture MCP server
  required at startup. See the official
  [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
  and [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
- Claude Code runs print mode with verbose `stream-json`, strict fixture MCP
  configuration, no session persistence, and only terminal-todo MCP tools
  allowed. See the official
  [headless mode](https://code.claude.com/docs/en/headless) and
  [MCP documentation](https://code.claude.com/docs/en/mcp).

## Full behavioral contract

[`conformance/scenarios/manifest.json`](../conformance/scenarios/manifest.json)
is the versioned vendor-neutral catalog. It defines nine scenarios:

| Scenario | Behavior |
|---|---|
| `discovery` | Find the project integration without the prompt naming terminal-todo |
| `bootstrap` | Start with one bounded session brief |
| `atomic_acquire` | Resolve concurrent contention without `next` plus `claim` races |
| `heartbeat` | Renew ownership before a long-running lease checkpoint |
| `handoff` | Persist a material finding before release and consume it as a successor |
| `no_work` | Treat `NO_WORK` as structured control flow |
| `lease_recovery` | Reacquire expired work without impersonating the stale owner |
| `quiet_narration` | Keep routine coordination out of user-facing narration |
| `cleanup` | End every session with complete, block, or release |

The accompanying schema and 100-point scoring model are checked in the normal
Go test suite. Hard-gate failures cap the result below conformance for
race-prone allocation, invalid lease mutation, fabricated work, lost handoff,
or abandoned ownership.

The current command is a lifecycle smoke evaluation, not a claim that all nine
catalog fixtures are executable. Reports must identify the scenario they ran;
full-suite certification comes only after the remaining deterministic clock,
multi-turn, and concurrent-host drivers are implemented.

## Observed local baseline

The production-readiness evaluation that introduced this runner produced:

| Host | Version | Result | Evidence |
|---|---|---|---|
| Codex | 0.144.5 | Passed, 100/100 | Bounded bootstrap, atomic acquire, structured marker update, completion, and one concise final sentence were all observed; the persisted audit state matched the host stream |
| Claude Code | 2.1.215 | Infrastructure skip | Local authentication reported `loggedIn: false`, so the runner stopped before sending a prompt |

The Codex turn completed in about 28 seconds and reported 148,463 input tokens,
of which 128,512 were cached, plus 532 output tokens. Host context and caching
vary, so operators should treat real-agent evaluation as a budgeted check, not
as ordinary unit-test traffic.

This is evidence for one installed host and authentication environment, not a
portable certification of every model or host release.

## Automation policy

Normal CI validates:

- fixture/catalog consistency;
- injection-safe host argv;
- process isolation and output bounds;
- recursive redaction;
- structured infrastructure failures;
- evidence normalization, assertions, and hard-gate scoring.

CI does not run paid hosts or depend on maintainer credentials. A scheduled or
release-candidate environment may run `todo conformance --run --json` only
when it has an explicit usage budget, controlled credentials, and a policy for
retaining the redacted report.

Useful options:

```text
--host codex|claude|all
--run
--json
--include-events
--model <host-model>
--timeout <duration>
--keep-workspace
```
