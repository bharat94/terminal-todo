# Plans

This directory holds durable engineering plans: multi-phase work that spans
more pull requests than a single issue description can carry.

A plan is a checked-in design document, not a task tracker. Live execution
state belongs in the project's `.terminal-todo` graph; the plan explains the
reasoning, the sequencing, and the acceptance criteria that the graph
references.

## Conventions

- One file per plan, named `YYYY-MM-DD-<slug>.md`.
- Every plan states its status at the top: `proposed`, `in progress`,
  `superseded`, or `complete`.
- Evidence claims must be reproducible. If a plan asserts a defect, it must
  include the exact commands that demonstrate it.
- Update the plan when reality diverges from it. A stale plan is worse than
  no plan.

## Index

| Plan | Status | Summary |
|---|---|---|
| [Coordination core refactor](2026-08-16-coordination-core-refactor.md) | In progress | Collapse the duplicated CLI and JSON-RPC implementations onto one typed coordination core, repair the broken error contract, and unlock a testable, installable package layout. |
| [Deep bugfix and hardening](2026-08-22-deep-bugfix-and-hardening.md) | Proposed | Harden the store, lifecycle, and harness after four parallel bug hunts (38 defects): migrations, lease boundaries, atomicity, ghost receipts, redaction, and determinism. |
