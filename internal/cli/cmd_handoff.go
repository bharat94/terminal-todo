package cli

import (
	"errors"
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdHandoff(args []string) {
	ids := parseIDs(args)
	if len(ids) != 1 {
		fail(ErrInvalidArgs, "exactly one task ID is required")
	}
	actor := optionValue(args, "--as")
	if err := validateActor(actor, true); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	extra, err := parseExtraUpdates(args)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	if len(extra) == 0 {
		fail(ErrInvalidArgs, "at least one --set key=value handoff field is required")
	}
	if err := validateExtra(extra); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var handedOff *store.Task
	_, err = store.Update(tasksBinPath(), func(s *store.TaskStore) error {
		var handoffErr error
		handedOff, handoffErr = handoffTask(s, ids[0], actor, extra, projectNow())
		return handoffErr
	})
	if err != nil {
		switch {
		case isPersistedInputFailure(err):
			fail(ErrInvalidArgs, "%v", err)
		case errors.Is(err, errLeaseTaskNotFound):
			fail(ErrTaskNotFound, "%v", err)
		case errors.Is(err, errLeaseNotOwner):
			fail(ErrNotOwner, "%v", err)
		case errors.Is(err, errLeaseNotActive):
			fail(ErrLeaseNotActive, "%v", err)
		default:
			fail(ErrStoreCorrupted, "%v", err)
		}
	}

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("handoff", handedOff))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(handedOff)})
		return
	}
	fmt.Printf("Handed off task %d\n", handedOff.ID)
}
