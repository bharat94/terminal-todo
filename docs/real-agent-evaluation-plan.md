# Real-agent behavior evaluation plan

- Status: proposed local operating plan
- Last updated: 2026-08-15
- Primary benchmark: `terminal-todo-real-agent-v1`
- Primary scoring model: `terminal-todo-agent-conformance-v1`

## Purpose

This plan defines how to measure whether real coding agents use terminal-todo
safely, reliably, efficiently, and quietly. It turns the existing conformance
catalog into a repeatable evaluation program rather than treating one
successful model turn as proof of product quality.

The central rule is: grade observable behavior at the coordination boundary,
not an agent's explanation of what it believes it did. Every result must be
traceable to the MCP operation log, final persisted task state and audit
events, or actor-attributed assistant output.

No command in this document should be run against a paid host without an
explicitly approved budget and credentials intended for evaluation.

## Goals

1. Quantify end-to-end conformance for each supported host, model, host
   version, and integration version.
2. Measure run-to-run reliability, not just best-case capability.
3. Detect the safety failures that matter most: race-prone allocation,
   invalid lease mutation, fabricated work, lost handoff state, and abandoned
   ownership.
4. Separate product defects from host failures, authentication or approval
   problems, harness defects, and stochastic model behavior.
5. Track latency, model usage, visible coordination noise, and cost alongside
   correctness.
6. Determine whether the bundled skill improves behavior compared with
   reduced-instruction baselines.
7. Produce reproducible, privacy-reviewed evidence suitable for release
   decisions and longitudinal comparisons.

## Non-goals

- Proving the correctness of terminal-todo's storage, locking, or DAG
  implementation; unit, race, integration, and platform tests own that claim.
- Ranking general coding ability or comparing vendors outside the
  coordination workflow.
- Inspecting private reasoning, chain-of-thought, or undocumented host data.
- Running paid evaluations in ordinary pull-request CI.
- Claiming broad real-world adoption from synthetic fixtures alone.
- Treating an infrastructure skip as either a behavioral pass or failure.
- Optimizing prompts against a single model release until they overfit the
  nine published fixtures.

## Current evidence and limits

The repository already contains an executable, vendor-neutral catalog of nine
scenarios. A full run creates a separate disposable synthetic project for each
scenario and requires twelve isolated actor turns per host. It supports fresh
actor sessions, one explicit session resume, deterministic clock advancement,
and a two-actor contention barrier. The harness bounds and redacts host output
and grades authoritative operation traces, persisted state, audit events, and
final messages.

The historical lifecycle smoke baseline is useful only as a calibration
point:

| Host | Version | Result | Observed usage |
|---|---:|---|---|
| Codex | 0.144.5 | Lifecycle smoke passed, 100/100 | About 28 seconds; 148,463 input tokens, 128,512 cached input tokens, 532 output tokens |
| Claude Code | 2.1.215 | Infrastructure skip | Local client was unauthenticated; no behavioral score |

That smoke test was not the current nine-scenario suite, did not establish a
failure rate, and must not be described as full-catalog certification. No
current host/model pair has a repeated full-suite baseline yet.

## Questions and hypotheses

Each hypothesis has a decision rule so that the evaluation cannot be
reinterpreted after seeing results.

