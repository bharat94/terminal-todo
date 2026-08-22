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
		hasLease := hasActiveLease(s)
		// Lease countdown must tick in alt-screen even without store change.
		if !hasLease || !useAltScreen {
			if lastModified != 0 && s.LastModified == lastModified {
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

		if useAltScreen {
			clearScreen()
		}
		renderWatchDashboard(s, useAltScreen)
		time.Sleep(interval)
	}
}

func hasActiveLease(s *store.TaskStore) bool {
	for _, t := range s.Tasks {
		if t.Status == store.StatusInProgress && t.LeaseExpires != 0 {
			return true
		}
	}
	return false
}

func renderWatchDashboard(s *store.TaskStore, useAltScreen bool) {
	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	grouped := map[store.TaskStatus][]*store.Task{
		store.StatusInProgress: {},
		store.StatusPending:    {},
		store.StatusBlocked:    {},
		store.StatusCompleted:  {},
	}
	for _, t := range tasks {
		grouped[t.Status] = append(grouped[t.Status], t)
	}
	resolver := dependencyResolver()
	ready := rankedReadyTasks(s, resolver, nil, false)
	diagnostics := diagnoseAllocation(s, resolver, "", nil, false, 0)

	// Header
	now := time.Now().Format("15:04:05")
	fmt.Printf("%s◉ terminal-todo — live dashboard%s  %s%s%s\n", colorCyan, colorReset, colorGray, now, colorReset)
	// Summary
	total := len(tasks)
	if total > 0 {
		summary := ""
		for _, st := range []store.TaskStatus{store.StatusInProgress, store.StatusPending, store.StatusBlocked, store.StatusCompleted} {
			count := len(grouped[st])
			if count == 0 {
				continue
			}
			if summary != "" {
				summary += ", "
			}
			summary += fmt.Sprintf("%d %s", count, statusName(st))
		}
		fmt.Printf("%sTasks: %s%s", colorCyan, summary, colorReset)
		if len(ready) > 0 {
			fmt.Printf("  %sReady: %d%s", colorGreen, len(ready), colorReset)
		} else if total > 0 {
			fmt.Printf("  %sReady: 0 (%s)%s", colorGray, allocationReasonMessage(diagnostics.Reason), colorReset)
		}
		fmt.Println()
		// Progress bar
		completed := len(grouped[store.StatusCompleted])
		if total > 0 {
			pct := float64(completed) / float64(total) * 100
			fmt.Printf("%sProgress: %d/%d %.0f%% %s", colorGray, completed, total, pct, colorReset)
			barW := 20
			filled := int(float64(barW) * float64(completed) / float64(total))
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
			fmt.Printf(" %s%s%s\n", colorGreen, bar, colorReset)
		}
	} else {
		fmt.Printf("%sNo tasks yet — add one with: todo add \"Title\"%s\n", colorGray, colorReset)
	}

	// Ready queue (top 5)
	if len(ready) > 0 {
		fmt.Printf("\n%s▶ Ready queue%s (priority → id)\n", colorGreen, colorReset)
		limit := 5
		if len(ready) < limit {
			limit = len(ready)
		}
		for i := 0; i < limit; i++ {
			t := ready[i]
			caps := ""
			if len(t.Capabilities) > 0 {
				caps = fmt.Sprintf(" [%s]", strings.Join(t.Capabilities, ","))
			}
			tags := ""
			if len(t.Tags) > 0 {
				tags = fmt.Sprintf(" %s[%s]%s", colorGray, strings.Join(t.Tags, " "), colorReset)
			}
			fmt.Printf("  %s●%s %d %-36s prio %.2f%s%s\n", colorGreen, colorReset, t.ID, truncateWatchTitle(t.Title, 36), t.Priority, caps, tags)
		}
		if len(ready) > limit {
			fmt.Printf("  %s… and %d more%s\n", colorGray, len(ready)-limit, colorReset)
		}
	}

	// In Progress with lease countdown
	if len(grouped[store.StatusInProgress]) > 0 {
		fmt.Printf("\n%s◐ In Progress%s\n", colorYellow, colorReset)
		for _, t := range grouped[store.StatusInProgress] {
			lease := ""
			if t.LeaseExpires != 0 {
				if useAltScreen {
					remaining := time.Until(time.UnixMilli(int64(t.LeaseExpires)))
					if remaining > 0 {
						lease = fmt.Sprintf(" lease %s", formatWatchDuration(remaining))
					} else {
						lease = " lease expired"
					}
				} else {
					lease = fmt.Sprintf(" lease %s", formatTimestamp(t.LeaseExpires))
				}
			}
			caps := ""
			if len(t.Capabilities) > 0 {
				caps = fmt.Sprintf(" [%s]", strings.Join(t.Capabilities, ","))
			}
			fmt.Printf("  %s◐%s %d %-36s %s%s%s%s\n", colorYellow, colorReset, t.ID, truncateWatchTitle(t.Title, 36), colorYellow+t.Owner+colorReset, colorGray+lease+colorReset, caps, tagStrWatch(t.Tags))
		}
	}
	// Pending — split ready vs waiting
	if len(grouped[store.StatusPending]) > 0 {
		// Figure which pending are actually ready vs dependency blocked.
		readySet := make(map[uint64]struct{}, len(ready))
		for _, t := range ready {
			readySet[t.ID] = struct{}{}
		}
		var pendingReady []*store.Task
		var pendingWaiting []*store.Task
		for _, t := range grouped[store.StatusPending] {
			if _, ok := readySet[t.ID]; ok {
				pendingReady = append(pendingReady, t)
			} else {
				pendingWaiting = append(pendingWaiting, t)
			}
		}
		if len(pendingReady) > 0 {
			fmt.Printf("\n%s○ Pending — ready%s\n", colorCyan, colorReset)
			for _, t := range pendingReady {
				fmt.Printf("   ○ %d %-36s prio %.2f%s%s\n", t.ID, truncateWatchTitle(t.Title, 36), t.Priority, capsStrWatch(t.Capabilities), tagStrWatch(t.Tags))
			}
		}
		if len(pendingWaiting) > 0 {
			fmt.Printf("\n%s○ Pending — waiting%s\n", colorGray, colorReset)
			for _, t := range pendingWaiting {
				blockers := dependencyBlockersForWatch(t, s, resolver)
				depStr := ""
				if len(blockers) > 0 {
					depStr = fmt.Sprintf(" → %s", strings.Join(blockers, ", "))
				}
				fmt.Printf("   ○ %d %-30s%s%s%s\n", t.ID, truncateWatchTitle(t.Title, 30), colorGray+depStr+colorReset, capsStrWatch(t.Capabilities), tagStrWatch(t.Tags))
			}
		}
	}
	if len(grouped[store.StatusBlocked]) > 0 {
		fmt.Printf("\n%s⊗ Blocked%s\n", colorRed, colorReset)
		for _, t := range grouped[store.StatusBlocked] {
			reason := t.BlockReason
			if reason == "" {
				reason = "blocked"
			}
			fmt.Printf("  %s⊗%s %d %-36s %s%s\n", colorRed, colorReset, t.ID, truncateWatchTitle(t.Title, 36), colorRed+reason+colorReset, tagStrWatch(t.Tags))
		}
	}
	if len(grouped[store.StatusCompleted]) > 0 {
		// Show last 5 completed to keep dashboard compact; full list via `todo status`.
		fmt.Printf("\n%s● Completed%s", colorGreen, colorReset)
		if len(grouped[store.StatusCompleted]) > 5 {
			fmt.Printf(" %s(last 5 of %d)%s", colorGray, len(grouped[store.StatusCompleted]), colorReset)
		}
		fmt.Println()
		start := 0
		if len(grouped[store.StatusCompleted]) > 5 {
			start = len(grouped[store.StatusCompleted]) - 5
		}
		for _, t := range grouped[store.StatusCompleted][start:] {
			fmt.Printf("  %s●%s %d %-36s%s\n", colorGreen, colorReset, t.ID, truncateWatchTitle(t.Title, 36), tagStrWatch(t.Tags))
		}
	}
	// Recent events tail
	events := s.EventsSince(0)
	if len(events) > 0 {
		fmt.Printf("\n%sRecent events%s\n", colorCyan, colorReset)
		start := 0
		if len(events) > 5 {
			start = len(events) - 5
		}
		for _, e := range events[start:] {
			actor := e.Actor
			if actor == "" {
				actor = "—"
			}
			title := ""
			if t, ok := s.GetTask(e.TaskID); ok {
				title = truncateWatchTitle(t.Title, 28)
			}
			fmt.Printf("  #%d %s %s #%d %s by %s\n", e.ID, formatTimestamp(e.Timestamp), e.Type, e.TaskID, title, actor)
		}
	}
	if useAltScreen {
		fmt.Printf("\n%s watch: 1s poll · q/Ctrl-C to quit · --plain to disable alt-screen · `todo help watch`%s\n", colorGray, colorReset)
	}
}

func truncateWatchTitle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func tagStrWatch(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return fmt.Sprintf(" %s[%s]%s", colorGray, strings.Join(tags, " "), colorReset)
}

func capsStrWatch(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	return fmt.Sprintf(" [%s]", strings.Join(caps, ","))
}

func dependencyBlockersForWatch(task *store.Task, s *store.TaskStore, resolver dag.DependencyResolver) []string {
	var blockers []string
	for _, dep := range task.Depends {
		depID, local := dag.ParseLocalID(dep)
		if local {
			if t, ok := s.GetTask(depID); ok {
				if t.Status != store.StatusCompleted {
					blockers = append(blockers, fmt.Sprintf("%d", depID))
				}
			} else {
				blockers = append(blockers, fmt.Sprintf("%d?", depID))
			}
		} else {
			if !resolver(dep) {
				blockers = append(blockers, dep)
			}
		}
	}
	return blockers
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
