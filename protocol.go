package main

import (
	"sort"
	"time"

	"terminal-todo/dag"
	"terminal-todo/store"
)

const protocolVersion = "1"

type protocolLogEntry struct {
	Timestamp string `json:"timestamp"`
	Agent     string `json:"agent"`
	Message   string `json:"message"`
}

type protocolEvent struct {
	ID        uint64            `json:"id"`
	Timestamp string            `json:"timestamp"`
	Type      store.EventType   `json:"type"`
	TaskID    uint64            `json:"task_id"`
	Actor     string            `json:"actor"`
	Data      map[string]string `json:"data"`
}

type protocolMetadata struct {
	Capabilities []string           `json:"capabilities"`
	Owner        string             `json:"owner,omitempty"`
	LeaseExpires *string            `json:"lease_expires,omitempty"`
	Priority     float32            `json:"priority"`
	Lineage      string             `json:"lineage,omitempty"`
	Tags         []string           `json:"tags"`
	RetryCount   uint32             `json:"retry_count"`
	LastError    string             `json:"last_error,omitempty"`
	BlockReason  string             `json:"block_reason,omitempty"`
	Log          []protocolLogEntry `json:"log"`
	Extra        map[string]string  `json:"extra"`
}

type protocolTask struct {
	ID        uint64           `json:"id"`
	Title     string           `json:"title"`
	Status    string           `json:"status"`
	Depends   []string         `json:"depends"`
	Created   string           `json:"created"`
	Completed *string          `json:"completed,omitempty"`
	Metadata  protocolMetadata `json:"metadata"`
}

type taskEnvelope struct {
	SchemaVersion string       `json:"schema_version"`
	Task          protocolTask `json:"task"`
}

type tasksEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	Tasks         []protocolTask `json:"tasks"`
}

type availableTask struct {
	ID           uint64   `json:"id"`
	Title        string   `json:"title"`
	Priority     float32  `json:"priority"`
	Capabilities []string `json:"capabilities"`
	Reason       string   `json:"reason"`
}

type blockedSummary struct {
	Count           int      `json:"count"`
	PrimaryBlockers []string `json:"primary_blockers"`
}

type nextEnvelope struct {
	SchemaVersion  string          `json:"schema_version"`
	AvailableTasks []availableTask `json:"available_tasks"`
	BlockedSummary blockedSummary  `json:"blocked_summary"`
}

func newProtocolTask(task *store.Task) protocolTask {
	capabilities := append([]string(nil), task.Capabilities...)
	depends := append([]string(nil), task.Depends...)
	tags := append([]string(nil), task.Tags...)
	logEntries := make([]protocolLogEntry, len(task.Log))
	for i, e := range task.Log {
		logEntries[i] = newProtocolLogEntry(e)
	}
	if capabilities == nil {
		capabilities = []string{}
	}
	if depends == nil {
		depends = []string{}
	}
	if tags == nil {
		tags = []string{}
	}
	extra := task.Extra
	if extra == nil {
		extra = map[string]string{}
	}

	result := protocolTask{
		ID:      task.ID,
		Title:   task.Title,
		Status:  statusName(task.Status),
		Depends: depends,
		Created: formatTimestamp(task.Created),
		Metadata: protocolMetadata{
			Capabilities: capabilities,
			Owner:        task.Owner,
			Priority:     task.Priority,
			Lineage:      task.Lineage,
			Tags:         tags,
			RetryCount:   task.RetryCount,
			LastError:    task.LastError,
			BlockReason:  task.BlockReason,
			Log:          logEntries,
			Extra:        extra,
		},
	}
	if task.Completed > 0 {
		completed := formatTimestamp(task.Completed)
		result.Completed = &completed
	}
	if task.LeaseExpires > 0 {
		leaseExpires := formatTimestamp(task.LeaseExpires)
		result.Metadata.LeaseExpires = &leaseExpires
	}
	return result
}

func statusName(status store.TaskStatus) string {
	switch status {
	case store.StatusPending:
		return "pending"
	case store.StatusInProgress:
		return "in_progress"
	case store.StatusCompleted:
		return "completed"
	case store.StatusBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

func formatTimestamp(milliseconds uint64) string {
	return time.UnixMilli(int64(milliseconds)).UTC().Format(time.RFC3339Nano)
}

func newProtocolLogEntry(e store.LogEntry) protocolLogEntry {
	return protocolLogEntry{
		Timestamp: formatTimestamp(e.Timestamp),
		Agent:     e.Agent,
		Message:   e.Message,
	}
}

func newProtocolEvent(e store.Event) protocolEvent {
	data := e.Data
	if data == nil {
		data = map[string]string{}
	}
	return protocolEvent{
		ID:        e.ID,
		Timestamp: formatTimestamp(e.Timestamp),
		Type:      e.Type,
		TaskID:    e.TaskID,
		Actor:     e.Actor,
		Data:      data,
	}
}

func newBlockedSummary(tasks map[uint64]*store.Task) blockedSummary {
	return newBlockedSummaryWithResolver(tasks, nil)
}

func newBlockedSummaryWithResolver(tasks map[uint64]*store.Task, resolver dag.DependencyResolver) blockedSummary {
	blocked := dag.NewDAG().GetBlockedTasksWithResolver(tasks, resolver)
	counted := make(map[uint64]struct{}, len(blocked))
	unique := make(map[string]struct{})
	for taskID, blockers := range blocked {
		counted[taskID] = struct{}{}
		for _, blocker := range blockers {
			unique[blocker] = struct{}{}
		}
	}
	for taskID, task := range tasks {
		if task.Status == store.StatusBlocked {
			counted[taskID] = struct{}{}
		}
	}
	primary := make([]string, 0, len(unique))
	for blocker := range unique {
		primary = append(primary, blocker)
	}
	sort.Strings(primary)
	return blockedSummary{Count: len(counted), PrimaryBlockers: primary}
}
