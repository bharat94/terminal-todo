package main

import (
	"fmt"
	"sort"

	"github.com/bharat94/terminal-todo/store"
)

const maxMutationReceiptIDs = 20

// mutationReceipt is the bounded acknowledgement returned when a caller opts
// into compact mutation output. Full task state remains available through the
// existing detail reads and legacy JSON results.
type mutationReceipt struct {
	SchemaVersion  string                 `json:"schema_version"`
	Operation      string                 `json:"operation"`
	Task           *mutationTaskReference `json:"task,omitempty"`
	Affected       mutationAffected       `json:"affected"`
	Replayed       *bool                  `json:"replayed,omitempty"`
	DetailFollowUp mutationDetailFollowUp `json:"detail_follow_up"`
}

type mutationTaskReference struct {
	ID           uint64  `json:"id"`
	Status       string  `json:"status,omitempty"`
	LeaseExpires *string `json:"lease_expires,omitempty"`
	RetryCount   uint32  `json:"retry_count,omitempty"`
}

type mutationAffected struct {
	Total     int      `json:"total"`
	IDs       []uint64 `json:"ids"`
	Truncated bool     `json:"truncated"`
}

type mutationDetailFollowUp struct {
	Available    bool   `json:"available"`
	CLI          string `json:"cli,omitempty"`
	NativeMethod string `json:"native_method,omitempty"`
	MCPTool      string `json:"mcp_tool,omitempty"`
	Guidance     string `json:"guidance,omitempty"`
}

func newMutationReceipt(operation string, ids []uint64) mutationReceipt {
	return mutationReceipt{
		SchemaVersion: protocolVersion,
		Operation:     operation,
		Affected:      newMutationAffected(ids),
		DetailFollowUp: mutationDetailFollowUp{
			Available: false,
			Guidance:  "full detail is unavailable after this mutation; inspect or export state before running it",
		},
	}
}

func newTaskMutationReceipt(operation string, task *store.Task) mutationReceipt {
	receipt := newMutationReceipt(operation, []uint64{task.ID})
	receipt.Task = newMutationTaskReference(task)
	receipt.DetailFollowUp = taskDetailFollowUp(task.ID)
	return receipt
}

func newMutationTaskReference(task *store.Task) *mutationTaskReference {
	reference := &mutationTaskReference{
		ID:         task.ID,
		Status:     statusName(task.Status),
		RetryCount: task.RetryCount,
	}
	if task.LeaseExpires > 0 {
		expires := formatTimestamp(task.LeaseExpires)
		reference.LeaseExpires = &expires
	}
	return reference
}

func newProtocolTaskMutationReceipt(operation string, task protocolTask) mutationReceipt {
	receipt := newMutationReceipt(operation, []uint64{task.ID})
	receipt.Task = &mutationTaskReference{
		ID:           task.ID,
		Status:       task.Status,
		LeaseExpires: task.Metadata.LeaseExpires,
		RetryCount:   task.Metadata.RetryCount,
	}
	receipt.DetailFollowUp = taskDetailFollowUp(task.ID)
	return receipt
}

func newMutationAffected(ids []uint64) mutationAffected {
	unique := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		unique[id] = struct{}{}
	}
	sorted := make([]uint64, 0, len(unique))
	for id := range unique {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	total := len(sorted)
	if total > maxMutationReceiptIDs {
		sorted = sorted[:maxMutationReceiptIDs]
	}
	bounded := append([]uint64(nil), sorted...)
	if bounded == nil {
		bounded = []uint64{}
	}
	return mutationAffected{
		Total:     total,
		IDs:       bounded,
		Truncated: total > len(bounded),
	}
}

func taskDetailFollowUp(id uint64) mutationDetailFollowUp {
	return mutationDetailFollowUp{
		Available:    true,
		CLI:          fmt.Sprintf("todo cat %d --json", id),
		NativeMethod: "todo.cat",
		MCPTool:      "terminal_todo_cat",
	}
}

func lineageDetailFollowUp(id uint64) mutationDetailFollowUp {
	return mutationDetailFollowUp{
		Available:    true,
		CLI:          fmt.Sprintf("todo lineage %d --json", id),
		NativeMethod: "todo.lineage",
		Guidance:     "MCP callers can read the parent with terminal_todo_cat, then inspect its dependency task IDs",
	}
}

func graphDetailFollowUp() mutationDetailFollowUp {
	return mutationDetailFollowUp{
		Available:    true,
		CLI:          "todo status --json",
		NativeMethod: "todo.status",
		MCPTool:      "terminal_todo_status",
	}
}

func receiptRequested(args []string) bool {
	return hasFlag(args, "--receipt")
}

func uniqueTaskIDs(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	unique := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