| ID | Question or hypothesis | Primary measure | Initial decision rule |
|---|---|---|---|
| H1 | A supported host/model can follow the complete coordination contract. | Capped suite score and level | Every release-gate run is `conformant` (at least 90/100). |
| H2 | Safety-critical behavior is dependable, not occasional. | Hard-gate failures | Zero hard-gate failures in release-gate runs. Any occurrence blocks certification and triggers triage. |
| H3 | Results are repeatable across fresh sessions. | Full-suite conformance rate | At least 90% of scored characterization runs are conformant; report a 95% Wilson interval. |
| H4 | Atomic contention never produces dual ownership or `next` plus `claim`. | `atomic_acquire` assertions | 100% pass in observed runs; one failure blocks certification. |
| H5 | The integration preserves ownership through renewal, recovery, handoff, and cleanup. | Four lifecycle criteria and associated gates | 100% pass in release-gate runs. |
| H6 | Routine coordination remains out of the user conversation. | `quiet_narration`, message words, protocol leak patterns | Scenario passes every release-gate run; median final response at most 40 words and no raw protocol leak. |
| H7 | The bundled skill materially improves correct behavior. | Paired difference between full integration and MCP-only ablation | At least +10 percentage points in conformance rate or a clearly documented ceiling effect with no safety regression. |
| H8 | Correctness does not depend on one host's default model alias. | Cross-model criterion rates | Every model claimed as supported independently meets H1-H6. Unpinned defaults are reported separately. |
| H9 | Evaluation cost and latency are operationally acceptable. | Tokens, estimated cost, wall time | Stay within the approved run budget and scenario timeouts; publish median and p95 rather than setting a product threshold before the first full baseline. |

H7 is an attribution study, not a requirement for the first certification. Its
threshold may be revised once both arms have at least five paired runs, but
the original and revised rules must both remain in the evaluation record.

## Benchmark contract

The primary benchmark is the checked-in manifest at
`conformance/scenarios/manifest.json`. Its nine behaviors are:

| Scenario | Behavior under test | Safety significance |
|---|---|---|
| `discovery` | Discover the project integration without terminal-todo being named in the prompt. | Measures onboarding and autonomous resumption. |
| `bootstrap` | Begin with one bounded brief instead of broad status/history reads. | Controls context growth and coordination noise. |
| `atomic_acquire` | Resolve two-worker contention through idempotent atomic acquisition. | Hard gates race-prone allocation and invalid ownership. |
| `heartbeat` | Renew a lease before post-checkpoint mutation. | Hard gates mutation without valid ownership. |
| `handoff` | Persist a material finding before release and consume it as a successor. | Hard gates lost handoff state. |
| `no_work` | Treat `NO_WORK` as structured terminal control flow. | Hard gates fabricated work and busy loops. |
| `lease_recovery` | Reacquire expired work as a successor without impersonation. | Hard gates invalid lease mutation. |
| `quiet_narration` | Keep bookkeeping and protocol payloads out of the final response. | Measures user-visible quality and leakage. |
| `cleanup` | Complete, block, or release before the session ends. | Hard gates abandoned ownership. |

The model awards 100 points across discovery (10), bounded startup (10),
atomic allocation (15), lease maintenance (10), durable handoff (15), no-work
control flow (10), lease recovery (15), quiet coordination (10), and cleanup
(5). A hard-gate failure caps the result at 49. Infrastructure failures are
unscored.

Catalog and scoring changes require a new suite or scoring-model version when
they change meaning. Published results must always name both IDs and the exact
Git commit.

## Evaluation matrix

Treat the host executable, model, integration, and environment as separate
variables. “Codex passed” is not precise enough.

### Required dimensions

| Dimension | Values | Recording rule |
|---|---|---|
| Host | Codex, Claude Code | Record exact executable version and path hash or installation source. |
| Model | Explicit stable model ID; host default alias | Prefer explicit IDs for comparisons. Evaluate defaults as a separate configuration because aliases can move. |
| Integration | Full project integration; MCP-only ablation | Full integration is the product arm. MCP-only needs harness support before attribution runs. |
| terminal-todo | Release candidate and latest certified release | Record version string, Git SHA, clean/dirty state, Go version, and binary SHA-256. |
| Platform | Maintainer's native macOS arm64 first; Linux amd64 for portability characterization | Record OS, architecture, filesystem, and whether execution is local or CI. Add Windows only when the selected host officially supports the tested non-interactive mode. |
| Credential context | Dedicated evaluation account or profile | Record a non-secret profile label only. Never store tokens or account identifiers. |
| Time | UTC start/end and backend status notes | Needed because a pinned client/model name does not pin a remote service implementation. |

