# Coordination core refactor

- Status: in progress — Phase 0 complete, Phase 1 underway
- Opened: 2026-08-16
- Last updated: 2026-08-16
- Baseline commit: `bd88b9f`
- Owner: maintainer

## Summary

terminal-todo implements its coordination semantics twice. The CLI implements
each lifecycle operation in `cmd_*.go`, and the JSON-RPC server implements the
same operation again in `serve.go`; the MCP server is a thin adapter over the
JSON-RPC dispatch table and therefore inherits the second copy. Nothing forces
the two copies to agree.

The visible cost is already in production. The documented CLI error taxonomy
is unreachable for every mutating lifecycle command, so ordinary ownership and
lifecycle conflicts are reported to agents and scripts as `STORE_CORRUPTED` —
documented as "Task store or config file is corrupted". The hidden cost is
structural: the duplicated logic cannot be unit tested, the root package sits
at 37.2% coverage against 65-96% for every extracted sub-package, and nothing
in the module is importable, which is why `go install` is still unsupported.

This plan collapses both surfaces onto one typed coordination core. The
pattern is not new to this repository: `renewLease` in `lease.go` is already
shared by `cmdHeartbeat` and `handleHeartbeat` and already classifies its
failures with sentinel errors and `errors.Is`. The plan generalizes the
pattern that heartbeat alone got right.

## Evidence

### Defect 1 — the documented CLI error contract is unreachable

`docs/agent-protocol.md:604-611` publishes a CLI error code and exit code for
each failure class. Mutating lifecycle commands cannot produce any of them,
because `updateStore` in `todo.go` routes every non-input error to
`fail(ErrStoreCorrupted, ...)` and no command classifies the domain errors it
raises inside its own mutation closure.

Reproduction against `bd88b9f`:

```bash
todo init && todo add "A" && todo add "B"
todo claim 1 --as w1

todo claim 1 --as w2 --json   # documented: ALREADY_CLAIMED, exit 5
todo done 1 --as w2 --json    # documented: NOT_OWNER, exit 1
todo release 2 --as w1 --json # documented: a lifecycle error
todo claim 99 --as w1 --json  # documented: TASK_NOT_FOUND, exit 1
```

All four return:

```json
{ "schema_version": "1", "error": { "code": "STORE_CORRUPTED", "message": "…" } }
```

with exit status 2. There are 26 such domain errors raised inside `updateStore`
closures across `cmd_*.go`, and every one of them is misclassified.

This matters beyond tidiness. The bundled skill teaches agents to read
structured error codes and treat them as control flow. An agent that sees
`STORE_CORRUPTED` should stop and escalate, not pick different work. Today a
routine ownership conflict tells it the store is corrupt.

### Defect 2 — the JSON-RPC surface classifies errors by substring

`serve.go` recovers the error class from the error text at 22 sites, for
example:

```go
if strings.Contains(err.Error(), "not found")            { … rpcTaskNotFound }
if strings.Contains(err.Error(), "already claimed")      { … rpcAlreadyClaimed }
if strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "unblock") { … }
```

Two consequences follow. First, the classification is incomplete in the same
way as the CLI: `handleClaim` matches `"already claimed"` but the store also
raises `"task %d is already completed"`, which falls through to
`rpcStoreCorrupted`. `handleRelease` never matches `"task %d is not in
progress"`, so releasing a pending task is reported as store corruption over
MCP too. Second, error strings are load-bearing. Rewording a message is a
silent protocol break, and the compatibility contract has no way to express
that.

### Defect 3 — the two surfaces have already drifted

Comparing the pairs directly:

| Operation | CLI | JSON-RPC |
|---|---|---|
| `claim` conflict | `already claimed by %s (expires in %s)` | `already claimed by %s` |
| `done` conflict | `is claimed by %s; use --as %s` | `is claimed by %s` |
| `done` result | task envelope | `{"completed": […], "unblocked": []}` where `unblocked` is hardcoded empty and never populated |

