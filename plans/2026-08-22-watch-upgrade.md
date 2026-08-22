# Watch upgrade — live dashboard

- Status: proposed
- Opened: 2026-08-22
- Baseline: `43c2215`
- Owner: maintainer

## Summary

`todo watch` `internal/cli/cmd_watch.go:11` is a 83-line poll loop: `watchTask` reprints title/status/owner/5 logs on change else sleeps 2s; `watchDashboard` is `clearScreen(); cmdStatus([])` `cmd_status.go:23` every 2s with `\033[H\033[2J` flicker. No lease TTL, no caps/priority/deps/extra, no events tail, no ready queue, no keyboard quit, no alt-screen. It squanders the event log `store/store.go:46` and `allocation.go:diagnoseAllocation` that already exist.

Upgrade `watch` into a useful live dashboard for humans without adding a daemon or web. Stay polling (filesystem trust boundary `docs/design.md:12`), but make it efficient (event cursor `store/store.go:222 EventsSince`), flicker-free, and rich.

Non-goals: full BubbleTea TUI, web UI, fsnotify/inotify, network.

## Evidence

```bash
todo init && todo add "A" --priority 0.9 --caps go && todo add "B" --after 1
todo watch           # flickers full screen, truncates title 40 cols cmd_status.go:123, no leases/events
todo watch 1 --poll 100ms  # sleeps fixed 2s default, no countdown, exits only on completed
```

- `cmd_watch.go:73 watchDashboard` calls `cmdStatus` which recomputes groups each tick, no diff.
- `cmd_status.go:120` shows `In Progress/Pending/Blocked/Completed` but no `Ready` (`rankedReadyTasks` `allocation.go:128`) or lease expiry.
- `store/store.go:268 leaseIsExpired <= now`, but watch never shows `LeaseExpires` `store/store.go:69`.

## Design

Keep one binary, one command. Two modes but shared renderer.

- **Transport:** polling `store.LoadCurrent` (reclaims leases) is correct. Add cursor `lastEventID = s.NextEventID-1` and skip render if `LastModified` unchanged and `EventsSince(lastEventID)` empty. Default poll 500ms (from 2s), `--poll` stays, add `--plain` (no ANSI) for tests/pipes.
- **Screen:** alt-screen `\033[?1049h` / `\033[?1049l` + hide cursor `\033[?25l/h`, restore on `SIGINT/SIGTERM` and `q`/`Ctrl-C`. Fall back to current `clearScreen` when `!isTerminal` or `--plain`.
- **Layout `watch` (dashboard):** 4 sections, single column (terminal width aware via `stty size` fallback 80):
  1. Header: `Tasks: X pending, Y in_progress...` + `Ready: N` from `diagnoseAllocation` + `caps` demand.
  2. Ready queue (top 5 `rankedReadyTasks`) with priority.
  3. In Progress with owner + `TTL 12m3s` countdown (live per tick, no store write).
  4. Blocked (with block_reason) + Recent events tail (5, from `EventsSince`).
  Truncate/wrap respecting width, keep colors `cmd_status.go:13`.
- **Layout `watch <id>`:** title/priority/caps/tags/lineage + status+owner+lease countdown + deps `depends`/`dependents` + `extra` + `block_reason`/`last_error`/`retry_count` + log tail 5 + events for task tail 5. Exit on `completed` or `q`.
- **Input:** single-char non-blocking read via `golang.org/x/term` `MakeRaw` (already indirect via `x/sys`). If not TTY, remain poll-only. No new deps if we avoid `term`; use `os.Stdin` raw via `syscall` fallback — prefer `x/term` (already `x/sys` present).

Invariant: watch is read-mostly, may persist lease reclaim via `LoadCurrent` — document.

## Phases (one diff each, independently revertible)

**Phase 0 — Foundation: flicker-free polling (commit 1)**
- Alt-screen + cursor hide/restore, `defer restore()`, signal handler.
- Event-cursor skip: `if s.LastModified==prev && len(s.EventsSince(cursor))==0 { sleep; continue }`
- `--plain` flag, `--poll` validate `>0` (already `cmd_watch.go:14` but silent ignore — now `fail` on bad).
- Tests: `watch --plain` exits on signal, `--poll bad` errors.

**Phase 1 — Rich `watch <id>` (commit 2)**
- Render deps, caps, priority, lease countdown, extra, block_reason.
- Live countdown without store write (compute `time.Until(lease)` each tick).
- Fix: handle deleted task (show `removed` then exit, not `fail` crash).

**Phase 2 — Rich dashboard (commit 3)**
- Ready queue + blocked + events tail panels, `Ready: all dependencies completed` reason.
- Width-aware truncation, keep existing `statusIcon`/`tagStr`.

**Phase 3 — Polish & parity (commit 4)**
- `watch --all` (linked repos `cmd_status.go:168`), `--tag`/`--as` filtering like `status`.
- Help text, docs `README.md:638` line.

Each commit: `go test ./... -count=1` + manual `todo watch` on fixture.

## Alternatives

- BubbleTea TUI: rejected for now — wants new dep, rewrites status. Watch upgrade is incremental; TUI can build on it later as `todo tui`.
- Web UI: rejected — past trust boundary, needs daemon.
- fsnotify: rejected — advisory locks require shared FS polling; notify misses lease reclaim.

## Exit criteria

- `todo watch` no flicker, `q` quits, lease countdown ticks.
- `todo watch <id>` shows deps/caps/priority/extra/lease.
- `make test -count=1` green, no new `store.StoreCorrupted` cases.
