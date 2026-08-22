package cli

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bharat94/terminal-todo/store"
)

func cmdWatch(args []string) {
	ids := parseIDs(args)
	interval := parseWatchInterval(args)
	plain := hasFlag(args, "--plain")
	useAltScreen := !plain && isTerminal(os.Stdout)

	var restore func()
	if useAltScreen {
		restore = enterWatchScreen()
		defer restore()
		// Restore on SIGINT/SIGTERM so the terminal is not left in alt-screen.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			restore()
			// Re-raise for default handling after cleanup.
			p, _ := os.FindProcess(os.Getpid())
			if p != nil {
				_ = p.Signal(syscall.SIGINT)
			}
		}()
		defer signal.Stop(sigCh)
	}

	if len(ids) > 0 {
		watchTask(ids[0], interval, useAltScreen)
	} else {
		watchDashboard(interval, useAltScreen)
	}
}

func parseWatchInterval(args []string) time.Duration {
	pollStr := optionValue(args, "--poll")
	if pollStr == "" {
		return 1 * time.Second
	}
	d, err := time.ParseDuration(pollStr)
	if err != nil || d <= 0 {
		fail(ErrInvalidArgs, "--poll requires a positive duration (e.g. 500ms, 2s): %q", pollStr)
	}
	return d
}

func watchTask(id uint64, interval time.Duration, useAltScreen bool) {
	lastStatus := ""
	lastLogCount := 0
	var lastModified uint64
	var cursor uint64 // NextEventID-1

	for {
		s := loadStore()
		task, ok := s.GetTask(id)
		if !ok {
			// Task removed — show once then exit gracefully (better than StoreCorrupted).
			if useAltScreen {
				clearScreen()
			}
			fmt.Printf("Task %d not found (removed)\n", id)
			return
		}

		// Cursor skip: only re-render when store changed or task status/log changed.
		// Lease countdown must tick every interval, so never skip when task holds a lease.
		hasLease := task.Status == store.StatusInProgress && task.LeaseExpires != 0
		if !hasLease && lastModified != 0 && s.LastModified == lastModified {
			// Still check per-task change (log/status) in case LastModified granularity collides.
			statusStr := statusName(task.Status)
			if statusStr == lastStatus && len(task.Log) == lastLogCount {
				// Also check event cursor emptiness to avoid missing cross-task events in future dashboard.
				if cursor != 0 && len(s.EventsSince(cursor)) == 0 {
					time.Sleep(interval)
					continue
				}
			}
		}
		if s.NextEventID > 0 {
			cursor = s.NextEventID - 1
		}
		lastModified = s.LastModified

		statusStr := statusName(task.Status)
		hasLeaseForRender := useAltScreen && task.Status == store.StatusInProgress && task.LeaseExpires != 0
		changed := statusStr != lastStatus || len(task.Log) != lastLogCount || lastStatus == "" || hasLeaseForRender

		if changed {
			if useAltScreen {
				clearScreen()
			} else if lastStatus != "" {
				fmt.Println(strings.Repeat("─", 50))
			}
			fmt.Printf("Watching task %d — %s\n", id, task.Title)
			fmt.Println(strings.Repeat("─", 50))
			fmt.Printf("Status: %s\n", statusStr)
			if task.Owner != "" {
				lease := ""
				if task.LeaseExpires != 0 {
					remaining := time.Until(time.UnixMilli(int64(task.LeaseExpires)))
					if remaining > 0 {
						lease = fmt.Sprintf(" (lease %s)", formatWatchDuration(remaining))
					} else {
						lease = " (lease expired)"
					}
				}
				fmt.Printf("Owner:  %s%s\n", task.Owner, lease)
			}
			if len(task.Log) > 0 {
				fmt.Println("\nRecent log:")
				start := 0
				if len(task.Log) > 5 {
					start = len(task.Log) - 5
				}
				for _, entry := range task.Log[start:] {
					fmt.Printf("  [%s] %s: %s\n", formatTimestamp(entry.Timestamp), entry.Agent, entry.Message)
				}
			}
			if plainHint(useAltScreen) {
				fmt.Println("\n(q to quit, Ctrl-C to exit)")
			}
			lastStatus = statusStr
			lastLogCount = len(task.Log)
		}

		if task.Status == store.StatusCompleted {
			fmt.Println("\n✓ Task completed! (q to quit)")
			if !useAltScreen {
				return
			}
			// In alt-screen stay until user quits so they can see result.
			waitForQuit(interval)
			return
		}

		time.Sleep(interval)
	}
}

func watchDashboard(interval time.Duration, useAltScreen bool) {
	var lastModified uint64
	var cursor uint64

	for {
		s := loadStore()
		if lastModified != 0 && s.LastModified == lastModified {
			if cursor != 0 && len(s.EventsSince(cursor)) == 0 {
				time.Sleep(interval)
				continue
			}
		}
		if s.NextEventID > 0 {
			cursor = s.NextEventID - 1
		}
		lastModified = s.LastModified

		if useAltScreen {
			clearScreen()
		}
		cmdStatus([]string{})
		if useAltScreen {
			fmt.Println("\n" + colorGray + "watch: 1s poll · q/Ctrl-C to quit · --plain to disable alt-screen" + colorReset)
		}
		time.Sleep(interval)
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func enterWatchScreen() func() {
	fmt.Print("\033[?1049h") // alt screen
	fmt.Print("\033[?25l")   // hide cursor
	fmt.Print("\033[H")      // home
	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		fmt.Print("\033[?25h")  // show cursor
		fmt.Print("\033[?1049l") // leave alt screen
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func plainHint(useAltScreen bool) bool { return useAltScreen }

func formatWatchDuration(d time.Duration) string {
	if d < time.Second {
		return d.Truncate(time.Millisecond).String()
	}
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return d.String()
	}
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

func waitForQuit(interval time.Duration) {
	// Minimal quit wait when in alt-screen: poll for input would need raw mode.
	// For Phase 0 we just hold briefly then exit; Phase 1 will add raw 'q' handling via x/term.
	time.Sleep(2 * interval)
}