Only mutation receipts have a parity test
(`mutation_receipt_test.go:266`). Everything else — resulting task state,
emitted events, retry counters, error codes — is unverified across surfaces.

### Structural costs

| Symptom | Measurement |
|---|---|
| Root package coverage | 37.2%, versus 65.6-96.3% for `store`, `dag`, `lock`, `fsutil`, `conformance` |
| `cmd_*.go` coverage | 0.0% — the CLI is exercised only by `exec.Command` against a built binary, so no statement is attributed |
| Test wall time | ~50s for the root package, dominated by subprocess spawning |
| `serve.go` | 2,702 lines, 51 handlers, one file |
| Package layout | 83 files, all `package main`; nothing importable; `go install` unsupported (acknowledged in `README.md`) |
| CLI argument parsing | five parallel flag tables (`valueFlags`, `booleanFlags`, `knownCommands`, `parseIDs`'s `valueOptions`, `extractTitle`'s skip list) that must be edited in lockstep |

`os.Exit` inside `fail()` is what makes the CLI untestable in process, which is
what forces subprocess tests, which is what hides the coverage.

## Non-goals

- Changing coordination semantics. Where the CLI and JSON-RPC disagree today,
  the reconciled behavior is chosen deliberately and recorded, but no new
  lifecycle rules are introduced.
- Changing the on-disk schema, the MCP tool list, or the JSON-RPC method
  names.
- Changing human-readable CLI output. `docs/compatibility.md` already
  declares it unstable; keeping it stable anyway avoids pointless churn.
- Expanding the operating boundary. No network mode, no daemon.
- Rewriting the conformance harness. Phase 4 reconciles its contract; it does
  not redesign it.

## Compatibility position

Fixing Defect 1 changes CLI error codes and exit statuses for failure paths.
That is a behavior change on a documented surface, and it must be treated as
one:

- It is a **fix toward** the published contract in `docs/agent-protocol.md`,
  not a departure from it. No documented code or exit status changes meaning;
  codes that were unreachable become reachable.
- Callers that branch on exit status 2 to mean "store is corrupt" were
  already wrong, but scripts may have adapted to the bug. The change is
  called out in the release notes and in `docs/compatibility.md`.
- The protocol schema version does not change, because no envelope shape,
  field, or code definition changes.

## Phases

Phases land as separate pull requests and are independently revertible. Each
phase must leave `make test-race` and the full CI matrix green.

### Phase 0 — Prove the contract, then fix it

**Status: complete.** Landed in `fe31a7a`.

Establish the safety net before moving any logic, so the refactor is provably
behavior-preserving rather than hopefully so.

1. **Cross-surface parity harness.** A table-driven test that, for each
   lifecycle operation and each failure condition, runs the operation through
   CLI JSON, JSON-RPC, and MCP against identical fixture stores and asserts
   the three agree on: error code, resulting task state, emitted events, and
   retry counters. The harness is the artifact; the current disagreements it
   finds are recorded as expectations to fix, not silently normalized.
2. **Typed coordination errors.** A sentinel error set with `errors.Is`
   classification, following `lease.go`'s existing
   `errLeaseNotOwner`/`errLeaseNotActive` precedent. One classification
   function maps a domain error to a CLI `ErrorCode` and to an `rpcError`
   code, so both surfaces cannot disagree.
3. **Delete substring classification.** Replace all 22 `strings.Contains(err.
   Error(), …)` sites and the CLI's `updateStore` catch-all.

Exit criteria: the reproductions in Defect 1 return their documented codes and
exit statuses on both surfaces; zero `strings.Contains(err.Error()` remains in
`serve.go`; the parity harness covers claim, acquire, done, release, block,
unblock, handoff, heartbeat, decompose, update, rm.

