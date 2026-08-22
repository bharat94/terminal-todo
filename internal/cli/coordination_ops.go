package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
)

// Lifecycle operations.
//
// Each function is the single implementation of one state transition. It runs
// inside a store transaction, validates through the shared guards, applies the
// mutation, and records the log entry and audit event. It renders nothing and
// decides nothing about presentation.
//
// Surfaces are adapters over these functions: the CLI parses flags and prints,
// the JSON-RPC server decodes parameters and builds envelopes, and MCP wraps
// the JSON-RPC dispatch table. Because the transition itself exists once,
// the surfaces cannot drift apart on what a transition does, which fields it
// touches, or which events it emits.
//
// renewLease in lease.go is the same shape and predates this file.

// claimTask leases a specific ready task to actor.
//
// Guards run in a deliberate order: a terminal or blocked status is reported
// before an unmet prerequisite, and an unmet prerequisite before a competing
// lease. The most fundamental reason the task cannot be worked is the one the
// caller hears about.
func claimTask(
	s *store.TaskStore,
	id uint64,
	actor string,
	ttl time.Duration,
	resolver dag.DependencyResolver,
	now time.Time,
) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireClaimableStatus(task); err != nil {
		return nil, err
	}
	if err := requireDependenciesComplete(task, s.Tasks, resolver); err != nil {
		return nil, err
	}
	if err := requireLeaseAvailable(task, actor, now); err != nil {
		return nil, err
	}

	task.Owner = actor
	task.Status = store.StatusInProgress
	task.LeaseExpires = uint64(now.UnixMilli()) + uint64(ttl.Milliseconds())
	s.AddLog(id, actor, "claimed")
	s.AddEvent(store.EventTaskClaimed, id, actor, map[string]string{"ttl": ttl.String()})
	return task, nil
}

// completeTask marks owned work complete and clears its lease.
//
// An unowned task may be completed by anyone, which is what lets a human close
// out work a crashed agent left behind.
func completeTask(
	s *store.TaskStore,
	id uint64,
	actor string,
	resolver dag.DependencyResolver,
	now time.Time,
) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if task.Status == store.StatusCompleted {
		return nil, fmt.Errorf("task %d is already completed: %w", id, errInvalidTransition)
	}
	if err := requireDependenciesComplete(task, s.Tasks, resolver); err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.Status = store.StatusCompleted
	task.Completed = uint64(now.UnixMilli())
	task.Owner = ""
	task.LeaseExpires = 0
	task.BlockReason = ""
	s.AddEvent(store.EventTaskCompleted, id, actor, nil)
	return task, nil
}

// releaseTask yields an owned lease back to the pool.
//
// A release counts as a failed attempt: it increments the retry count and, when
// a reason is supplied, records it as the task's last error so the next worker
// inherits the finding. handoffTask exists for the case where a worker yields
// deliberately and should not be charged a retry.
func releaseTask(s *store.TaskStore, id uint64, actor, failure string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireInProgress(task); err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.RetryCount++
	data := map[string]string{}
	if failure != "" {
		task.LastError = failure
		data["error"] = failure
		s.AddLog(id, actor, fmt.Sprintf("released with error: %s", failure))
	} else {
		s.AddLog(id, actor, "released")
	}
	s.AddEvent(store.EventTaskReleased, id, actor, data)
	task.Owner = ""
	task.LeaseExpires = 0
	task.Status = store.StatusPending
	return task, nil
}