### Initial matrix

Run the smallest matrix that answers the release question before expanding:

1. Codex current installed version × one explicit supported model × full
   integration.
2. Claude Code current installed version × one explicit supported model ×
   full integration, after authentication and project MCP approval are
   deliberately configured.
3. Each host's default model alias × full integration, labeled
   “moving-default observation,” not a reproducible model certification.
4. Full integration versus MCP-only ablation for the same host/model/version,
   after the ablation control exists.
5. The latest certified release versus the release candidate when a behavior
   or integration change could plausibly alter results.

Do not combine host or model results into one average. A configuration earns
its own certification or remains unscored.

## Baselines and controls

Use four baselines to determine where a regression belongs:

1. **Deterministic harness baseline.** The normal Go suite validates catalog
   loading, host argv construction, isolation, time control, concurrent
   orchestration, redaction, evidence normalization, assertions, and scoring.
   This must pass before any paid run.
2. **Historical lifecycle smoke.** The July 2026 Codex result checks that a
   real host could discover MCP, acquire, update, complete, and answer quietly.
   It is contextual evidence only, not a comparable full-suite score.
3. **Last-known-good full suite.** Once established, retain the latest passing
   report for each exact host/model/integration tuple. Compare candidate runs
   against it at scenario, assertion, operation, latency, and usage levels.
4. **Instruction ablation.** Run the same fixtures with project MCP available
   but without the bundled skill, except that `discovery` already intentionally
   omits preloading. This estimates what the integration instructions add.
   Randomize arm order and keep model, host, prompt, catalog, and binary fixed.

A CLI-only fallback arm is useful for product research but is not directly
comparable until the harness can authoritatively trace CLI operations. Keep it
outside certification scoring until it emits equivalent normalized evidence.

## Metrics

### Primary correctness and safety

- Capped and raw score per full suite.
- Conformance level per suite.
- Pass/fail per scenario, assertion, criterion, and hard gate.
- Full-suite conformance rate across repetitions.
- Hard-gate failure count and rate, both overall and by gate.
- Infrastructure success rate, reported separately from behavioral results.
- Exact final owner/status for every fixture task.
- Operation-order violations, duplicate acquisitions, retries, and unexpected
  mutations.

The capped score is a summary, not a substitute for the hard-gate table. A
configuration with any hard-gate failure is unsafe even if most criteria pass.

### Reliability and stability

- Per-scenario pass rate with a 95% Wilson confidence interval.
- Per-criterion pass rate with a 95% Wilson confidence interval.
- Median and interquartile range of raw and capped scores.
- Model/host version-to-version change in pass rate and score.
- Failure clustering by scenario, actor, turn, operation, and time of day.
- First-attempt result. Diagnostic reruns never replace the original trial.

### Efficiency

- End-to-end suite and per-scenario wall time; report median, p90, and p95.
- Host-reported input, cached-input, and output tokens when available.
- Estimated and invoiced cost using the price effective on the run date.
- Tool-call count by operation and actor.
- Acquisition retries, repeated broad reads, and unnecessary event/status
  calls.
- Captured bytes and output-limit failures.

Token fields are host-specific and may be absent. Missing usage is `unknown`,
never zero. Preserve the raw host-reported units and put normalized estimates
in a separate derived summary.

### User experience

- Final assistant-message count and word count.
- Raw command, request ID, lease, receipt, or schema leakage.
- Routine coordination narration classified by the deterministic patterns
  first and a blinded human review only for ambiguous cases.
- Meaningful outcome, blocker, or handoff clarity on a small reviewer rubric:
  accurate, concise, actionable (0 or 1 each).
- Host-rendered tool rows or expanded panels in an interactive observation
  study. Keep this separate from server text bytes and assistant narration,
  because terminal-todo cannot control host chrome.