Outcome: all four Defect 1 reproductions now return their documented code and
exit status. All 22 substring sites are gone, plus two the original survey
missed — one in `cmd_bootstrap.go` and one in `handleBootstrap` that used a
differently named error variable. The guard test that found them matches any
`strings.Contains|HasPrefix|HasSuffix|Index` against an `*err*.Error()`
receiver, so the narrower original pattern cannot hide a variant again.

The parity harness covers 18 failure conditions. It also established that
`claim` and `unblock` are deliberately absent from the MCP surface — atomic
`acquire` is the supported allocation primitive — which is now pinned by
`TestMCPOmitsRaceProneAllocation` rather than left implicit.

`INVALID_TRANSITION` (exit 1, JSON-RPC `-32014`) was added rather than
shoehorning lifecycle-state conflicts into an existing code. Error identifiers
are append-only protocol values, so adding one is contract-legal; reporting
"claiming completed work" as `DEPENDENCY_ERROR` or `INVALID_ARGS` would not
have been honest.

### Phase 1 — One core per operation

**Status: in progress.** Seven of twelve operations extracted in `6e96d86`
and the commit following it.

Extract each lifecycle operation into `internal/coord` as a function over
`*store.TaskStore` that returns a typed result and a typed error, exactly as
`renewLease` does today. Migrate one operation per commit so each step is
reviewable and bisectable.

Order, easiest to hardest: `heartbeat` (already done — becomes the reference),
`release`, `block`, `unblock`, `claim`, `done`, `handoff`, `log`, `rm`,
`update`, `decompose`, `acquire`.

Extracted so far into `coordination_ops.go`: `claim`, `complete`, `release`,
`block`, `unblock`, `log`, `decompose`. `heartbeat` and `handoff` already had
shared cores in `lease.go` and `handoff.go`; `handoff`'s hand-rolled
classification switch was collapsed into the shared classifier. Remaining:
`update`, `rm`, `prune`, `acquire`.

Extraction is not always a pure merge. `decompose` publishes two different
capability field names — `caps` in the CLI `--into` document, `capabilities`
in the JSON-RPC subtask — and both are documented. Aliasing them onto one type
silently broke the CLI format; the existing suite caught it, and the CLI shape
now converts into the shared one explicitly. Divergences that are genuinely
part of the published contract survive the refactor; only accidental ones are
removed.

`internal/coord` remains the destination, but the extraction is landing in
`package main` first. Moving files and deduplicating logic in one step would
produce a diff nobody can review; the package move is Phase 3's job.

Both surfaces become adapters: parse input, call the core, render output. The
CLI keeps human formatting and receipts; the RPC layer keeps envelopes and
receipts. Neither keeps a mutation closure.

Exit criteria: no lifecycle mutation logic appears in more than one file;
`serve.go` drops below 1,200 lines; the parity harness passes unchanged;
`internal/coord` reaches 85% coverage from in-process unit tests.

### Phase 2 — Declarative CLI surface

Replace the five parallel flag tables, the 130-line dispatch switch, and the
200-line hand-maintained usage string with one command registry: name, aliases,
positional arity, flag specs with arity and type, handler, and help text.

Handlers return `error` instead of calling `os.Exit`; `main` is the only place
that exits. This is what makes the CLI unit-testable in process and lets the
subprocess tests shrink to the few cases that genuinely need a real process
(exit codes, locking across processes, stdio servers).

Add a test asserting the registry and `docs/agent-protocol.md` describe the
same commands and flags, so documentation drift becomes a build failure.

Exit criteria: adding a flag requires editing exactly one place; `todo help` is
generated; root package coverage above 70%; root package test wall time under
20s.

### Phase 3 — Package layout and distribution

Move to a conventional layout: `cmd/todo/main.go` plus internal packages
(`coord`, `cli`, `protocol`, `mcp`, `rpc`). Keep `store`, `dag`, `lock`,
`fsutil`, and `conformance` where they are.