// blockTask records a blocker and releases ownership.
//
// Ownership is dropped deliberately: recovery of blocked work must not depend
// on the worker that discovered the blocker still being alive.
func blockTask(s *store.TaskStore, id uint64, actor, reason string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if task.Status == store.StatusCompleted {
		return nil, fmt.Errorf("task %d is already completed: %w", id, errInvalidTransition)
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	task.Status = store.StatusBlocked
	task.BlockReason = reason
	task.Owner = ""
	task.LeaseExpires = 0
	s.AddLog(id, actor, fmt.Sprintf("blocked: %s", reason))
	s.AddEvent(store.EventTaskBlocked, id, actor, map[string]string{"reason": reason})
	return task, nil
}

// unblockTask returns blocked work to the pending pool.
//
// Blocked tasks hold no lease, so the owner fields are cleared rather than
// checked: stores written before blocking released ownership can still carry a
// stale lease, and unblocking is the point at which that legacy state is
// repaired.
func unblockTask(s *store.TaskStore, id uint64, actor string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if task.Status != store.StatusBlocked {
		return nil, fmt.Errorf("task %d is not blocked: %w", id, errInvalidTransition)
	}

	task.Status = store.StatusPending
	task.BlockReason = ""
	task.Owner = ""
	task.LeaseExpires = 0
	s.AddLog(id, actor, "unblocked")
	s.AddEvent(store.EventTaskUnblocked, id, actor, nil)
	return task, nil
}

// logTask appends a finding to a task's audit trail without changing its
// lifecycle state. Writing to work leased by another agent is refused, because
// the log is the handoff record and must stay attributable.
func logTask(s *store.TaskStore, id uint64, actor, message string) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	s.AddLog(id, actor, message)
	return task, nil
}

// decomposeTask injects child work under a parent and makes the parent depend
// on it.
//
// The parent returns to pending and yields its lease: its own completion now
// depends on children that a different worker may pick up, so holding the
// lease would block the graph it just created. Sibling children are parallel
// unless dependencies are added between them explicitly.
func decomposeTask(
	s *store.TaskStore,
	parentID uint64,
	actor string,
	subtasks []decomposeSubtask,
) (*store.Task, []*store.Task, error) {
	parent, err := requireTask(s, parentID)
	if err != nil {
		return nil, nil, fmt.Errorf("parent task %d not found: %w", parentID, errTaskNotFound)
	}
	if parent.Status == store.StatusCompleted {
		return nil, nil, fmt.Errorf("parent task %d is already completed: %w", parentID, errInvalidTransition)
	}
	if err := requireOwner(parent, actor); err != nil {
		return nil, nil, err
	}
	// Stage validation before mutation: detect cycles and cardinality on the
	// projected graph so a failure leaves no partial children in the store.
	parentDepSet := make(map[string]struct{}, len(parent.Depends))
	for _, dep := range parent.Depends {
		canonical, err := canonicalDependency(dep)
		if err != nil {
			canonical = dep
		}
		parentDepSet[canonical] = struct{}{}
	}
	projectedDeps := make([]string, 0, len(parentDepSet)+len(subtasks))
	for dep := range parentDepSet {
		projectedDeps = append(projectedDeps, dep)
	}
	// Reserve child IDs without mutating NextID.
	nextID := s.NextID
	for range subtasks {
		projectedDeps = append(projectedDeps, fmt.Sprintf("todo://local/%d", nextID))
		nextID++
	}
	if err := validateProjectedCardinality(
		"dependencies",
		len(parentDepSet),
		len(projectedDeps),
		maxTaskDependencies,
	); err != nil {
		return nil, nil, persistedInputFailure(err)
	}
	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)
	if err := d.DetectCycle(projectedDeps, parentID); err != nil {
		return nil, nil, fmt.Errorf("decompose would create a cycle: %w: %w", err, errCycleDetected)
	}

	added := make([]*store.Task, 0, len(subtasks))
	for _, sub := range subtasks {
		child := s.AddTask(sub.Title, nil)
		child.Capabilities = sub.Capabilities
		child.Lineage = fmt.Sprintf("todo://local/%d", parentID)
		parent.Depends = append(parent.Depends, fmt.Sprintf("todo://local/%d", child.ID))
		added = append(added, child)
	}

	parent.Status = store.StatusPending
	parent.Owner = ""
	parent.LeaseExpires = 0
	parent.BlockReason = ""
	s.AddEvent(store.EventTaskDecomposed, parentID, actor, map[string]string{
		"count": fmt.Sprintf("%d", len(subtasks)),
	})
	return parent, added, nil
}