### Operational health

- Authentication, approval, launch, timeout, output-limit, session-resume, and
  normalization failure counts.
- Disposable workspace cleanup success.
- Report redaction audit result.
- Host preflight availability and version-probe latency.

## Evidence and artifacts

### Evidence hierarchy

Use sources in this order when accounts disagree:

1. MCP-boundary operation trace, including actor and call order.
2. Final terminal-todo task store and audit events.
3. Harness clock and actor/session transitions.
4. Actor-attributed final assistant messages.
5. Bounded, redacted host stdout/stderr for diagnosing infrastructure and
   parser failures.
6. Human observation, only for host UI behavior that machine streams cannot
   represent.

An assistant sentence saying “completed” cannot override an in-progress task.
A host exit code cannot establish correct ownership without store evidence.

### Local artifact layout

Store raw evaluation material under the ignored project state directory, not
in Git:

```text
.terminal-todo/evaluations/
  2026-08-15T220000Z_codex_<model>_<sha>/
    manifest.json
    preflight.json
    run-001/
      report.json
      stderr.txt
      exit-code.txt
      checksums.txt
    summary.json
    summary.md
    triage/
```

The run manifest should contain:

- suite and scoring-model IDs;
- repository SHA and clean/dirty state;
- terminal-todo version and binary SHA-256;
- host name, exact version, requested model, and reported model;
- OS, architecture, filesystem type, Go version, locale, and timezone;
- integration arm, repetition number, randomized run-order position, UTC
  timestamps, command flags, timeout, and operator label;
- approved maximum turns and currency budget, without credentials; and
- SHA-256 checksums for every retained artifact.

Do not commit raw reports. A deliberately published aggregate must contain no
workspace paths, session IDs, prompts containing secrets, credentials,
account identifiers, or unreviewed host events. Even though the harness
redacts known fields, run an explicit privacy review before sharing.

## Experimental design

### Unit of analysis

One trial is one complete nine-scenario suite for one exact configuration.
Each scenario receives a fresh synthetic workspace; each actor receives a
fresh conversation unless the fixture explicitly resumes it. Do not split a
partially successful suite into independent “passes” when reporting the suite
conformance rate.

Scenario-level trials may be used for focused diagnosis or higher-powered
contention testing after a scenario-selection option exists. They do not
replace full-suite release runs.

### Repetitions

Use staged sample sizes because a full suite costs twelve model turns per
host:

| Stage | Repetitions per configuration | Purpose | Promotion rule |
|---|---:|---|---|
| Calibration | 1 | Validate credentials, approval, parsing, redaction, and budget telemetry. | Infrastructure succeeds and artifacts are complete; behavioral failure is retained and triaged. |
| Release gate | 5 | Catch common stochastic regressions without uncontrolled spend. | 5/5 scored runs conformant, zero hard-gate failures, and no unresolved harness ambiguity. |
| Characterization | 20 | Estimate scenario and criterion reliability and compare versions. | At least 90% conformant, zero hard-gate failures, and intervals published. |
| Safety stress | 30 focused scenario runs | Exercise `atomic_acquire`, lease recovery, and cleanup once `--scenario` exists. | 30/30 pass; any failure blocks the affected claim. |

Five successes are a release gate, not proof of a 95% success probability.
For context, even 20/20 successes still have uncertainty. Reliability claims
must include the interval and sample size. If a lower 95% Wilson bound of 90%
is required, plan for roughly 35 all-success trials rather than changing the
threshold after observing data.

### Randomization and independence

- Generate a run-order schedule before execution and preserve it in the
  manifest.
- Interleave comparison arms (ABBA or randomized blocks) rather than running
  all of one arm first.
- Use new disposable workspaces and host sessions for every trial.
- Keep fixture prompts, catalog commit, model ID, host version, timeout, and
  machine fixed within a block.
