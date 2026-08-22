package cli

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bharat94/terminal-todo/dag"
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
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			restore()
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
	var cursor uint64

	for {
		s := loadStore()
		task, ok := s.GetTask(id)
		if !ok {
			if useAltScreen {
				clearScreen()
			}
			fmt.Printf("Task %d not found (removed)\n", id)
			return
		}

		hasLease := task.Status == store.StatusInProgress && task.LeaseExpires != 0
		if !hasLease && lastModified != 0 && s.LastModified == lastModified {
			statusStr := statusName(task.Status)
			if statusStr == lastStatus && len(task.Log) == lastLogCount {
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
		hasLeaseForRender := useAltScreen && hasLease
		changed := statusStr != lastStatus || len(task.Log) != lastLogCount || lastStatus == "" || hasLeaseForRender

		if changed {
			if useAltScreen {
				clearScreen()
			} else if lastStatus != "" {
				fmt.Println(strings.Repeat("─", 50))
			}
			renderWatchTask(s, task, statusStr, useAltScreen)
			lastStatus = statusStr
			lastLogCount = len(task.Log)
		}

		if task.Status == store.StatusCompleted {
			fmt.Println("\n✓ Task completed! (q to quit)")
			if !useAltScreen {
				return
			}
			waitForQuit(interval)
			return
		}

		time.Sleep(interval)
	}
}

func renderWatchTask(s *store.TaskStore, task *store.Task, statusStr string, useAltScreen bool) {
	fmt.Printf("Watching task %d — %s\n", task.ID, task.Title)
	fmt.Println(strings.Repeat("─", 50))
	// Status line with priority/caps/tags/lineage
	meta := fmt.Sprintf("Status: %s", statusStr)
	if task.Priority != 0 {
		meta += fmt.Sprintf("  Priority: %.2f", task.Priority)
	}
	if len(task.Capabilities) > 0 {
		meta += fmt.Sprintf("  Caps: %s", strings.Join(task.Capabilities, ","))
	}
	fmt.Println(meta)
	if len(task.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(task.Tags, ", "))
	}
	if task.Lineage != "" {
		fmt.Printf("Lineage: %s\n", task.Lineage)
	}
	fmt.Printf("Created: %s\n", formatTimestamp(task.Created))
	if task.Completed != 0 {
		fmt.Printf("Completed: %s\n", formatTimestamp(task.Completed))
	}
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
	if task.RetryCount > 0 {
		fmt.Printf("Retries: %d\n", task.RetryCount)
	}
	if task.LastError != "" {
		fmt.Printf("Last error: %s\n", task.LastError)
	}
	if task.BlockReason != "" {
		fmt.Printf("Blocked: %s\n", task.BlockReason)
	}
	// Depends
	if len(task.Depends) > 0 {
		deps := make([]string, 0, len(task.Depends))
		for _, dep := range task.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local {
				if t, ok := s.GetTask(depID); ok {
					deps = append(deps, fmt.Sprintf("%d (%s)", depID, t.Title))
				} else {
					deps = append(deps, fmt.Sprintf("%d [missing]", depID))
				}
			} else {
				deps = append(deps, dep)
			}
		}
		fmt.Printf("Depends: %s\n", strings.Join(deps, ", "))
	}
	// Dependents
	var dependents []string
	for _, t := range s.Tasks {
		for _, dep := range t.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local && depID == task.ID {
				dependents = append(dependents, fmt.Sprintf("%d (%s)", t.ID, t.Title))
				break
			}
		}
	}
	if len(dependents) > 0 {
		sort.Strings(dependents)
		fmt.Printf("Dependents: %s\n", strings.Join(dependents, ", "))
	}
	// Extra
	if len(task.Extra) > 0 {
		fmt.Println("Extra:")
		keys := make([]string, 0, len(task.Extra))
		for k := range task.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s: %s\n", k, task.Extra[k])
		}
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
	// Events tail for this task
	events := s.EventsSince(0)
	var taskEvents []store.Event
	for i := len(events) - 1; i >= 0 && len(taskEvents) < 5; i-- {
		if events[i].TaskID == task.ID {
			taskEvents = append([]store.Event{events[i]}, taskEvents...)
		}
	}
	if len(taskEvents) > 0 {
		fmt.Println("\nRecent events:")
		for _, e := range taskEvents {
			fmt.Printf("  #%d %s %s by %s\n", e.ID, formatTimestamp(e.Timestamp), e.Type, e.Actor)
		}
	}
	if plainHint(useAltScreen) {
		fmt.Println("\n(q to quit, Ctrl-C to exit)")
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
	fmt.Print("\033[?1049h")
	fmt.Print("\033[?25l")
	fmt.Print("\033[H")
	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		fmt.Print("\033[?25h")
		fmt.Print("\033[?1049l")
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
	time.Sleep(2 * interval)
}
