# terminal-todo documentation

This is the documentation map for terminal-todo. Start with the path that
matches what you are trying to do; each guide links to the deeper contracts it
depends on.

## Get started

| Goal | Guide |
|---|---|
| Install or upgrade the `todo` binary | [Installation](installation.md) |
| Learn the core workflow in a few commands | [README quick start](../README.md#quick-start) |
| Follow complete human and multi-agent recipes | [Examples](examples.md) |
| Connect Codex or Claude Code | [Agent integrations](integrations.md) |
| Get help or report a problem | [Support](../SUPPORT.md) |

The shortest successful workflow is:

```bash
todo init
todo add "Implement the feature"
todo claim 1 --as developer --ttl 30m
todo done 1 --as developer
```

Autonomous workers should use `acquire` with a unique request ID instead of
selecting with `next` and then claiming separately. See the
[agent loop](../README.md#the-agent-loop).

## Understand the system

- [Problem statement](problem_statement.md) explains the coordination gap the
  project addresses.
- [System design](design.md) describes the state model, allocation, lifecycle,
  history, and trust boundary.
- [Concurrency and locking](concurrency-and-locking.md) specifies the
  transaction and crash-safety invariants.
- [Vision](vision.md) describes the longer-term distributed multi-agent
  orchestration direction without expanding the current operating boundary.

## Build an integration

- [Agent integrations](integrations.md) covers the bundled skill, automated
  Codex and Claude Code setup, MCP lifecycle, and troubleshooting.
- [Agent protocol](agent-protocol.md) is the reference for versioned CLI JSON,
  native JSON-RPC, MCP tools, errors, events, and schemas.
- [Compatibility contract](compatibility.md) defines what clients may rely on
  across releases.
- [Coordination noise budget](coordination-noise.md) explains bounded MCP
  responses and the user-facing narration boundary.

Use the protocol documents for automation. Human-readable CLI output may
improve between releases.

## Evaluate real agents

- [Real-agent conformance](conformance.md) documents the opt-in nine-scenario
  suite, evidence model, safety boundary, and current observed baseline.
- [Real-agent behavior evaluation plan](real-agent-evaluation-plan.md) turns
  the suite into a repeatable release-gate and longitudinal measurement
  program with budgets, repetitions, confidence intervals, and evidence
  controls.
- [Scenario catalog](../conformance/scenarios/README.md) documents the fixture
  schema, assertions, scoring, and extension workflow.
- [Codex default-model calibration, 2026-08-15](evaluations/2026-08-15-codex-default.md)
  records the first valid full-catalog observation and its limitations.

The default `todo conformance` command performs only local host discovery.
Model execution requires the explicit `--run` flag and may consume paid or
rate-limited usage.

## Operate and maintain

- [Compatibility contract](compatibility.md) covers supported platforms,
  filesystem requirements, schema evolution, and verification tiers.
- [Security and data lifecycle](security-and-data.md) covers permissions,
  sensitive data, backups, restore, retention, and incident response.
- [Production readiness](production-readiness.md) records the beta verdict,
  release evidence, supported boundary, and remaining 1.0 gates.
- [Releasing](releasing.md) is the maintainer procedure for verified,
  checksummed, SBOM-equipped, attested artifacts.

## Project history and decisions

- [Dogfooding retrospective](dogfooding-retrospective.md) records observed
  coordination friction and the product changes derived from it.
- [Vision](vision.md) and the [problem statement](problem_statement.md) explain
  product intent.

## Contribute

Read [CONTRIBUTING.md](../CONTRIBUTING.md) for setup, verification, and
compatibility-sensitive change requirements. Use the private process in
[SECURITY.md](../SECURITY.md) for vulnerabilities and the public guidance in
[SUPPORT.md](../SUPPORT.md) for questions and bug reports.
