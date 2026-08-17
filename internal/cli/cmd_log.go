package cli

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdLog(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	message := optionValue(args, "--msg")
	if err := validateRequiredPersistedString("message", message, maxLogMessageBytes); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	owner := optionValue(args, "--as")
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var logged *store.Task
	updateLifecycleStore(func(s *store.TaskStore) error {
		var err error
		logged, err = logTask(s, ids[0], owner, message)
		return err
	})

	if receiptRequested(args) {
		writeJSON(newTaskMutationReceipt("log", logged))
		return
	}
	if hasFlag(args, "--json") {
		writeJSON(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(logged)})
		return
	}
	fmt.Printf("Logged to task %d: %s\n", ids[0], message)
}
