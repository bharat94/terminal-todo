package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type ErrorCode string

const (
	ErrTaskNotFound    ErrorCode = "TASK_NOT_FOUND"
	ErrNotInitialized  ErrorCode = "NOT_INITIALIZED"
	ErrInvalidArgs     ErrorCode = "INVALID_ARGS"
	ErrCycleDetected   ErrorCode = "CYCLE_DETECTED"
	ErrAlreadyClaimed  ErrorCode = "ALREADY_CLAIMED"
	ErrNotOwner        ErrorCode = "NOT_OWNER"
	ErrDependency      ErrorCode = "DEPENDENCY_ERROR"
	ErrStoreCorrupted  ErrorCode = "STORE_CORRUPTED"
	ErrLockContention  ErrorCode = "LOCK_CONTENTION"
	ErrSchemaVersion   ErrorCode = "SCHEMA_VERSION"
	ErrNoWork          ErrorCode = "NO_WORK"
	ErrAgentAtCapacity ErrorCode = "AGENT_AT_CAPACITY"
)

type ErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
}

type errorEnvelope struct {
	SchemaVersion string        `json:"schema_version"`
	Error         ErrorResponse `json:"error"`
}

func fail(code ErrorCode, msg string, args ...interface{}) {
	message := fmt.Sprintf(msg, args...)
	failDetails(code, message, "")
}

func failDetails(code ErrorCode, message, details string) {
	args := os.Args[1:]
	if hasFlag(args, "--json") {
		output, err := json.MarshalIndent(errorEnvelope{
			SchemaVersion: protocolVersion,
			Error: ErrorResponse{
				Code:    code,
				Message: message,
				Details: details,
			},
		}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		} else {
			fmt.Fprintln(os.Stderr, string(output))
		}
	} else {
		if details != "" {
			fmt.Fprintf(os.Stderr, "Error: %s (%s)\n", message, details)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		}
	}
	os.Exit(exitCode(code))
}

func exitCode(code ErrorCode) int {
	switch code {
	case ErrTaskNotFound, ErrInvalidArgs, ErrNotOwner, ErrDependency:
		return 1
	case ErrNotInitialized, ErrStoreCorrupted, ErrSchemaVersion:
		return 2
	case ErrLockContention:
		return 3
	case ErrCycleDetected:
		return 4
	case ErrAlreadyClaimed:
		return 5
	case ErrNoWork:
		return 6
	case ErrAgentAtCapacity:
		return 7
	default:
		return 1
	}
}

// isJSONOutput returns true when either --json appears anywhere in os.Args
// or when the first positional argument contains --json (helpful for tests).
func isJSONOutput() bool {
	for _, arg := range os.Args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

// extractFlagValue returns the value for a given flag from an args slice.
func extractFlagValue(args []string, flag string) string {
	if args == nil {
		return ""
	}
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// writeJSON writes a JSON-serializable value to stdout.
func writeJSON(v interface{}) {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fail(ErrStoreCorrupted, "failed to encode JSON: %v", err)
	}
	fmt.Println(string(output))
}
