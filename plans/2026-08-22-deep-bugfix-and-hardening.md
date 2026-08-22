# Deep bugfix and hardening

- Status: proposed
- Opened: 2026-08-22
- Baseline commit: `947e827`
- Owner: maintainer (via Muse Spark)

## Summary

Four parallel deep hunts (store/DAG/lock, allocation/lifecycle, protocol/surfaces, conformance/infra) found 38 distinct defects against `947e827`. Seven are critical data-integrity issues, six are high-severity security or crash-recovery flaws, and the remainder are parity, error-taxonomy, and harness gaps. The 2026-08-16 coordination-core refactor is otherwise complete (Phases 0-3, 5 done; Phase 4 v2 contract done, paid calibration pending). This plan finishes the refactor's open graph items, then hardens the core in three strictly sequenced phases that are each independently revertible and each gated by `make test-race` and the full CI matrix.

Boundary: most fixes are toward the documented contract in `docs/agent-protocol.md`. Three change observable wire behavior and are treated as compatibility-noted fixes toward that contract (see per-item notes): `complete` second call now `INVALID_TRANSITION` (was success), dependency-URI errors now `INVALID_ARGS`/`INVALID_PARAMS` (was `STORE_CORRUPTED`), and acquisition receipts are GC'd on `prune`/`rm` (was retained per `agent-protocol.md:934` — updated). Fingerprint canonicalization is deferred rather than changing stable hashes in this series.

## Evidence

All defects were verified by reading HEAD and, where noted, by exercising the existing tests:

- `store/store.go:121` migration 1 unconditionally resets `NextEventID=1` after it loads a store with `NextEventID=11` — IDs reuse, `EventsSince` pagination breaks.
- `store/store.go:260,340` `CleanExpiredLeases`/`HasExpiredLeases` use `< now` while `lease.go:24`/`handoff.go:18` use `<= now` and `coordination_errors.go:135` uses `> now` — at `LeaseExpires==now` a claim succeeds while the stale `InProgress` record survives without an event.
- `coordination_ops.go:227-239` `decomposeTask` mutates `s.Tasks` and `parent.Depends` before `DetectCycle`; on error the partial children survive; `store.Update:371-377` persists lease expirations even when `mutate` failed, making the partial decompose durable.
- `coordination_ops.go:69-94` `completeTask` lacks a `StatusCompleted` guard — second `complete` silently re-stamps `Completed` and emits a duplicate `EventTaskCompleted`.
- `allocation.go:313-353` replay returns a ghost task after `pruneCompletedTasks` or `RemoveTask` deletes it — `Acquisitions` never GCs.
- `lock/lock.go:27` sidecar `.lock` is per-inode; `os.Remove(tasks.bin.lock)` between holders creates a new inode and two concurrent `LOCK_EX` holders — last `Rename` wins, tasks lost. Reproduced with two `Open`+`Acquire(Write)` and an intervening `Remove`.
- `conformance/redact.go:106` misses `apiKey`/`sessionId`/`token`/`secret`/`password` camelCase — secrets survive redaction into retained reports.
- `projectclock/clock.go:19-31` silent fallback to `time.Now()` when `TERMINAL_TODO_CLOCK_FILE` is set but corrupt — deterministic harness becomes wall-clock, heartbeat `lease_after` assertion flaps.
- `cmd_add.go:46` repeatable `--tag` overwrites instead of accumulating — parity failure vs `todo.add` RPC.
- `coordination_ops.go:595` / `dag/dag.go:62` canonicalization gaps affect `diagnoseAllocation:198-205` and `newlyReadyAfter:550` cross-repo unblocked reporting.

Selected repro:

```bash
# migration clobber
python3 -c "import msgpack; ..." # craft v1 store with NextEventID=11, load via store.LoadCurrent, observe 1

# lease boundary
todo init && todo add "A" && todo claim 1 --as w1 --ttl 30m
# set clock file to exact expiry, CleanExpiredLeases returns 0 but claim by w2 succeeds

# decompose partial
todo add "parent" && todo add "child" && todo update 2 --add-dep todo://local/1
todo decompose 1 --as w1 --into '{"subtasks":[{"title":"c1"},{"title":"c2"}]}' # with cycle
ls .terminal-todo/tasks.bin # extra children visible after failure if lease expired during call
```

## Non-goals

- Changing DAG or lease semantics beyond repairing the documented contract.
- Introducing a network or daemon mode.
- Redesigning the conformance harness (Phase 4 already reconciled v2; this plan only fixes harness determinism and redaction).
- Publishing Homebrew tap or spending the $5 calibration budget — that remains the maintainer's explicit approval (Task 37).

## Sequencing and risk

