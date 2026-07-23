# Real-agent conformance scenarios

This directory defines the vendor-neutral behavioral contract for an agent
using terminal-todo. The fixtures grade what a worker does, not how it reasons
or which host produced the turn.

## Contract

`manifest.json` is the ordered scenario catalog. Each referenced fixture:

- starts from a clean, deterministic project state;
- uses symbolic task and actor references rather than generated IDs;
- fixes the harness clock and request IDs;
- declares the prompts and harness-controlled clock or peer actions;
- describes observable assertions over normalized operations, task state,
  events, errors, and assistant output; and
- assigns criterion IDs from `scoring-model.json`.

`scenario.schema.json` is the machine-readable fixture schema. It intentionally
does not prescribe a runner implementation. A runner may drive an MCP host,
native JSON-RPC client, or CLI-capable agent as long as it emits the normalized
evidence described below.

## Normalized evidence

Before grading, a runner converts host-specific events into this logical
record:

```json
{
  "operations": [
    {
      "actor": "subject",
      "operation": "bootstrap",
      "transport": "mcp",
      "arguments": {"capabilities": ["go"]},
      "result": {"outcome": "ok"}
    }
  ],
  "tasks": {},
  "events": [],
  "errors": [],
  "assistant_messages": []
}
```

Operations use terminal-todo's canonical method suffixes (`ping`, `bootstrap`,
`acquire`, `heartbeat`, `update`, `log`, `release`, `block`, `complete`,
`events`, and so on), independent of transport. Runners must preserve call
order. They may remove nondeterministic timestamps, process IDs, host event
IDs, and lease tokens, but must not remove actor identity, request IDs,
operation arguments, domain errors, or user-visible text before assertions
run.

CLI shell activity is normalized too. In particular, `todo next` followed by
`todo claim` must remain distinguishable from atomic `todo acquire`; a runner
must not reinterpret that pair as `acquire`.

## Deterministic execution

A full-suite runner executes every scenario in its own temporary project with
no network access other than the selected agent host and a fake terminal-todo
clock initially set to the fixture's `initial_time`. The harness creates tasks
in listed order and resolves symbolic references such as `task:work` to the
resulting local IDs. It also substitutes fixture variables in prompts and
arguments.

The harness, not the model, performs actions with `"by": "harness"`. An
`advance_clock` action changes the fake clock without sleeping. A
`concurrent_turns` action starts named workers behind one barrier so allocation
races are repeatable. Host approval, tool launch, and configuration failures
are infrastructure failures and should be reported separately from behavioral
scores.

Use a fresh conversation for each actor unless a turn explicitly resumes that
actor. Do not preload the terminal-todo skill for the discovery scenario; all
other scenarios may use the project-installed integration exactly as an end
user would.

The first executable runner intentionally starts with a real-host lifecycle
smoke evaluation. The catalog below is the versioned behavioral contract that
the runner grows into; a host must not claim full-suite conformance until all
nine fixtures are executable and reported.

## Grading

Assertions are evaluated against normalized evidence. Each assertion names one
or more scoring criteria. A criterion earns all of its points only when every
assertion that references it passes. This avoids awarding partial credit for a
dangerous lifecycle sequence.

Hard-gate failures cap the overall score at 49 even if the raw score is higher.
The gates cover race-prone allocation, acting without a valid lease, inventing
work after `NO_WORK`, losing handoff state, and ending a session with abandoned
ownership. See `scoring-model.json` for points, gates, and certification
levels.

Assistant-output assertions are semantic. Pattern lists identify obvious
leaks, while a reviewer or deterministic classifier decides whether a sentence
merely narrates bookkeeping. Product progress, a meaningful blocker, or a
handoff outcome is not bookkeeping.

## Scenario boundaries

These fixtures evaluate agent behavior, not terminal-todo's core unit-tested
storage guarantees. They deliberately cover:

1. unprompted discovery;
2. bounded session bootstrap;
3. atomic conflict-free acquisition;
4. proactive lease renewal;
5. durable cross-agent handoff;
6. structured `NO_WORK` control flow;
7. expired-lease recovery;
8. quiet user-facing narration; and
9. explicit end-of-session cleanup.

The catalog is additive. Existing scenario IDs and criterion meanings should
not be silently changed after results are published; add a new fixture or
scoring-model version instead.
