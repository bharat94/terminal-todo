package main

import (
	"fmt"

	"terminal-todo/dag"
	"terminal-todo/store"
)

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

	fmt.Printf("What-if analysis for task %d: %s\n\n", task.ID, task.Title)

	if filter == "" || filter == "done" {
		fmt.Println("If marked DONE:")
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
		var newlyReady []*store.Task
		for _, t := range ready {
			if _, wasBlocked := beforeBlocked[t.ID]; wasBlocked {
				newlyReady = append(newlyReady, t)
			}
		}

		if len(newlyReady) > 0 {
			fmt.Printf("  Would unblock %d task(s):\n", len(newlyReady))
			for _, t := range newlyReady {
				fmt.Printf("    - %d: %s\n", t.ID, t.Title)
			}
		} else {
			stillBlocked := len(blocked)
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
		fmt.Printf("  Would block %d dependent(s)\n", countDependents(s.Tasks, ids[0]))
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
