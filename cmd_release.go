package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

func cmdRelease(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}
	owner := optionValue(args, "--as")
	errorMsg := optionValue(args, "--error")

	released := make([]*store.Task, 0, len(ids))
	updateStore(func(s *store.TaskStore) error {
		for _, id := range ids {
			task, ok := s.GetTask(id)
			if !ok {
				return fmt.Errorf("task %d not found", id)
			}
			if task.Status != store.StatusInProgress {
				return fmt.Errorf("task %d is not in progress", id)
			}
			if task.Owner != "" && task.Owner != owner {
				return fmt.Errorf("task %d is claimed by %s; use --as %s", id, task.Owner, task.Owner)
			}
			task.Status = store.StatusPending
			task.RetryCount++
			data := map[string]string{}
			if errorMsg != "" {
				task.LastError = errorMsg
				data["error"] = errorMsg
				s.AddLog(id, owner, fmt.Sprintf("released with error: %s", errorMsg))
			} else {
				s.AddLog(id, owner, "released")
			}
			s.AddEvent(store.EventTaskReleased, id, owner, data)
			task.Owner = ""
			task.LeaseExpires = 0
			released = append(released, task)
		}
		return nil
	})
	if receiptRequested(args) {
		ids := make([]uint64, 0, len(released))
		for _, task := range released {
			ids = append(ids, task.ID)
		}
		receipt := newMutationReceipt("release", ids)
		if len(released) == 1 {
			receipt.Task = newMutationTaskReference(released[0])
			receipt.DetailFollowUp = taskDetailFollowUp(released[0].ID)
		} else {
			receipt.DetailFollowUp = graphDetailFollowUp()
		}
		writeJSON(receipt)
		return
	}
	if hasFlag(args, "--json") {
		protocolTasks := make([]protocolTask, 0, len(released))
		for _, task := range released {
			protocolTasks = append(protocolTasks, newProtocolTask(task))
		}
		writeJSON(tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks})
		return
	}
	for _, id := range ids {
		fmt.Printf("Released task %d\n", id)
	}
}