- Do not tune the skill or prompt between repetitions in a block.
- Space characterization runs across at least two time windows when practical
  to expose remote-service drift.
- Record host retries and backend outages. A retry after infrastructure
  failure is a new trial; preserve both records and link them.

### Statistical reporting

For every configuration report:

- `n_requested`, `n_started`, `n_scored`, and `n_infrastructure_failed`;
- conformance count/rate and two-sided 95% Wilson interval;
- scenario, criterion, and hard-gate counts/rates;
- median, IQR, minimum, and maximum score;
- median and p95 latency and model usage where available; and
- absolute paired difference for blocked comparisons.

Use paired McNemar results for binary full-integration versus ablation
comparisons when the two arms share a run block. Use paired bootstrap
intervals for score, latency, and token differences. With small samples,
publish the individual trial table and avoid “statistically significant”
language.

Never average an infrastructure skip into the behavioral denominator. Also
report infrastructure reliability, because a tool that cannot launch is not
operationally successful even though its model behavior is unknown.

## Cost, credential, and privacy controls

Before a real run, the operator must approve all of:

1. host(s), model(s), repetition count, and maximum possible turns;
2. a currency or token ceiling, including a stop threshold below the account
   hard limit;
3. the credential profile and confirmation that it is allowed for automated
   evaluation;
4. artifact retention duration and who may read raw reports; and
5. whether any interactive UI observation will be recorded.

Operational controls:

- Preflight first; it is local and consumes no model usage.
- Run one calibration suite per host before scheduling repetitions.
- Never use `--host all` when separate host budgets or credentials need
  independent stop decisions.
- Stop immediately on an unexpected hard-gate failure, redaction failure,
  runaway output, repeated infrastructure failure, or 80% of the approved
  budget—whichever comes first.
- Do not place API keys in command arguments, manifests, reports, logs, task
  metadata, or shell history.
- Use existing host authentication; do not copy or repurpose credentials.
- Keep `--keep-workspace` off except for a single approved diagnostic rerun.
- If a workspace is retained, inspect and delete it after extracting the
  minimum evidence needed for triage.
- Treat assistant messages and host event streams as potentially sensitive
  even though fixtures use synthetic project data.

The existing suite does not enforce a monetary cap. Until budget automation
lands, a human must calculate the maximum turn count (`12 × suites × hosts`),
watch provider usage, and launch only the approved batch size.

## Failure classification and triage

### Classification

Classify every unsuccessful trial into exactly one primary category:

| Category | Examples | Behavioral denominator? |
|---|---|---|
| Host infrastructure | Missing executable, unauthenticated account, MCP approval, launch error, remote outage | No |
| Harness infrastructure | Session extraction, timeout caused by runner, output cap, fixture materialization, normalization | No, until ruled behavioral |
| Agent behavior | Wrong operation, invalid ownership, fabricated task, lost handoff, noisy final answer | Yes |
| Product defect | Server accepted an invalid mutation, returned misleading structured data, corrupted state | Yes for end-to-end outcome; also open a core defect |
| Ambiguous | Evidence incomplete or conflicting | No certification; resolve before counting |

Timeouts need special care. A host that never becomes available is
infrastructure; an agent that repeatedly loops despite valid tool results is
behavioral. Preserve the trace before deciding.

### Triage procedure

1. Freeze the original artifact directory and compute checksums. Never
   overwrite the failed trial with a rerun.
2. Confirm exact host/model/integration versions, exit status, and whether the
   suite marked the trial scored.
3. Locate the first failing assertion and the earliest operation where actual
   behavior diverges from the fixture.
4. Reconcile MCP trace, final store, audit events, and assistant output.
5. Assign the failure to core, MCP surface, integration skill, host adapter,
   model behavior, evaluator, or environment.
6. Reproduce with deterministic tests or a fake host before spending another
   model turn when possible.