// acquirePlan is the resolved input to an atomic acquisition: everything that
// must be settled before the transaction opens.
//
// Both surfaces built this identically — default TTL from config, explicit TTL
// parsing, explicit-versus-registered capabilities, agent profile lookup, and
// the idempotency fingerprint. The fingerprint is the reason it matters that
// they agree: it is what makes a retried request with the same parameters
// replay instead of allocating a second task, so a difference in how either
// surface derives it would silently break idempotency across transports.
type acquirePlan struct {
	Actor        string
	TTL          time.Duration
	Capabilities []string
	MaxLoad      int
	Fingerprint  string
}

// newAcquirePlan resolves an acquisition request. explicitTTL and
// explicitCapabilities are the caller's overrides; an empty TTL falls back to
// project configuration, and nil capabilities fall back to the agent's
// registered profile. Passing an empty non-nil slice is meaningful: it
// explicitly requests no capabilities rather than inheriting registered ones.
func newAcquirePlan(actor, explicitTTL string, explicitCapabilities []string) (acquirePlan, error) {
	cfg, err := loadConfig()
	if err != nil {
		return acquirePlan{}, fmt.Errorf("loading config: %w", err)
	}

	ttl := parseDefaultTTL(cfg)
	ttlMode := "default"
	if explicitTTL != "" {
		parsed, parseErr := time.ParseDuration(explicitTTL)
		if parseErr != nil || parsed <= 0 {
			return acquirePlan{}, errInvalidTTL
		}
		ttl = parsed
		ttlMode = "explicit:" + ttl.String()
	}

	capabilitiesMode := "registered"
	if explicitCapabilities != nil {
		capabilitiesMode = "explicit"
		if len(explicitCapabilities) == 0 {
			explicitCapabilities = []string{}
		}
	}
	if err := validateCapabilities(explicitCapabilities); err != nil {
		return acquirePlan{}, err
	}

	capabilities, maxLoad, err := agentAllocationProfile(actor, explicitCapabilities)
	if err != nil {
		return acquirePlan{}, fmt.Errorf("loading agent profile: %w", err)
	}

	return acquirePlan{
		Actor:        actor,
		TTL:          ttl,
		Capabilities: capabilities,
		MaxLoad:      maxLoad,
		Fingerprint:  acquireFingerprint(actor, ttlMode, capabilitiesMode, explicitCapabilities),
	}, nil
}

// errInvalidTTL is the shared rejection for a malformed or non-positive TTL.
// Surfaces render it with their own flag or field name.
var errInvalidTTL = errors.New("must be a positive duration")

// parseLeaseTTL resolves a lease duration, falling back to project
// configuration when the caller supplied nothing.
func parseLeaseTTL(explicit string) (time.Duration, error) {
	cfg, err := loadConfig()
	if err != nil {
		return 0, fmt.Errorf("loading config: %w", err)
	}
	if explicit == "" {
		return parseDefaultTTL(cfg), nil
	}
	ttl, err := time.ParseDuration(explicit)
	if err != nil || ttl <= 0 {
		return 0, errInvalidTTL
	}
	return ttl, nil
}

// taskUpdate describes a metadata change. Every field is optional, and the
// distinction between "absent" and "empty" matters: a nil Title leaves the
// title alone, whereas SetCapabilities with an empty slice deliberately clears
// the requirement list.
type taskUpdate struct {
	Title           *string
	Priority        *float32
	SetCapabilities bool
	Capabilities    []string
	Extra           map[string]string
	AddDeps         []string
	RemoveDeps      []string
}

