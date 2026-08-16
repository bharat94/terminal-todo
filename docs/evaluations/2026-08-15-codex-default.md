# Codex default-model calibration — 2026-08-15

## Decision

This is a valid single-run calibration, not a certification.

Codex passed seven of the nine real-agent scenarios. The suite earned a raw
score of `75/100`, then two hard-gate failures capped the result at `49/100`
and classified the configuration as `non_conformant` under
`terminal-todo-agent-conformance-v1`.

Do not generalize this result beyond the exact configuration below. One run
does not establish a reliability rate, and the moving default model was not
reported as a stable model ID.

## Configuration

| Field | Value |
|---|---|
| Suite | `terminal-todo-real-agent-v1` |
| Scoring model | `terminal-todo-agent-conformance-v1` |
| terminal-todo commit | `a41994611168b61fe30ec283774ca021a6764876` |
| Binary SHA-256 | `66fef40e3578ebce04f474f4ee37e8a19730ec1b67f76155a5e3e0dac498247a` |
| Reported integration version | `dev` |
| Host | Codex CLI `0.144.5` |
| Model | Host default; exact stable model ID not reported |
| Transport | MCP over stdio |
| Platform | macOS arm64, local filesystem |
| Repetitions | One complete suite; twelve actor turns |
| Isolation | Nine disposable synthetic workspaces, `workspace-write` sandbox |
| Raw host events | Omitted |
| Retained workspaces | None |

The run used the approved single-suite budget. The current report schema does
not expose aggregate token or monetary usage, so neither value is recorded.
Usage capture remains a P0 item in the
[evaluation plan](../real-agent-evaluation-plan.md#automation-backlog).

## Scenario results

| Scenario | Result | MCP operations | Evidence summary |
|---|---:|---:|---|
| `discovery` | Pass | 6 | Discovered the integration, bootstrapped, and acquired work. |
| `bootstrap` | Pass | 1 | Used one bounded bootstrap as the initial coordination read. |
| `atomic_acquire` | Pass | 2 | Two actors called atomic acquire; one received structured `NO_WORK`. |
| `heartbeat` | Fail | 9 | Lease remained valid, but progress was recorded with `log`; the assertion requires `update` after heartbeat. |
| `handoff` | Fail | 7 | Author called update then release, and the successor recovered the checksum constraint; the expected `extra.finding` trace shape did not match. |
| `no_work` | Pass | 1 | One acquire returned structured `NO_WORK`; no work was fabricated. |
| `lease_recovery` | Pass | 5 | Successor reclaimed expired work with its own identity. |
| `quiet_narration` | Pass | 4 | Final narration was concise and contained no protocol leakage. |
| `cleanup` | Pass | 7 | The task ended blocked and unowned rather than abandoned. |

No authentication, approval, launch, timeout, output-limit, normalization, or
other infrastructure failure occurred.

## Score

| Criterion | Points | Result |
|---|---:|---|
| Discovers coordination | 10 | Pass |
| Starts bounded | 10 | Pass |
| Allocates atomically | 15 | Pass |
| Maintains lease | 10 | Fail |
| Hands off durably | 15 | Fail |
| Handles no work | 10 | Pass |
| Recovers expired lease | 15 | Pass |
| Coordinates quietly | 10 | Pass |
| Closes ownership | 5 | Pass |

The failed heartbeat assertion triggered `invalid_lease_mutation`; the failed
handoff assertion triggered `lost_handoff`. Either gate caps a suite below
conformance, regardless of its raw score.

## Interpretation

The seven passing scenarios are meaningful evidence that the installed Codex
host can discover and use terminal-todo through MCP, resolve atomic
contention, handle an empty queue, recover expired work, keep routine
coordination quiet, and close ownership.

The two failures need careful treatment:

- The heartbeat trace contains heartbeat followed by durable `log` calls, and
  the final lease was valid. The fixture currently accepts only `update` as
  its progress mutation even though the bundled skill permits both structured
  updates and audit logs. This may be a benchmark-contract mismatch rather
  than unsafe lease behavior.
- The handoff trace contains `update` before `release`, and the successor's
  final message demonstrated that the checksum constraint survived in durable
  state. The failing assertion expects a specific `extra.finding` argument
  shape. Because compact reporting omitted arguments and the workspace was
  deleted, this run cannot establish which key or value shape the host used.

These observations do not erase the recorded failures. Changing the accepted
operation or metadata shape after seeing a result would require a reviewed
catalog decision and, if it changes benchmark meaning, a new suite or scoring
version. The next step is to reconcile the written behavioral contract, skill
guidance, and assertion semantics before authorizing more paid repetitions.

## Instrumentation history

An earlier attempt on the same date emitted `10/100`, but every MCP operation
trace was empty and the synthetic clock was not inherited by the MCP child.
Persisted audit events proved that tool mutations had occurred, making that
score invalid. The defect was fixed in `a419946`; it is not included in this
baseline or any behavioral denominator.

## Reproduction boundary

The equivalent opt-in command is:

```bash
todo conformance --run --host codex --json
```

The exact result is not guaranteed to repeat because the host default model is
a moving alias. Future release-gate runs should pass `--model` with an explicit
stable model ID and retain a compact checksummed report according to the
[real-agent evaluation plan](../real-agent-evaluation-plan.md).
