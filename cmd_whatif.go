package main

import (
	"fmt"

	"github.com/bharat94/terminal-todo/dag"

	"github.com/bharat94/terminal-todo/store"
)

type whatifItem struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

type whatifDone struct {
	WouldUnblock []whatifItem `json:"would_unblock"`
	StillBlocked int          `json:"still_blocked"`
}

type whatifBlocked struct {
	WouldBlock int `json:"would_block"`
}

type whatifEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	TaskID        uint64         `json:"task_id"`
	Title         string         `json:"title"`
	IfDone        *whatifDone    `json:"if_done,omitempty"`
	IfBlocked     *whatifBlocked `json:"if_blocked,omitempty"`
}

func cmdWhatIf(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fail(ErrInvalidArgs, "task ID required")
	}

	filter := ""
	for _, arg := range args {
		if arg == "--done" || arg == "--claim" || arg == "--block" {
			filter = arg[2:]
		}
	}

	s := loadStore()
	task, ok := s.GetTask(ids[0])
	if !ok {
		fail(ErrTaskNotFound, "task %d not found", ids[0])
	}

	var newlyReady []*store.Task
	stillBlocked := 0
	if filter == "" || filter == "done" {
		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)

		simTasks := make(map[uint64]*store.Task)
		for k, v := range s.Tasks {
			t := *v
			t.Depends = append([]string(nil), v.Depends...)
			t.Capabilities = append([]string(nil), v.Capabilities...)
			t.Tags = append([]string(nil), v.Tags...)
			t.Log = append([]store.LogEntry(nil), v.Log...)
			simTasks[k] = &t
		}
		if simTask, ok := simTasks[ids[0]]; ok {
			simTask.Status = store.StatusCompleted
		}

		resolver := dependencyResolver()
		ready := d.GetReadyTasksWithResolver(simTasks, resolver)
		blocked := d.GetBlockedTasksWithResolver(simTasks, resolver)
		beforeBlocked := d.GetBlockedTasksWithResolver(s.Tasks, resolver)

		for _, t := range ready {
			if _, wasBlocked := beforeBlocked[t.ID]; wasBlocked {
				newlyReady = append(newlyReady, t)
			}
		}
		stillBlocked = len(blocked)
	}

	wouldBlock := 0
	if filter == "" || filter == "block" {
		wouldBlock = countDependents(s.Tasks, ids[0])
	}

	if hasFlag(args, "--json") {
		var ifDone *whatifDone
		var ifBlocked *whatifBlocked
		if filter == "" || filter == "done" {
			items := make([]whatifItem, 0, len(newlyReady))
			for _, t := range newlyReady {
				items = append(items, whatifItem{ID: t.ID, Title: t.Title})
			}
			ifDone = &whatifDone{WouldUnblock: items, StillBlocked: stillBlocked}
		}
		if filter == "" || filter == "block" {
			ifBlocked = &whatifBlocked{WouldBlock: wouldBlock}
		}
		writeJSON(whatifEnvelope{
			SchemaVersion: protocolVersion,
			TaskID:        task.ID,
			Title:         task.Title,
			IfDone:        ifDone,
			IfBlocked:     ifBlocked,
		})
		return
	}

	fmt.Printf("What-if analysis for task %d: %s\n\n", task.ID, task.Title)

	if filter == "" || filter == "done" {
		fmt.Println("If marked DONE:")
		if len(newlyReady) > 0 {
			fmt.Printf("  Would unblock %d task(s):\n", len(newlyReady))
			for _, t := range newlyReady {
				fmt.Printf("    - %d: %s\n", t.ID, t.Title)
			}
		} else {
			if stillBlocked > 0 {
				fmt.Printf("  No tasks would be unblocked (%d task(s) still blocked)\n", stillBlocked)
			} else {
				fmt.Println("  No impact on other tasks")
			}
		}
	}

	if filter == "" || filter == "block" {
		fmt.Println()
		fmt.Println("If BLOCKED:")
		fmt.Printf("  Would block %d dependent(s)\n", wouldBlock)
	}
}

func countDependents(tasks map[uint64]*store.Task, id uint64) int {
	count := 0
	for _, t := range tasks {
		for _, dep := range t.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local && depID == id {
				count++
			}
		}
	}
	return count
}