// updateTask applies a metadata change to owned or unowned work.
//
// Dependency edits are the delicate part. Removals must name an edge that
// exists, additions must name a task that exists, and the resulting graph is
// checked for cycles before anything is committed — the check runs against a
// DAG built with the projected edges, not the current ones, so a removal
// cannot produce a false positive.
func updateTask(s *store.TaskStore, id uint64, actor string, u taskUpdate) (*store.Task, error) {
	task, err := requireTask(s, id)
	if err != nil {
		return nil, err
	}
	if err := requireOwner(task, actor); err != nil {
		return nil, err
	}

	if len(u.AddDeps) > 0 || len(u.RemoveDeps) > 0 {
		if err := applyDependencyEdits(s, task, actor, u.AddDeps, u.RemoveDeps); err != nil {
			return nil, err
		}
	}

	if u.Title != nil {
		title := strings.TrimSpace(*u.Title)
		if title == "" {
			return nil, lifecycleError(ErrInvalidArgs, "title cannot be empty")
		}
		task.Title = title
	}
	if u.Priority != nil {
		if !validPriority32(*u.Priority) {
			return nil, lifecycleError(ErrInvalidArgs, "priority must be between 0 and 1")
		}
		task.Priority = *u.Priority
	}
	if u.SetCapabilities {
		task.Capabilities = u.Capabilities
	}

	if task.Extra == nil {
		task.Extra = make(map[string]string)
	}
	if err := validateProjectedCardinality(
		"extra",
		len(task.Extra),
		projectedExtraCount(task.Extra, u.Extra),
		maxTaskExtraEntries,
	); err != nil {
		return nil, persistedInputFailure(err)
	}
	for key, value := range u.Extra {
		task.Extra[key] = value
	}

	if u.Title != nil || u.Priority != nil || u.SetCapabilities || len(u.Extra) > 0 {
		s.AddEvent(store.EventTaskUpdated, task.ID, actor, nil)
	}
	return task, nil
}

// applyDependencyEdits rewrites a task's dependency list and records one event
// per edge changed.
func applyDependencyEdits(s *store.TaskStore, task *store.Task, actor string, add, remove []string) error {
	// Existing edges are canonicalized too. A store written before this
	// normalization can hold several spellings of one edge, and an edit is the
	// point at which that is repaired for the task being edited.
	depSet := make(map[string]bool, len(task.Depends))
	for _, dep := range task.Depends {
		canonical, err := canonicalDependency(dep)
		if err != nil {
			// Preserve a reference this build cannot interpret rather than
			// dropping an edge that a newer binary may understand.
			canonical = dep
		}
		depSet[canonical] = true
	}
	for _, dep := range remove {
		canonical, err := canonicalDependency(dep)
		if err != nil {
			return err
		}
		if !depSet[canonical] {
			return fmt.Errorf("dependency %q not found on task %d: %w", dep, task.ID, errTaskNotFound)
		}
		delete(depSet, canonical)
	}
	for _, dep := range add {
		canonical, err := canonicalDependency(dep)
		if err != nil {
			return err
		}
		if depSet[canonical] {
			continue
		}
		if depID, local := dag.ParseLocalID(canonical); local {
			if _, ok := s.Tasks[depID]; !ok {
				return fmt.Errorf("dependency task %d not found: %w", depID, errTaskNotFound)
			}
		}
		depSet[canonical] = true
	}
	if err := validateProjectedCardinality(
		"dependencies", len(task.Depends), len(depSet), maxTaskDependencies,
	); err != nil {
		return persistedInputFailure(err)
	}

	newDeps := make([]string, 0, len(depSet))
	for dep := range depSet {
		newDeps = append(newDeps, dep)
	}
	sort.Strings(newDeps)

	// Detect cycles against the projected graph. Building it with the current
	// edges would report a false positive whenever an edit removes the very
	// dependency that closed the cycle.
	d := dag.NewDAG()
	oldDeps := task.Depends
	task.Depends = newDeps
	d.BuildFromTasks(s.Tasks)
	task.Depends = oldDeps
	if err := d.DetectCycle(nil, task.ID); err != nil {
		return fmt.Errorf("cannot update dependencies: %w: %w", err, errCycleDetected)
	}

	// Compare canonically on both sides so a re-spelled edge is not reported
	// as one removal plus one addition of the same dependency.
	oldSet := make(map[string]bool, len(task.Depends))
	for _, dep := range task.Depends {
		canonical, err := canonicalDependency(dep)
		if err != nil {
			canonical = dep
		}
		oldSet[canonical] = true
	}
	for _, dep := range newDeps {
		if !oldSet[dep] {
			s.AddEvent(store.EventDependencyAdded, task.ID, actor, map[string]string{"dep": dep})
		}
	}
	for dep := range oldSet {
		if !depSet[dep] {
			s.AddEvent(store.EventDependencyRemoved, task.ID, actor, map[string]string{"dep": dep})
		}
	}

	task.Depends = newDeps
	return nil
}