7. If a real-host rerun is necessary, authorize one diagnostic repetition
   with `--keep-workspace`; link it to the original and do not substitute it in
   the aggregate.
8. Add the smallest regression test that would have detected the defect.
9. Start a new evaluation block after a fix. Do not mix pre-fix and post-fix
   trials in one rate.

Any hard-gate failure triggers a stop-and-triage rule. Do not continue the
batch merely to improve the average.

## Phased rollout

### Phase 0: make measurement reproducible

- Keep deterministic conformance tests green on all supported platforms.
- Add machine-readable run manifests and an aggregate summarizer.
- Capture wall time and available host token/usage fields without weakening
  output bounds or redaction.
- Add report validation and a privacy/redaction audit.
- Establish dedicated evaluation credential profiles and budget approval.

Exit: a fake-host batch can produce the complete artifact layout and summary
without contacting a model.

### Phase 1: establish the first full-suite baseline

- Run local preflight for Codex and Claude Code.
- With explicit approval, run one calibration suite for one host at a time.
- Triage infrastructure and behavioral failures before repetition.
- Complete five release-gate runs for each configuration that will be called
  supported.

Exit: every claimed configuration meets the release gate, and its sanitized
summary identifies versions, sample size, costs, and limits.

### Phase 2: characterize reliability and attribution

- Expand stable configurations to 20 repetitions.
- Implement and run the MCP-only instruction ablation in randomized blocks.
- Add focused 30-run safety stress for concurrent acquire, lease recovery,
  and cleanup.
- Compare current and previous host/model/integration versions.

Exit: per-scenario confidence intervals exist, safety stress has no failures,
and integration impact is quantified.

### Phase 3: test ecological validity

The catalog intentionally avoids repository source. Add a separate, consented
field study using disposable copies of representative repositories:

- small single-language project, medium monorepo, and documentation-heavy
  project;
- one worker, concurrent workers, interruption/recovery, and cross-agent
  handoff sessions;
- realistic tasks whose product output is reviewed independently of
  coordination behavior; and
- no production secrets, live deployment credentials, or user data.

Measure task success, ownership violations, handoff utility, duplicate work,
time-to-first-meaningful-action, user narration, and operator intervention.
Do not merge field-study scores into the conformance score.

Exit: at least three repository shapes and two multi-agent workflows have
reviewed case reports with no unresolved ownership-safety failure.

### Phase 4: release and longitudinal operation

- Run five trials on integration, host-adapter, scenario, or protocol release
  candidates.
- Run a 20-trial characterization on major model/host changes or quarterly,
  subject to budget.
- Preserve last-known-good summaries and graph trends by exact configuration.
- Retire a certification when its host default or model alias changes; do not
  silently carry results forward.

## Acceptance gates

### Configuration certification

An exact host/model/integration configuration is certified only when:

- all deterministic local checks pass at the evaluated commit;
- preflight records the intended executable and exact host version;
- five of five independent full-suite release trials are scored and
  conformant;
- no hard gate fails;
- every lifecycle scenario (`atomic_acquire`, `heartbeat`, `handoff`,
  `no_work`, `lease_recovery`, and `cleanup`) passes every trial;
- no unresolved ambiguity, redaction defect, or artifact integrity issue
  remains; and
- actual usage remains inside the approved budget.

Label this “release-gate certified, n=5,” not “95% reliable.”

### Product release gate

- A documentation-only or core change with no integration behavior impact may
  rely on deterministic tests plus the last compatible certification.
- A skill, host adapter, conformance runner, MCP behavior, receipt, bootstrap,
  allocation, lease, handoff, or cleanup change requires fresh release-gate
  trials for affected configurations.
- A hard-gate failure blocks the release regardless of aggregate score.
- An unscored host cannot be advertised as behaviorally certified.
- A known infrastructure incompatibility must be documented with exact
  versions and a remediation path.

### Broader reliability claim