This closes a gap the README already admits: `go install
github.com/bharat94/terminal-todo/cmd/todo@latest` becomes supported. For a
tool whose adoption depends on an agent being able to install it in one line,
that is the highest-leverage distribution change available, ahead of any new
feature.

Also in scope: a checksum-verifying install script and a Homebrew tap formula,
both driven from the existing goreleaser artifacts. Release targets and
attestations are unchanged.

Exit criteria: `go install …/cmd/todo@latest` works from a clean module cache;
goreleaser produces byte-identical target sets; installation docs cover
`go install`, script, Homebrew, and manual archive.

### Phase 4 — Close the open conformance thread

The project's own graph carries two unfinished items, both stalled on a
decision rather than on code:

- Task 32, "Reconcile heartbeat and handoff conformance semantics", released
  with "Diagnosis complete; implementation awaits explicit approval".
- Task 37, "Run post-change sandboxed Codex v2 calibration", blocked awaiting
  approval for one 12-turn suite under a $5 ceiling.

The 2026-08-15 Codex calibration failed exactly two scenarios, and
`docs/evaluations/2026-08-15-codex-default.md` concluded both may be
benchmark-contract mismatches rather than unsafe agent behavior:

1. `heartbeat` accepts only `update` as the durable progress mutation, while
   the bundled skill explicitly permits a structured update *or* an audit log.
   The host used `log`, kept a valid lease, and was scored as
   `invalid_lease_mutation`.
2. `handoff` asserts a specific `extra.finding` argument shape, but compact
   reporting omitted call arguments and the workspace was deleted, so the run
   could not establish which shape the host actually used.

Work:

- Reconcile the written contract, the skill, and the assertions so all three
  say the same thing. Whichever way the heartbeat question is decided, the
  decision is recorded and, if it changes benchmark meaning, published as a
  new suite and scoring version rather than an edit to a frozen one.
- Fix the evidence gap directly: record redacted MCP call arguments in the
  trace so a failing assertion is diagnosable without retaining workspaces.
  This is a harness defect, and it is the reason one paid run produced an
  unexplainable failure.
- Only then put the calibration budget decision to the user. Re-running a
  paid suite against an unreconciled contract buys nothing.

Exit criteria: assertions, skill, and `docs/conformance.md` agree; a failing
assertion reports the observed argument shape; task 32 completes and task 37
carries an explicit approve-or-decline decision.

### Phase 5 — Coverage and hardening

- Coverage reporting in CI with a floor that ratchets, starting at the level
  Phase 2 achieves.
- `golangci-lint` alongside the existing `gofmt` and `go vet` gates.
- Fuzz targets for the JSON-RPC and MCP parameter decoders and the
  `todo://<alias>/<id>` URI parser — all three parse untrusted-ish input and
  none is fuzzed.
- Store fault injection: interrupted writes, partial temp files, and stale
  lock sidecars, extending the crash-recovery evidence that
  `docs/production-readiness.md` lists as a 1.0 gate.

## Sequencing and risk

Phase 0 is the only phase that must come first; it is the safety net for
everything after it. Phases 1 and 2 are strictly sequential. Phase 3 depends
on 1 and 2 landing, because moving files before the logic is deduplicated
multiplies the diff. Phase 4 is independent of 1-3 and can be interleaved.
Phase 5 follows 2.

Principal risk is a large-diff regression during Phase 1. Mitigations: one
operation per commit, the Phase 0 parity harness gating every commit, and the
existing cross-platform race matrix.

Secondary risk is scope creep into the conformance harness. Phase 4 is
deliberately narrow: reconcile the contract and fix the evidence gap. Redesign
is out of scope.

## What this plan deliberately does not do

It does not add features. The project's differentiator — a portable,
user-owned, lease-based coordination graph shared across vendors — is already
built and documented well beyond most projects of its age. The gap is not
capability. It is that the implementation carries two copies of its own
semantics, one of which is silently wrong on a documented contract, and that
the whole thing is hard to install and hard to test. Those are the things
worth fixing before the next feature.
