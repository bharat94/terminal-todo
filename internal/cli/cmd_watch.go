package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
	"golang.org/x/term"
)

func cmdWatch(args []string) {
	ids := parseIDs(args)
	interval := parseWatchInterval(args)
	plain := hasFlag(args, "--plain")
	filterTag := optionValue(args, "--tag")
	filterAs := optionValue(args, "--as")
	filterAll := hasFlag(args, "--all")
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

	quit, restoreRaw := watchQuitChannel(useAltScreen)
	if restoreRaw != nil {
		defer restoreRaw()
	}

	if len(ids) > 0 {
		watchTask(ids[0], interval, useAltScreen, quit)
	} else {
		watchDashboard(interval, useAltScreen, filterTag, filterAs, filterAll, quit)
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

func watchTask(id uint64, interval time.Duration, useAltScreen bool, quit <-chan struct{}) {
	lastStatus := ""
	lastLogCount := 0
	var lastModified uint64
	var cursor uint64

	for {
		select {
		case <-quit:
			return
		default:
		}
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
					if sleepOrQuit(interval, quit) {
						return
					}
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
			if waitForQuitQuit(interval, quit) {
				return
			}
			return
		}

		if sleepOrQuit(interval, quit) {
			return
		}
	}
}

func renderWatchTask(s *store.TaskStore, task *store.Task, statusStr string, useAltScreen bool) {
	w := watchWidth()
	titleMax := w - 22
	if titleMax < 20 {
		titleMax = 20
	}
	if titleMax > 80 {
		titleMax = 80
	}
	fmt.Printf("Watching task %d — %s\n", task.ID, truncateWatchTitle(task.Title, titleMax))
	fmt.Println(strings.Repeat("─", w/2))
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

func watchDashboard(interval time.Duration, useAltScreen bool, filterTag, filterAs string, filterAll bool, quit <-chan struct{}) {
	var lastModified uint64
	var cursor uint64

	if filterAll {
		for {
			select {
			case <-quit:
				return
			default:
			}
			projects := collectWatchProjects(filterTag, filterAs)
			aggModified, aggCursor := aggregateWatchCursor(projects)
			hasLease := hasActiveLeaseAggregated(projects)
			if !hasLease || !useAltScreen {
				if lastModified != 0 && aggModified == lastModified {
					if cursor != 0 && aggCursor == cursor {
						if sleepOrQuit(interval, quit) {
							return
						}
						continue
					}
				}
			}
			lastModified = aggModified
			cursor = aggCursor
			if useAltScreen {
				clearScreen()
			}
			renderWatchDashboardAll(projects, useAltScreen, filterTag, filterAs)
			if sleepOrQuit(interval, quit) {
				return
			}
		}
	}

	for {
		select {
		case <-quit:
			return
		default:
		}
		s := loadStore()
		hasLease := hasActiveLease(s)
		if !hasLease || !useAltScreen {
			if lastModified != 0 && s.LastModified == lastModified {
				if cursor != 0 && len(s.EventsSince(cursor)) == 0 {
					if sleepOrQuit(interval, quit) {
						return
					}
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
		renderWatchDashboard(s, useAltScreen, filterTag, filterAs)
		if sleepOrQuit(interval, quit) {
			return
		}
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

type watchProject struct {
	Alias     string
	Path      string
	Available bool
	Error     string
	Store     *store.TaskStore
}

func collectWatchProjects(filterTag, filterAs string) []watchProject {
	localStore := loadStore()
	projects := []watchProject{{Alias: "local", Path: ".", Available: true, Store: localStore}}
	registry, err := loadRepositoryRegistry()
	if err != nil {
		return projects
	}
	aliases := make([]string, 0, len(registry.Repositories))
	for alias := range registry.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		path := registry.Repositories[alias]
		resolvedPath := path
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(projectRoot, resolvedPath)
		}
		s, err := store.LoadCurrent(filepath.Join(filepath.Clean(resolvedPath), ".terminal-todo", "tasks.bin"))
		if err != nil {
			projects = append(projects, watchProject{Alias: alias, Path: path, Available: false, Error: err.Error()})
			continue
		}
		projects = append(projects, watchProject{Alias: alias, Path: path, Available: true, Store: s})
	}
	return projects
}

func aggregateWatchCursor(projects []watchProject) (uint64, uint64) {
	var aggModified uint64
	var aggCursor uint64
	for _, p := range projects {
		if !p.Available {
			continue
		}
		aggModified += p.Store.LastModified
		if p.Store.NextEventID > 0 {
			aggCursor += p.Store.NextEventID
		}
	}
	return aggModified, aggCursor
}

func hasActiveLeaseAggregated(projects []watchProject) bool {
	for _, p := range projects {
		if p.Available && hasActiveLease(p.Store) {
			return true
		}
	}
	return false
}

func renderWatchDashboardAll(projects []watchProject, useAltScreen bool, filterTag, filterAs string) {
	for i, p := range projects {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s[%s] %s%s\n", colorCyan, p.Alias, p.Path, colorReset)
		if !p.Available {
			fmt.Printf("  %sunavailable: %s%s\n", colorRed, p.Error, colorReset)
			continue
		}
		renderWatchDashboard(p.Store, useAltScreen, filterTag, filterAs)
	}
}

func renderWatchDashboard(s *store.TaskStore, useAltScreen bool, filterTag, filterAs string) {
	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	passesFilter := func(t *store.Task) bool {
		if filterTag != "" && !hasTag(t.Tags, filterTag) {
			return false
		}
		if filterAs != "" && t.Owner != filterAs {
			return false
		}
		return true
	}

	grouped := map[store.TaskStatus][]*store.Task{
		store.StatusInProgress: {},
		store.StatusPending:    {},
		store.StatusBlocked:    {},
		store.StatusCompleted:  {},
	}
	for _, t := range tasks {
		if !passesFilter(t) {
			continue
		}
		grouped[t.Status] = append(grouped[t.Status], t)
	}
	resolver := dependencyResolver()
	readyAll := rankedReadyTasks(s, resolver, nil, false)
	var ready []*store.Task
	for _, t := range readyAll {
		if passesFilter(t) {
			ready = append(ready, t)
		}
	}
	diagnostics := diagnoseAllocation(s, resolver, "", nil, false, 0)
	titleMax := watchDashboardTitleMax()
	smallMax := titleMax - 6
	if smallMax < 15 {
		smallMax = 15
	}
	eventTitleMax := watchWidth() - 52
	if eventTitleMax < 15 {
		eventTitleMax = 15
	}
	if eventTitleMax > 40 {
		eventTitleMax = 40
	}

	// Header
	now := time.Now().Format("15:04:05")
	fmt.Printf("%s◉ terminal-todo — live dashboard%s  %s%s%s\n", colorCyan, colorReset, colorGray, now, colorReset)
	// Summary (filtered)
	filteredTotal := len(grouped[store.StatusInProgress]) + len(grouped[store.StatusPending]) + len(grouped[store.StatusBlocked]) + len(grouped[store.StatusCompleted])
	total := len(tasks)
	if filteredTotal > 0 {
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
		} else if filteredTotal > 0 {
			fmt.Printf("  %sReady: 0 (%s)%s", colorGray, allocationReasonMessage(diagnostics.Reason), colorReset)
		}
		fmt.Println()
		// Progress bar
		completed := len(grouped[store.StatusCompleted])
		if filteredTotal > 0 {
			pct := float64(completed) / float64(filteredTotal) * 100
			fmt.Printf("%sProgress: %d/%d %.0f%% %s", colorGray, completed, filteredTotal, pct, colorReset)
			barW := 20
			filled := int(float64(barW) * float64(completed) / float64(filteredTotal))
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
			fmt.Printf(" %s%s%s\n", colorGreen, bar, colorReset)
		}
		if filterTag != "" || filterAs != "" {
			fmt.Printf("%sFiltered by:%s%s%s%s\n", colorGray, colorReset,
				func() string {
					s := ""
					if filterTag != "" {
						s += " tag=" + filterTag
					}
					if filterAs != "" {
						s += " as=" + filterAs
					}
					return s
				}(), colorReset, "")
		}
	} else {
		if total == 0 {
			fmt.Printf("%sNo tasks yet — add one with: todo add \"Title\"%s\n", colorGray, colorReset)
		} else {
			fmt.Printf("%sNo tasks match filter%s", colorGray, colorReset)
			if filterTag != "" {
				fmt.Printf(" tag=%s", filterTag)
			}
			if filterAs != "" {
				fmt.Printf(" as=%s", filterAs)
			}
			fmt.Println()
		}
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
			fmt.Printf("  %s●%s %d %-*s prio %.2f%s%s\n", colorGreen, colorReset, t.ID, titleMax, truncateWatchTitle(t.Title, titleMax), t.Priority, caps, tags)
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
			fmt.Printf("  %s◐%s %d %-*s %s%s%s%s\n", colorYellow, colorReset, t.ID, titleMax, truncateWatchTitle(t.Title, titleMax), colorYellow+t.Owner+colorReset, colorGray+lease+colorReset, caps, tagStrWatch(t.Tags))
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
				fmt.Printf("   ○ %d %-*s prio %.2f%s%s\n", t.ID, titleMax, truncateWatchTitle(t.Title, titleMax), t.Priority, capsStrWatch(t.Capabilities), tagStrWatch(t.Tags))
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
				fmt.Printf("   ○ %d %-*s%s%s%s\n", t.ID, smallMax, truncateWatchTitle(t.Title, smallMax), colorGray+depStr+colorReset, capsStrWatch(t.Capabilities), tagStrWatch(t.Tags))
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
			fmt.Printf("  %s⊗%s %d %-*s %s%s\n", colorRed, colorReset, t.ID, titleMax, truncateWatchTitle(t.Title, titleMax), colorRed+reason+colorReset, tagStrWatch(t.Tags))
		}
	}
	if len(grouped[store.StatusCompleted]) > 0 {
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
			fmt.Printf("  %s●%s %d %-*s%s\n", colorGreen, colorReset, t.ID, titleMax, truncateWatchTitle(t.Title, titleMax), tagStrWatch(t.Tags))
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
				title = truncateWatchTitle(t.Title, eventTitleMax)
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

func watchQuitChannel(useAltScreen bool) (<-chan struct{}, func()) {
	quit := make(chan struct{})
	if !useAltScreen || !term.IsTerminal(int(os.Stdin.Fd())) {
		return quit, nil
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return quit, nil
	}
	restore := func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			if buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3 || buf[0] == 27 {
				select {
				case <-quit:
				default:
					close(quit)
				}
				return
			}
		}
	}()
	return quit, restore
}

func sleepOrQuit(d time.Duration, quit <-chan struct{}) bool {
	select {
	case <-quit:
		return true
	case <-time.After(d):
		return false
	}
}

func waitForQuitQuit(d time.Duration, quit <-chan struct{}) bool {
	return sleepOrQuit(2*d, quit)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func watchWidth() int {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w >= 40 && w <= 500 {
			return w
		}
	}
	if c := os.Getenv("COLUMNS"); c != "" {
		var w int
		if _, err := fmt.Sscanf(c, "%d", &w); err == nil && w >= 40 && w <= 500 {
			return w
		}
	}
	return 80
}

func watchDashboardTitleMax() int {
	w := watchWidth()
	m := w - 44
	if m < 20 {
		m = 20
	}
	if m > 60 {
		m = 60
	}
	return m
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
