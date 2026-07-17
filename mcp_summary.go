package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxMCPResultSummaryLength = 240

func mcpResultSummary(toolName string, value interface{}, isError bool) string {
	if isError {
		return compactMCPResultSummary(mcpErrorSummary(value))
	}

	var summary string
	switch toolName {
	case "terminal_todo_ping":
		if result, ok := value.(pingResult); ok {
			state := "not initialized"
			if result.Initialized {
				state = "ready"
			}
			summary = fmt.Sprintf("terminal-todo %s (protocol %s).", state, result.ProtocolVersion)
		}
	case "terminal_todo_init":
		summary = "Initialized terminal-todo project state."
	case "terminal_todo_status":
		switch result := value.(type) {
		case tasksEnvelope:
			summary = summarizeMCPTasks(result.Tasks)
		case projectsEnvelope:
			taskCount := 0
			for _, project := range result.Projects {
				taskCount += len(project.Tasks)
			}
			summary = fmt.Sprintf("Returned %d task(s) across %d project(s).", taskCount, len(result.Projects))
		}
	case "terminal_todo_cat":
		if task, ok := value.(protocolTask); ok {
			summary = summarizeMCPTask(task)
		}
	case "terminal_todo_add":
		if result, ok := value.(map[string]interface{}); ok {
			summary = fmt.Sprintf("Added task %v: %v.", result["id"], result["title"])
		}
	case "terminal_todo_acquire":
		if result, ok := value.(acquireEnvelope); ok {
			verb := "Acquired"
			if result.Replayed {
				verb = "Replayed acquisition for"
			}
			summary = fmt.Sprintf("%s task %d: %s.", verb, result.Task.ID, result.Task.Title)
		}
	case "terminal_todo_heartbeat":
		if result, ok := value.(taskEnvelope); ok {
			summary = fmt.Sprintf("Renewed task %d lease.", result.Task.ID)
		}
	case "terminal_todo_update":
		if task, ok := value.(protocolTask); ok {
			summary = fmt.Sprintf("Updated task %d.", task.ID)
		}
	case "terminal_todo_log":
		if result, ok := value.(logResult); ok {
			summary = fmt.Sprintf("Recorded a note on task %d.", result.ID)
		}
	case "terminal_todo_decompose":
		if result, ok := value.(decomposeResult); ok {
			summary = fmt.Sprintf("Decomposed task %d into %d subtask(s).", result.Parent.ID, len(result.Subtasks))
		}
	case "terminal_todo_block":
		if result, ok := value.(blockResult); ok {
			summary = fmt.Sprintf("Blocked task %d.", result.ID)
		}
	case "terminal_todo_release":
		if result, ok := value.(releaseResult); ok {
			summary = fmt.Sprintf("Released task %d.", result.ID)
		}
	case "terminal_todo_complete":
		if result, ok := value.(map[string]interface{}); ok {
			if ids, ok := result["completed"].([]uint64); ok {
				summary = fmt.Sprintf("Completed %d task(s): %s.", len(ids), joinTaskIDs(ids))
			}
		}
	case "terminal_todo_events":
		summary = "Returned coordination event history."
	}
	if summary == "" {
		summary = "terminal-todo operation completed."
	}
	return compactMCPResultSummary(summary)
}

func summarizeMCPTasks(tasks []protocolTask) string {
	counts := map[string]int{}
	for _, task := range tasks {
		counts[task.Status]++
	}
	return fmt.Sprintf(
		"%d task(s): %d pending, %d in progress, %d blocked, %d completed.",
		len(tasks),
		counts["pending"],
		counts["in_progress"],
		counts["blocked"],
		counts["completed"],
	)
}

func summarizeMCPTask(task protocolTask) string {
	return fmt.Sprintf("Task %d [%s]: %s.", task.ID, task.Status, task.Title)
}

func joinTaskIDs(ids []uint64) string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(values, ", ")
}

func mcpErrorSummary(value interface{}) string {
	result, ok := value.(map[string]interface{})
	if !ok {
		return "terminal-todo operation failed."
	}
	message, _ := result["message"].(string)
	code, _ := result["code"].(int)
	name := mcpErrorName(code)
	if name == "" {
		name = "ERROR"
	}
	if message == "" {
		return name + "."
	}
	return fmt.Sprintf("%s: %s", name, message)
}

func mcpErrorName(code int) string {
	names := map[int]ErrorCode{
		rpcInvalidParams:       ErrInvalidArgs,
		rpcTaskNotFound:        ErrTaskNotFound,
		rpcNotInitialized:      ErrNotInitialized,
		rpcCycleDetected:       ErrCycleDetected,
		rpcAlreadyClaimed:      ErrAlreadyClaimed,
		rpcNotOwner:            ErrNotOwner,
		rpcDependency:          ErrDependency,
		rpcStoreCorrupted:      ErrStoreCorrupted,
		rpcLockContention:      ErrLockContention,
		rpcSchemaVersion:       ErrSchemaVersion,
		rpcNoWork:              ErrNoWork,
		rpcAgentCapacity:       ErrAgentAtCapacity,
		rpcIdempotencyConflict: ErrIdempotencyConflict,
		rpcLeaseNotActive:      ErrLeaseNotActive,
	}
	return string(names[code])
}

func compactMCPResultSummary(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maxMCPResultSummaryLength {
		return value
	}
	end := maxMCPResultSummaryLength - 3
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "..."
}
