package cli

import (
	"encoding/json"
	"fmt"

	"github.com/bharat94/terminal-todo/store"
	"strings"
)

// DecomposePayload is the --into document. The CLI spells the capability field
// "caps" and the JSON-RPC surface spells it "capabilities"; both are published
// and neither can move, so the CLI shape is converted into the shared one
// rather than aliased onto it.
type DecomposePayload struct {
	Subtasks []decomposePayloadSubtask `json:"subtasks"`
}

type decomposePayloadSubtask struct {
	Title string   `json:"title"`
	Caps  []string `json:"caps"`
}

func (p DecomposePayload) subtasks() []decomposeSubtask {
	subtasks := make([]decomposeSubtask, 0, len(p.Subtasks))
	for _, sub := range p.Subtasks {
		subtasks = append(subtasks, decomposeSubtask{Title: sub.Title, Capabilities: sub.Caps})
	}
	return subtasks
}

type decomposeEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	Parent        protocolTask   `json:"parent"`
	Subtasks      []protocolTask `json:"subtasks"`
}

func cmdDecompose(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	var payloadStr string
	for i, arg := range args {
		if arg == "--into" && i+1 < len(args) {
			payloadStr = args[i+1]
			break
		}
	}

	if payloadStr == "" {
		fail(ErrInvalidArgs, "--into <json> is required")
	}

	var payload DecomposePayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		fail(ErrInvalidArgs, "invalid JSON: %v", err)
	}
	if len(payload.Subtasks) == 0 {
		fail(ErrInvalidArgs, "at least one subtask is required")
	}
	if len(payload.Subtasks) > maxMutationReceiptIDs {
		fail(ErrInvalidArgs, "at most %d subtasks can be created in one decomposition", maxMutationReceiptIDs)
	}
	for i := range payload.Subtasks {
		payload.Subtasks[i].Title = strings.TrimSpace(payload.Subtasks[i].Title)
		payload.Subtasks[i].Caps = normalizePersistedValues(payload.Subtasks[i].Caps)
		if err := validateRequiredPersistedString("subtask title", payload.Subtasks[i].Title, maxTaskTitleBytes); err != nil {
			fail(ErrInvalidArgs, "%v", err)
		}
		if err := validateCapabilities(payload.Subtasks[i].Caps); err != nil {
			fail(ErrInvalidArgs, "subtask %d: %v", i+1, err)
		}
	}

	parentID := ids[0]
	agent := optionValue(args, "--as")
	if err := validateActor(agent, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	var parent *store.Task
	var added []*store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		var decomposeErr error
		parent, added, decomposeErr = decomposeTask(s, parentID, agent, payload.subtasks())
		return decomposeErr
	})
	if receiptRequested(args) {
		ids := make([]uint64, 0, len(added))
		for _, subtask := range added {
			ids = append(ids, subtask.ID)
		}
		receipt := newMutationReceipt("decompose", ids)
		receipt.Task = newMutationTaskReference(parent)
		receipt.DetailFollowUp = lineageDetailFollowUp(parent.ID)
		writeJSON(receipt)
		return
	}
	if hasFlag(args, "--json") {
		subtasks := make([]protocolTask, 0, len(added))
		for _, subtask := range added {
			subtasks = append(subtasks, newProtocolTask(subtask))
		}
		writeJSON(decomposeEnvelope{
			SchemaVersion: protocolVersion,
			Parent:        newProtocolTask(parent),
			Subtasks:      subtasks,
		})
		return
	}
	fmt.Printf("Decomposing task %d into %d subtasks...\n", parentID, len(payload.Subtasks))
	for _, subTask := range added {
		fmt.Printf("  Added subtask %d: %s\n", subTask.ID, subTask.Title)
	}
	fmt.Println("Decomposition complete.")
}
