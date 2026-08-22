# Watch polish — quit and width

- Status: complete
- Opened: 2026-08-22
- Baseline: `65a2bde`
- Completed: 2026-08-22 — `6690393` (2 commits)
- Owner: maintainer

## Summary

Watch upgrade (flicker-free, rich, filtered, --all) is complete, but two polish gaps remain from the original design: no single-key `q` quit (requires Ctrl-C) and fixed 36-col truncation ignores terminal width. Both are cheap and keep the polling/alt-screen model.

## Evidence

- `cmd_watch.go:596 waitForQuit` sleeps 2*interval after completed, no raw input. User must Ctrl-C; `q` does nothing.
- `truncateWatchTitle(s,36)` and `%-36s` hard-coded, overflows on 80-col or wastes 200-col. No `COLUMNS` or winsize.

## Design

Stay in `internal/cli/cmd_watch.go` with `golang.org/x/term` (already `x/sys` present). No BubbleTea yet.

- **Quit:** when `useAltScreen` && `isTerminal(os.Stdin)`, `term.MakeRaw(int(os.Stdin.Fd()))`, restore on deferred. Goroutine reads 1 byte via `os.Stdin.Read`, closes `quit` on `q`/`Q`/`\x03` (Ctrl-C). Main loops select on `quit` via non-blocking channel check each tick (or use `time.After` + `select`). Plain/piped: no raw, Ctrl-C via signal handler already covers.
- **Width:** `term.GetSize(int(os.Stdout.Fd()))` when tty, fallback `COLUMNS` env or 80. Compute `avail = width - prefixLen` for title truncation. Keep min 20, max 60 for dashboard; task view uses `width-20`. No responsive reflow needed, just per-render truncation.

## Phases

**1. Raw `q` quit (commit 1)** — add `x/term`, `setupQuitChan`, integrate into `watchTask`/`watchDashboard` loops, `waitForQuit` becomes `waitForQuitWithChan`.

**2. Width-aware truncation (commit 2)** — `watchWidth()` helper, `truncateWatchTitle` uses dynamic max, update all `%-36s` to computed.

Exit: `q` quits in <200ms in alt-screen, `COLUMNS=120` shows longer titles, `go test -count=1` green.
