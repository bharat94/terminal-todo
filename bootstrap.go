package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bharat94/terminal-todo/dag"
	"github.com/bharat94/terminal-todo/store"
)

const (
	defaultBootstrapLimit      = 5
	defaultBootstrapEventLimit = 5
	maxBootstrapLimit          = 20
	maxBootstrapActorBytes     = 128
	maxBootstrapTitleBytes     = 160
	maxBootstrapDetailBytes    = 240
	maxBootstrapValueBytes     = 96
)

type bootstrapParams struct {
	Actor        string   `json:"actor"`
	Capabilities []string `json:"capabilities,omitempty"`
	ObjectiveID  uint64   `json:"objectiveId,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	EventLimit   int      `json:"eventLimit,omitempty"`
}

type bootstrapWorker struct {
	Actor            string   `json:"actor"`
	Capabilities     []string `json:"capabilities"`
	CapabilitySource string   `json:"capability_source"`
	MaxLoad          int      `json:"max_load"`
	CurrentLoad      int      `json:"current_load"`
	AtCapacity       bool     `json:"at_capacity"`
	Registered       bool     `json:"registered"`
}

type bootstrapTask struct {
	ID           uint64   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Priority     float32  `json:"priority"`
	Capabilities []string `json:"capabilities"`
	Owner        string   `json:"owner,omitempty"`
	LeaseExpires *string  `json:"lease_expires,omitempty"`
	BlockReason  string   `json:"block_reason,omitempty"`
}

type bootstrapTaskList struct {
	Total int             `json:"total"`
	Items []bootstrapTask `json:"items"`
}

type bootstrapProgress struct {
	Scope      string `json:"scope"`
	Total      int    `json:"total"`
	Pending    int    `json:"pending"`
	InProgress int    `json:"in_progress"`
	Blocked    int    `json:"blocked"`
	Completed  int    `json:"completed"`
	Percent    int    `json:"percent"`
}

type bootstrapBlockers struct {
	ExplicitTotal            int             `json:"explicit_total"`
	Explicit                 []bootstrapTask `json:"explicit"`
	DependencyBlockedTotal   int             `json:"dependency_blocked_total"`
	PrimaryDependenciesTotal int             `json:"primary_dependencies_total"`
	PrimaryDependencies      []string        `json:"primary_dependencies"`
}

type bootstrapCapabilityDemand struct {
	Capability string `json:"capability"`
	TaskCount  int    `json:"task_count"`
	Matched    bool   `json:"matched"`
}

type bootstrapCapabilitySummary struct {
	UnclaimedTasks int                         `json:"unclaimed_tasks"`
	WithoutCaps    int                         `json:"without_capabilities"`
	Total          int                         `json:"total"`
	Items          []bootstrapCapabilityDemand `json:"items"`
}

type bootstrapEvent struct {
	ID        uint64          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Type      store.EventType `json:"type"`
	TaskID    uint64          `json:"task_id"`
	TaskTitle string          `json:"task_title,omitempty"`
	Actor     string          `json:"actor,omitempty"`
	Detail    string          `json:"detail,omitempty"`
}

type bootstrapEventList struct {
	Total int              `json:"total"`
	Items []bootstrapEvent `json:"items"`
}

type bootstrapResult struct {
	SchemaVersion    string                     `json:"schema_version"`
	Worker           bootstrapWorker            `json:"worker"`
	Objective        *bootstrapTask             `json:"objective,omitempty"`
	ObjectiveSource  string                     `json:"objective_source"`
	Progress         bootstrapProgress          `json:"progress"`
	OwnedWork        bootstrapTaskList          `json:"owned_work"`
	ReadyWork        bootstrapTaskList          `json:"ready_work"`
	Blockers         bootstrapBlockers          `json:"blockers"`
	CapabilityDemand bootstrapCapabilitySummary `json:"capability_demand"`
	RecentEvents     bootstrapEventList         `json:"recent_events"`
}

func normalizeBootstrapParams(p bootstrapParams) (bootstrapParams, error) {
	p.Actor = strings.TrimSpace(p.Actor)
	if p.Actor == "" {
		return p, fmt.Errorf("actor is required")
	}
	if len(p.Actor) > maxBootstrapActorBytes {
		return p, fmt.Errorf("actor must be at most %d bytes", maxBootstrapActorBytes)
	}
	if p.Limit == 0 {
		p.Limit = defaultBootstrapLimit
	}
	if p.EventLimit == 0 {
		p.EventLimit = defaultBootstrapEventLimit
	}
	if p.Limit < 1 || p.Limit > maxBootstrapLimit {
		return p, fmt.Errorf("limit must be between 1 and %d", maxBootstrapLimit)
	}
	if p.EventLimit < 1 || p.EventLimit > maxBootstrapLimit {
		return p, fmt.Errorf("eventLimit must be between 1 and %d", maxBootstrapLimit)
	}
	if len(p.Capabilities) > maxBootstrapLimit {
		return p, fmt.Errorf("capabilities must contain at most %d values", maxBootstrapLimit)
	}

	seen := make(map[string]struct{}, len(p.Capabilities))
	capabilities := make([]string, 0, len(p.Capabilities))
	for _, capability := range p.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return p, fmt.Errorf("capabilities cannot contain an empty value")
		}
		if len(capability) > maxBootstrapValueBytes {
			return p, fmt.Errorf("capability must be at most %d bytes", maxBootstrapValueBytes)
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	p.Capabilities = capabilities
	return p, nil
}

func buildBootstrap(s *store.TaskStore, registry *AgentRegistry, p bootstrapParams, capabilitiesExplicit bool, resolver dag.DependencyResolver) (bootstrapResult, error) {
	p, err := normalizeBootstrapParams(p)
	if err != nil {
		return bootstrapResult{}, err
	}

	card, registered := registry.Agents[p.Actor]
	effectiveCapabilities := append([]string(nil), p.Capabilities...)
	capabilitySource := "explicit"
	if !capabilitiesExplicit {
		effectiveCapabilities = append([]string(nil), card.Capabilities...)
		capabilitySource = "agent_card"
		if !registered {
			capabilitySource = "none"
		}
	}
	currentLoad := computeAgentLoad(s, p.Actor)

	objective, objectiveSource, err := selectBootstrapObjective(s, p.ObjectiveID)
	if err != nil {
		return bootstrapResult{}, err
	}

	tasks := s.GetAllTasks()
	owned := make([]*store.Task, 0)
	explicitBlocked := make([]*store.Task, 0)
	for _, task := range tasks {
		if task.Owner == p.Actor {
			owned = append(owned, task)
		}
		if task.Status == store.StatusBlocked {
			explicitBlocked = append(explicitBlocked, task)
		}
	}
	sortBootstrapTasks(owned)
	sortBootstrapTasks(explicitBlocked)

	ready := rankedReadyTasks(s, resolver, effectiveCapabilities, true)
	dependencyBlocked := dag.NewDAG().GetBlockedTasksWithResolver(s.Tasks, resolver)
	blockedSummary := newBlockedSummaryWithResolver(s.Tasks, resolver)

	return bootstrapResult{
		SchemaVersion: protocolVersion,
		Worker: bootstrapWorker{
			Actor:            compactBootstrapString(p.Actor, maxBootstrapActorBytes),
			Capabilities:     boundedStrings(effectiveCapabilities, maxBootstrapLimit, maxBootstrapValueBytes),
			CapabilitySource: capabilitySource,
			MaxLoad:          card.MaxLoad,
			CurrentLoad:      currentLoad,
			AtCapacity:       card.MaxLoad > 0 && currentLoad >= card.MaxLoad,
			Registered:       registered,
		},
		Objective:       bootstrapTaskPointer(objective),
		ObjectiveSource: objectiveSource,
		Progress:        bootstrapObjectiveProgress(s, objective),
		OwnedWork: bootstrapTaskList{
			Total: len(owned),
			Items: boundedBootstrapTasks(owned, p.Limit),
		},
		ReadyWork: bootstrapTaskList{
			Total: len(ready),
			Items: boundedBootstrapTasks(ready, p.Limit),
		},
		Blockers: bootstrapBlockers{
			ExplicitTotal:            len(explicitBlocked),
			Explicit:                 boundedBootstrapTasks(explicitBlocked, p.Limit),
			DependencyBlockedTotal:   len(dependencyBlocked),
			PrimaryDependenciesTotal: len(blockedSummary.PrimaryBlockers),
			PrimaryDependencies:      boundedStrings(blockedSummary.PrimaryBlockers, p.Limit, maxBootstrapValueBytes),
		},
		CapabilityDemand: buildBootstrapCapabilityDemand(s, registry, p.Limit),
		RecentEvents:     buildBootstrapEvents(s, p.EventLimit),
	}, nil
}

func selectBootstrapObjective(s *store.TaskStore, explicitID uint64) (*store.Task, string, error) {
	if explicitID > 0 {
		task, ok := s.GetTask(explicitID)
		if !ok {
			return nil, "", fmt.Errorf("objective task %d not found", explicitID)
		}
		return task, "explicit", nil
	}

	candidates := make([]*store.Task, 0)
	for _, task := range s.GetAllTasks() {
		if task.Status != store.StatusCompleted && task.Lineage == "" {
			candidates = append(candidates, task)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].Depends) != len(candidates[j].Depends) {
			return len(candidates[i].Depends) > len(candidates[j].Depends)
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > 0 {
		return candidates[0], "inferred_top_level", nil
	}

	completed := make([]*store.Task, 0)
	for _, task := range s.GetAllTasks() {
		if task.Status == store.StatusCompleted {
			completed = append(completed, task)
		}
	}
	sort.Slice(completed, func(i, j int) bool {
		if completed[i].Completed != completed[j].Completed {
			return completed[i].Completed > completed[j].Completed
		}
		return completed[i].ID > completed[j].ID
	})
	if len(completed) > 0 {
		return completed[0], "latest_completed", nil
	}
	return nil, "none", nil
}

func bootstrapObjectiveProgress(s *store.TaskStore, objective *store.Task) bootstrapProgress {
	progress := bootstrapProgress{Scope: "project"}
	scope := make(map[uint64]struct{})
	if objective != nil {
		progress.Scope = fmt.Sprintf("objective:%d", objective.ID)
		var visit func(uint64)
		visit = func(id uint64) {
			if _, seen := scope[id]; seen {
				return
			}
			task, ok := s.GetTask(id)
			if !ok {
				return
			}
			scope[id] = struct{}{}
			for _, dependency := range task.Depends {
				if dependencyID, local := dag.ParseLocalID(dependency); local {
					visit(dependencyID)
				}
			}
		}
		visit(objective.ID)
	} else {
		for id := range s.Tasks {
			scope[id] = struct{}{}
		}
	}

	for id := range scope {
		task := s.Tasks[id]
		progress.Total++
		switch task.Status {
		case store.StatusPending:
			progress.Pending++
		case store.StatusInProgress:
			progress.InProgress++
		case store.StatusBlocked:
			progress.Blocked++
		case store.StatusCompleted:
			progress.Completed++
		}
	}
	if progress.Total > 0 {
		progress.Percent = progress.Completed * 100 / progress.Total
	}
	return progress
}

func buildBootstrapCapabilityDemand(s *store.TaskStore, registry *AgentRegistry, limit int) bootstrapCapabilitySummary {
	counts := make(map[string]int)
	unclaimed := 0
	withoutCaps := 0
	for _, task := range s.Tasks {
		if task.Status == store.StatusCompleted || task.Owner != "" {
			continue
		}
		unclaimed++
		if len(task.Capabilities) == 0 {
			withoutCaps++
			continue
		}
		seen := make(map[string]struct{}, len(task.Capabilities))
		for _, capability := range task.Capabilities {
			if _, duplicate := seen[capability]; duplicate {
				continue
			}
			seen[capability] = struct{}{}
			counts[capability]++
		}
	}

	matched := make(map[string]bool)
	for _, card := range registry.Agents {
		for _, capability := range card.Capabilities {
			matched[capability] = true
		}
	}
	items := make([]bootstrapCapabilityDemand, 0, len(counts))
	for capability, count := range counts {
		items = append(items, bootstrapCapabilityDemand{
			Capability: compactBootstrapString(capability, maxBootstrapValueBytes),
			TaskCount:  count,
			Matched:    matched[capability],
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TaskCount != items[j].TaskCount {
			return items[i].TaskCount > items[j].TaskCount
		}
		return items[i].Capability < items[j].Capability
	})
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []bootstrapCapabilityDemand{}
	}
	return bootstrapCapabilitySummary{
		UnclaimedTasks: unclaimed,
		WithoutCaps:    withoutCaps,
		Total:          total,
		Items:          items,
	}
}

func buildBootstrapEvents(s *store.TaskStore, limit int) bootstrapEventList {
	total := len(s.Events)
	start := total - limit
	if start < 0 {
		start = 0
	}
	items := make([]bootstrapEvent, 0, total-start)
	for i := total - 1; i >= start; i-- {
		event := s.Events[i]
		item := bootstrapEvent{
			ID:        event.ID,
			Timestamp: formatTimestamp(event.Timestamp),
			Type:      event.Type,
			TaskID:    event.TaskID,
			Actor:     compactBootstrapString(event.Actor, maxBootstrapActorBytes),
			Detail:    bootstrapEventDetail(event),
		}
		if task, ok := s.Tasks[event.TaskID]; ok {
			item.TaskTitle = compactBootstrapString(task.Title, maxBootstrapTitleBytes)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []bootstrapEvent{}
	}
	return bootstrapEventList{Total: total, Items: items}
}

func bootstrapEventDetail(event store.Event) string {
	keys := []string{"reason", "error", "dep", "title", "owner", "ttl"}
	parts := make([]string, 0, 2)
	for _, key := range keys {
		value := strings.TrimSpace(event.Data[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+compactBootstrapString(value, maxBootstrapValueBytes))
		if len(parts) == 2 {
			break
		}
	}
	return compactBootstrapString(strings.Join(parts, ", "), maxBootstrapDetailBytes)
}

func sortBootstrapTasks(tasks []*store.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func boundedBootstrapTasks(tasks []*store.Task, limit int) []bootstrapTask {
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	result := make([]bootstrapTask, len(tasks))
	for i, task := range tasks {
		result[i] = newBootstrapTask(task)
	}
	return result
}

func bootstrapTaskPointer(task *store.Task) *bootstrapTask {
	if task == nil {
		return nil
	}
	result := newBootstrapTask(task)
	return &result
}

func newBootstrapTask(task *store.Task) bootstrapTask {
	result := bootstrapTask{
		ID:           task.ID,
		Title:        compactBootstrapString(task.Title, maxBootstrapTitleBytes),
		Status:       statusName(task.Status),
		Priority:     task.Priority,
		Capabilities: boundedStrings(task.Capabilities, maxBootstrapLimit, maxBootstrapValueBytes),
		Owner:        compactBootstrapString(task.Owner, maxBootstrapActorBytes),
		BlockReason:  compactBootstrapString(task.BlockReason, maxBootstrapDetailBytes),
	}
	if task.LeaseExpires > 0 {
		expires := formatTimestamp(task.LeaseExpires)
		result.LeaseExpires = &expires
	}
	return result
}

func boundedStrings(values []string, limit, maxBytes int) []string {
	if len(values) > limit {
		values = values[:limit]
	}
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = compactBootstrapString(value, maxBytes)
	}
	return result
}

func compactBootstrapString(value string, maxBytes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes - 3
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "..."
}