// pruneCompletedTasks removes every completed task and drops the now-dangling
// local edges that pointed at them.
//
// Cleaning the edges is what makes pruning safe: a surviving task that still
// listed a removed prerequisite would never be ready again, because the
// dependency could not resolve. Cross-repository edges are left alone — they
// resolve against another store, and their absence here means nothing.
//
// Removed tasks are returned in ascending ID order so callers do not have to
// re-sort a map iteration.
func pruneCompletedTasks(s *store.TaskStore) []*store.Task {
	completed := make(map[uint64]struct{})
	var removed []*store.Task
	for _, task := range s.GetAllTasks() {
		if task.Status == store.StatusCompleted {
			completed[task.ID] = struct{}{}
			removed = append(removed, task)
		}
	}

	for _, task := range s.Tasks {
		if _, willRemove := completed[task.ID]; willRemove {
			continue
		}
		kept := make([]string, 0, len(task.Depends))
		for _, dependency := range task.Depends {
			canonical, err := canonicalDependency(dependency)
			if err != nil {
				canonical = dependency
			}
			dependencyID, local := dag.ParseLocalID(canonical)
			if _, pruned := completed[dependencyID]; local && pruned {
				continue
			}
			kept = append(kept, canonical)
		}
		task.Depends = kept
	}

	for id := range completed {
		s.RemoveTask(id)
	}

	sort.Slice(removed, func(i, j int) bool { return removed[i].ID < removed[j].ID })
	return removed
}

// newlyReadyAfter reports the pending tasks whose prerequisites are satisfied
// now but were not before, given the IDs that just completed.
//
// This answers the question a worker actually has after finishing something:
// what did I just unblock? It is a read over already-committed state, so it
// must run after the mutation.
func newlyReadyAfter(s *store.TaskStore, completedIDs []uint64, resolver dag.DependencyResolver) []*store.Task {
	if len(completedIDs) == 0 {
		return nil
	}
	justCompleted := make(map[uint64]struct{}, len(completedIDs))
	for _, id := range completedIDs {
		justCompleted[id] = struct{}{}
	}

	var unblocked []*store.Task
	for _, task := range s.GetAllTasks() {
		if task.Status != store.StatusPending {
			continue
		}
		if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
			continue
		}
		// Only report tasks the completion actually released. A task that was
		// already ready is not news.
		dependsOnCompletion := false
		for _, dependency := range task.Depends {
			dependencyID, local := dag.ParseLocalID(dependency)
			if _, ok := justCompleted[dependencyID]; local && ok {
				dependsOnCompletion = true
				break
			}
		}
		if dependsOnCompletion {
			unblocked = append(unblocked, task)
		}
	}

	sort.Slice(unblocked, func(i, j int) bool { return unblocked[i].ID < unblocked[j].ID })
	return unblocked
}

// canonicalDependency returns the stable stored form of a dependency
// reference.
//
// Dependencies are resolved numerically but compared as strings, so every
// spelling of one edge has to collapse to one key. Without this, `1`,
// `todo://local/1`, and `todo://local/01` are three separate edges pointing at
// the same task: they inflate the dependency count, and removing one leaves
// the others, so a task stays blocked by an edge the user believes they
// deleted.
func canonicalDependency(reference string) (string, error) {
	if id, local := dag.ParseLocalID(reference); local {
		return fmt.Sprintf("todo://local/%d", id), nil
	}
	alias, id, err := dag.ParseTaskURI(reference)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("todo://%s/%d", alias, id), nil
}
