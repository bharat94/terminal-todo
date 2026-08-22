package cli

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdDone(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}
	owner := optionValue(args, "--as")
	if err := validateActor(owner, false); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}
	preflight := loadStore()
	resolver := snapshotDependencyResolver(preflight.GetAllTasks())

	completed := make([]*store.Task, 0, len(ids))
	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, err := completeTask(s, id, owner, resolver, projectNow())
			if err != nil {
				return err
			}
			completed = append(completed, task)
		}
		return nil
	})
	if receiptRequested(args) {
		completedIDs := make([]uint64, 0, len(completed))
		for _, task := range completed {
			completedIDs = append(completedIDs, task.ID)
		}
		receipt := newMutationReceipt("complete", completedIDs)
		receipt.DetailFollowUp = graphDetailFollowUp()
		if len(completed) == 1 {
			receipt.Task = newMutationTaskReference(completed[0])
			receipt.DetailFollowUp = taskDetailFollowUp(completed[0].ID)
		}
		writeJSON(receipt)
		return
	}
	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(completed))
		for _, task := range completed {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks})
		return
	}
	for _, id := range ids {
		fmt.Printf("Marked task %d as done\n", id)
	}
}