Use “at least 90% observed conformance” only after at least 20 scored trials
with at least 90% passing and zero hard-gate failures. Always publish the
Wilson interval. Stronger probability claims require a sample size chosen
from the desired lower confidence bound before testing.

## Operator runbook

The commands below create only local artifacts until the explicitly marked
real-agent step.

### 1. Freeze the candidate

```bash
git status --short
git rev-parse HEAD
go version
go test ./... -count=1
go test ./... -race -count=1 -timeout 5m
go vet ./...
go mod verify
mkdir -p tmp_test .terminal-todo/evaluations
go build -trimpath -o tmp_test/todo-eval .
shasum -a 256 tmp_test/todo-eval
```

Require a clean worktree for a publishable baseline. A dirty tree is allowed
only for local diagnosis and must be labeled with a diff checksum.

### 2. Preflight without model usage

```bash
run_id="$(date -u +%Y%m%dT%H%M%SZ)-preflight"
artifact_dir=".terminal-todo/evaluations/${run_id}"
mkdir -p "$artifact_dir"
tmp_test/todo-eval conformance --host all --json \
  >"$artifact_dir/preflight.json" \
  2>"$artifact_dir/preflight.stderr"
```

Review executable paths and versions. Resolve authentication and approval
deliberately in the host; never weaken an unrelated global security policy to
make the test pass.

### 3. Write the batch manifest and budget

Before launch, record the matrix row, repetitions, expected maximum turns,
timeout, artifact retention, and approved currency/token stop. For one full
suite, the current maximum is twelve model turns per host. Confirm the report
directory is ignored by Git.

### 4. Run one paid calibration suite — explicit approval required

The following command contacts the selected host and can incur cost:

```bash
run_id="$(date -u +%Y%m%dT%H%M%SZ)-codex-calibration"
artifact_dir=".terminal-todo/evaluations/${run_id}"
mkdir -p "$artifact_dir"
set +e
tmp_test/todo-eval conformance --run --host codex \
  --model '<explicit-model-id>' --json \
  >"$artifact_dir/report.json" \
  2>"$artifact_dir/stderr.txt"
exit_code=$?
set -e
printf '%s\n' "$exit_code" >"$artifact_dir/exit-code.txt"
shasum -a 256 \
  "$artifact_dir/report.json" \
  "$artifact_dir/stderr.txt" \
  "$artifact_dir/exit-code.txt" \
  >"$artifact_dir/checksums.txt"
```

Do not add `--include-events` or `--keep-workspace` for routine runs. Inspect
the compact report, all scenario statuses, infrastructure failures, criteria,
and hard gates before authorizing the next trial.

### 5. Repeat from a predeclared schedule

Run one suite per process and use a new artifact directory for every
repetition. Stop at the first hard-gate, privacy, budget, or repeated
infrastructure failure. Record failed launches and diagnostic reruns; do not
delete inconvenient data.

### 6. Summarize and review

Validate report schema, compute the metrics in this plan, perform the privacy
check, and write `summary.json` plus a human-readable `summary.md`. A second
reviewer should verify hard-gate counts and any manual narration labels before
publication.

### 7. Publish narrowly

Commit only a sanitized aggregate when it informs a release or compatibility
claim. Link the exact suite/scoring IDs, Git SHA, host/model versions, dates,
sample sizes, intervals, and known limitations. Keep raw host events local and
apply the retention policy.

## Automation backlog

### P0 — required before repeated paid evaluation

- [ ] Add `todo conformance --output <directory>` to atomically write a run
  manifest, report, stderr summary, exit status, and checksums.
- [ ] Extract host-reported token usage, cached tokens, model identity, and
  turn latency into optional versioned report fields.
- [ ] Add an offline aggregator that validates report schema and computes
  rates, Wilson intervals, score distributions, latency percentiles, and
  hard-gate tables.
- [ ] Add a batch budget file with maximum hosts, suites, turns, wall time,
  output bytes, and optional currency estimate; fail closed before launch.
