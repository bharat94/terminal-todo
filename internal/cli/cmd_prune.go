package cli

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdPrune(args []string) {
	var removed []*store.Task
	updateStore(func(s *store.TaskStore) error {
		removed = pruneCompletedTasks(s)
		return nil
	})
	if receiptRequested(args) {
		ids := make([]uint64, 0, len(removed))
		for _, task := range removed {
			ids = append(ids, task.ID)
		}
		writeJSON(newMutationReceipt("prune", ids))
		return
	}
	if hasFlag(args, "--json") {
		tasks := make([]protocolTask, 0, len(removed))
		for _, task := range removed {
			tasks = append(tasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: tasks})
		return
	}
	fmt.Printf("Removed %d completed task(s)\n", len(removed))
}
