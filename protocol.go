package main

import (
	"sort"
	"time"

	"terminal-todo/dag"
	"terminal-todo/store"
)

const protocolVersion = "1"

type protocolMetadata struct {
	Capabilities []string          `json:"capabilities"`
	Owner        string            `json:"owner,omitempty"`
	LeaseExpires *string           `json:"lease_expires,omitempty"`
	Priority     float32           `json:"priority"`
	Lineage      string            `json:"lineage,omitempty"`
	Extra        map[string]string `json:"extra"`
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
	if capabilities == nil {
		capabilities = []string{}
	}
	if depends == nil {
		depends = []string{}
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

func newBlockedSummary(tasks map[uint64]*store.Task) blockedSummary {
	blocked := dag.NewDAG().GetBlockedTasks(tasks)
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