- [ ] Add a fake-host batch mode that exercises manifests, repetitions,
  failures, summaries, and redaction without model calls.
- [ ] Add a report privacy audit that rejects unredacted workspace paths,
  session identifiers, configured secrets, and oversized raw events.

### P1 — required for attribution and stress testing

- [ ] Add `--scenario <id>` for focused diagnostic and safety-stress runs while
  preserving full-suite certification semantics.
- [ ] Add `--repeat <n>` with one fresh workspace and session set per trial,
  explicit stop rules, and partial-batch preservation.
- [ ] Add a declared integration arm such as `project_integration` versus
  `mcp_only`; record it in the report and prevent accidental score mixing.
- [ ] Generate and persist randomized/block run schedules before model calls.
- [ ] Add comparison tooling for current versus last-known-good reports at
  assertion and operation level.
- [ ] Emit process and scenario monotonic durations in the report.

### P2 — broader behavior and operations

- [ ] Add opt-in interactive UI observation forms for host tool-row counts;
  never infer host chrome from MCP payload bytes.
- [ ] Define a separate field-study schema for realistic repository tasks and
  reviewer outcomes.
- [ ] Add trend dashboards sourced only from sanitized aggregates.
- [ ] Add an explicit certification index keyed by suite, scoring model,
  terminal-todo commit, host version, model ID, platform, and integration arm.
- [ ] Document expiration rules for moving model aliases and superseded host
  releases.

## Concrete first evaluation backlog

1. Implement the P0 artifact manifest, offline summary, usage capture, privacy
   audit, and budget guard using fake-host fixtures.
2. Add tests proving an interrupted batch keeps every completed trial and
   never counts infrastructure failures as behavioral failures.
3. Run local preflight and record current Codex and Claude Code versions.
4. Obtain explicit budget approval for two calibration suites (24 maximum
   actor turns total), one host at a time.
5. Run Codex calibration; triage and decide whether to authorize its remaining
   four release-gate trials.
6. Authenticate and approve Claude Code in a dedicated evaluation context;
   run its calibration and make the same decision.
7. Publish the first sanitized n=5 full-suite summary per passing
   host/model/configuration.
8. Implement P1 scenario selection, repetition, integration ablation, and
   comparison tooling.
9. Run focused 30-trial safety stress and a 20-trial characterization only
   after reviewing calibration cost.
10. Design three consented field-study cases without changing the conformance
    score.

## Deliverables checklist

An evaluation cycle is complete only when it produces:

- [ ] predeclared matrix, schedule, hypotheses, and budget;
- [ ] exact build and environment manifest;
- [ ] immutable per-trial compact reports and checksums;
- [ ] separate behavioral and infrastructure denominators;
- [ ] scenario, criterion, hard-gate, latency, usage, and cost tables;
- [ ] confidence intervals and individual results for small samples;
- [ ] triage records for every unsuccessful or ambiguous trial;
- [ ] privacy review and retention decision;
- [ ] sanitized human-readable summary with precise claim language; and
- [ ] backlog updates based on observed failures, without silently changing
  the benchmark used for the completed block.

## Decision record template

```text
Evaluation ID:
Repository SHA / version:
Suite ID / scoring model ID:
Host / host version:
Requested model / reported model:
Integration arm:
Platform:
UTC window:
Approved suites / turns / budget:
Started / scored / infrastructure failed:
Conformant count and 95% Wilson interval:
Median [IQR] capped score:
Hard-gate failures:
Median / p95 duration:
Input / cached-input / output tokens:
Estimated cost and pricing date:
Privacy review:
Decision: certify | provisional | reject | unscored
Scope and expiration of decision:
Known limitations:
Artifact checksum manifest:
Reviewer:
```

This template is intentionally configuration-specific. Combining different
hosts, model IDs, integration arms, or suite versions invalidates the
certification decision.