Phase 1 must come first — it fixes durable store corruption. Phase 2 assumes Phase 1 `<=` lease semantics; Phase 3 harness fixes are independent of Phase 1/2 and can land in parallel. Principal risk is large-diff regression and a fingerprint/docs compatibility slip; mitigations are one fix per commit, the parity harness gating every commit, and `docs/compatibility.md` notes for every wire-visible change (#3, #8, #13, #21). Fingerprint canonicalization is deferred to avoid a hash-version bump in this series.

## Phase 1 — Store and lifecycle integrity (7 commits, must land first)

1. **Migrations never clobber live IDs.** `store/store.go:103-138` guard: `if s.NextEventID==0 && len(s.Events)>0 { s.NextEventID = maxEventID+1 } else if s.NextEventID==0 { s.NextEventID=1 }`; `if s.Acquisitions==nil { make }` only when nil (preserve non-empty map). Regression crafts v1 msgpack with `NextEventID=11`+10 events and `Acquisitions` with one receipt. Covers `C2,C3`.
2. **Lease expiry is `<= now` everywhere.** Introduce `leaseIsExpired(expiry, now uint64) bool { return expiry!=0 && expiry<=now }` in `store` and use (or mirror correctly) in `CleanExpiredLeases`, `HasExpiredLeases`, `requireLeaseAvailable` (via `!leaseIsExpired`), `renewLease`, `handoffTask`. Add boundary test at exact `UnixMilli`. Covers `H1`.
3. **Completing completed work is `INVALID_TRANSITION`.** Guard `completeTask` with `if task.Status==StatusCompleted { return errInvalidTransition }` (second `complete` returns exit 1 / `-32014`, 0 events, `Completed` timestamp unchanged). `Blocked` remains completable only after `unblock`; document. Compatibility note in `docs/compatibility.md`. Covers `completeTask` re-entry.
4. **Decompose is atomic (staged).** Compute canonical subtask caps, validate canonical cardinality, build projected DAG in-memory (no `s.Tasks` mutation), `DetectCycle`, then mutate `s.Tasks`/`NextID`/`parent.Depends`/`AddEvent`; do not suppress `store.Update` lease-reclaim persist — reclaim always persists (only stale-lease portion), decompose atomicity is local to `decomposeTask`. If staged validation fails, `s.Tasks`/`NextID`/`parent.Depends` unchanged (assert `len==before`). Covers decompose partial + `NextID` leak.
5. **Prune allocates and canonicalizes (split).** (5a) fresh slice + sort; (5b) canonicalize survivors via `canonicalDependency` (preserve raw only on parse error), `ParseLocalID` on canonical. Add legacy `["1","todo://local/01"]` prune test. Covers `H4`.
6. **Expired-lease events are deterministic.** Collect expired IDs, sort by `Task.ID` before `AddEvent(EventLeaseExpired…)`. Covers non-deterministic `map` iteration.
7. **Sidecar inode split-brain documented and best-effort mitigated (Unix).** Short-term: `Open` does bounded `fstat`/`stat` dev/ino verification after `Acquire` (max 3 retries, handle `ENOENT`); document as best-effort, not a close — concurrent hold on unlinked inode still wins. Long-term: 1.0 moves `flock` to `tasks.bin` itself; `docs/compatibility.md:46` notes filesystem requirement. Windows `lock_windows.go` unaffected. Add `TestMissingLockSidecarDoesNotCreateSplitBrain` with barrier.

Exit criteria: `go test ./... -race -count=1 -timeout 300s` green; fuzz seeds for `ParseTaskURI`/`tasks.bin` include migration cases; no new `store.StoreCorrupted` misclassification.

## Phase 2 — Parity, error taxonomy, and allocation correctness (8 commits)

8. **Ghost receipts GC on removal.** `RemoveTask`/`pruneCompletedTasks` delete `Acquisitions` whose `Task.ID` in removed set; replay that still references a pruned ID is now a miss (`replayed:false`, fresh allocation). Update `docs/agent-protocol.md:934` (“receipts for removed/pruned tasks are now GC’d”) and `docs/compatibility.md`; update `allocation_test.go:63` expectation. Covers ghost replay; fingerprint bump not needed.
9. **Dependency remove dedup canonical.** Deduplicate canonical `RemoveDeps` before `delete` loop (`seenRemove`).
10. **Acquire fingerprint canonical deferred.** Defer stable-hash change; if touched, version as `acquire.v2` with migration — not in this series. Keep `normalizePersistedValues` dedup/trim already done at call sites.
11. **CLI tags accumulate.** `cmd_add.go:46` `tags=append(tags, …)` then `normalizePersistedValues` dedupe; add `TestAddRepeatableTagParity`.
12. **Capability comma divergence: reject commas.** `validateCapabilities` rejects `strings.Contains(v, ",")` with `persistedInputFailure`; do not split in RPC path (splitting changes acceptor). Cover with `TestCapabilityCommaParity` (RPC with `","` → `INVALID_ARGS`).
13. **Dependency URI errors are `INVALID_ARGS`.** Wrap `ParseTaskURI`/`canonicalDependency` errors as `persistedInputFailure` so `fail(ErrInvalidArgs)` / `rpcInvalidParams` (or `rpcDependency`) not `StoreCorrupted`. Distinguish persisted corrupted edge (remains `STORE_CORRUPTED` on load) vs user-supplied URI. Covers `serve.go:777`/`coordination_ops.go:438`.
14. **MCP missing `INVALID_TRANSITION` summary; `status --all` and `heartbeat` alignment.** Add `rpcInvalidTransition: ErrInvalidTransition` to `mcp_summary.go:188`; `handleStatusAll` stays local-only on registry error (document as best-effort); make `handleHeartbeat` delegate to `rpcErrorFromDomain` (behavior-preserving refactor). Covers `mcp_summary`, `serve.go:1251` (registry error left as local-only per reviewer).
15. **`newlyReadyAfter` respects resolver (cross-repo).** Fix `serve.go:841` snapshot to `GetAllTasks` (or live `dependencyResolver()` inside `Update`) and handle `todo://alias/id` via `completedURIs` vs resolver; bounded test with linked-repo fixture.

Exit criteria: parity harness `TestSurfaceParity*` passes with cross-repo fixtures; MCP `tools/list` annotations still pinned; no `STORE_CORRUPTED` for URI errors.

## Phase 3 — Harness determinism, redaction, and infra gates (6 commits)

16. **Redaction covers camelCase and token families.** `camelToSnake(key)` inserts `_` before `[A-Z]` (unless preceded by `_`), `ToLower`, exact match `api_key, access_token, authorization, session_id, thread_id, token, secret, password`; cover nested objects and `EventText`. Add corpus with `{"sessionId":"s3cret","apiKey":"k"}`.
17. **Clock fallibility is explicit.** If `TERMINAL_TODO_CLOCK_FILE` is set but missing/corrupt, log to stderr and (when `TERMINAL_TODO_CONFORMANCE_WORKSPACE` set) fail fast rather than wall-clock. Add test.
18. **Harness durability (independent).** `conformance/clock.go:58 AdvanceClock` calls `fsutil.SyncDir` after `Rename`; `cmd_conformance_catalog.go:174` preserve `LastModified` across `Save`; `conformance/trace.go` reads under `RLock` (retry on `unexpected EOF`). Covers durability/nondeterminism/tear.
19. **Heartbeat alternative scoring requires post-checkpoint.** Unify `orderedAlternativeIndexes:554` to require first op after `checkpoint` (as `ordered_operations` does). Covers `cmd_conformance_assertions.go:543`. Deferred behind flag if scoring change widens gate.
20. **CI and quality gates alignment (chore, after functional).** `Makefile:30` lint delegates to `golangci-lint --timeout=5m`; `coverage.sh:51` floor comparison is trunc+`+0.5` rounding — keep but document and gate with `go test -count=1`; `govulncheck` pin `1.26.1` intentionally hardcoded (stdlib). Add fuzz seeds at bounds.
21. **Docs and capability drift.** `serve.go:683` capability `atomic_handoff` added to `docs/agent-protocol.md:922`; `--ready` no-op documented or removed; `--limit`/`page` parity documented. Covers doc drift.

Exit criteria: `make lint` == CI lint; coverage floors ratchet without rounding bypass; conformance determinism tests green with fixture clock file removed.

## Future plan (1.0 and beyond)

- **Lock primitive:** move `flock` to `tasks.bin` itself (no sidecar) once migration path for existing `.lock` files is scripted. Evaluate `fcntl` lease or `directory flock` for network mounts — cross-machine remains not-supported and must not imply consensus.
- **Distributed DMATO:** beyond shared filesystem, pursue event-sourced append log with per-task vector clocks; not required for 1.0 beta boundary.
- **Power-loss fault injection:** extend `store/fault_test.go` beyond interrupted write to exercise `Rename`+`SyncDir` power-loss via `libeatmydata` or `dm-flakey`.
- **Calibration:** after Phases 1-3, re-run one `todo conformance --run --suite v2 --host codex --json` under $5 ceiling, then update `docs/evaluations/2026-08-15-codex-default.md` to `n≥2` and close Tasks 32 and 37.
- **Input limits as ledger:** consider per-task byte ledger (log+extra+deps) to bound `tasks.bin` growth beyond per-field limits.

## Alternatives considered

- Fix everything in one commit: rejected — unreviewable and unbisectable.
- Rewrite lock to directory lock immediately: rejected — changes production safety boundary; shipped as Phase 1 mitigated loop plus documented follow-up.

## Checklist

- [ ] Phase 1 #1-7 landed with tests and green matrix
- [ ] Phase 2 #8-15 landed, parity harness green with cross-repo cases
- [ ] Phase 3 #16-21 landed, harness redaction/clock deterministic
- [ ] Close Task 32 (record reconciled skill/doc/assertion agreement) and request Task 37 approval
- [ ] Publish plan update and bump docs if any `agent-protocol.md` envelope changes
