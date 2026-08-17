package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

type ErrorCode string

const (
	ErrTaskNotFound        ErrorCode = "TASK_NOT_FOUND"
	ErrNotInitialized      ErrorCode = "NOT_INITIALIZED"
	ErrInvalidArgs         ErrorCode = "INVALID_ARGS"
	ErrCycleDetected       ErrorCode = "CYCLE_DETECTED"
	ErrAlreadyClaimed      ErrorCode = "ALREADY_CLAIMED"
	ErrNotOwner            ErrorCode = "NOT_OWNER"
	ErrDependency          ErrorCode = "DEPENDENCY_ERROR"
	ErrStoreCorrupted      ErrorCode = "STORE_CORRUPTED"
	ErrLockContention      ErrorCode = "LOCK_CONTENTION"
	ErrSchemaVersion       ErrorCode = "SCHEMA_VERSION"
	ErrNoWork              ErrorCode = "NO_WORK"
	ErrAgentAtCapacity     ErrorCode = "AGENT_AT_CAPACITY"
	ErrIdempotencyConflict ErrorCode = "IDEMPOTENCY_CONFLICT"
	ErrLeaseNotActive      ErrorCode = "LEASE_NOT_ACTIVE"
	ErrInvalidTransition   ErrorCode = "INVALID_TRANSITION"
)

type ErrorResponse struct {
	Code    ErrorCode   `json:"code"`
	Message string      `json:"message"`
	Details string      `json:"details,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type errorEnvelope struct {
	SchemaVersion string        `json:"schema_version"`
	Error         ErrorResponse `json:"error"`
}

// exitProcess ends the process with a status. Command handlers report failure
// by calling fail, which does not return, so the only way to observe a
// command's failure is to intercept the exit. Tests replace this hook with one
// that panics and recover it, which is what makes commands runnable in process
// rather than only through a built binary.
//
// The hook must never return in either mode: callers rely on fail being
// terminal, and several of them would otherwise fall through and report a
// second, wrong error.
var exitProcess = func(status int) { os.Exit(status) }

func fail(code ErrorCode, msg string, args ...interface{}) {
	message := fmt.Sprintf(msg, args...)
	failDetails(code, message, "")
}

func failDetails(code ErrorCode, message, details string) {
	failWithData(code, message, details, nil)
}

func failData(code ErrorCode, message string, data interface{}) {
	failWithData(code, message, "", data)
}

func failWithData(code ErrorCode, message, details string, data interface{}) {
	args := os.Args[1:]
	if hasFlag(args, "--json") || hasFlag(args, "--receipt") {
		output, err := json.MarshalIndent(errorEnvelope{
			SchemaVersion: protocolVersion,
			Error: ErrorResponse{
				Code:    code,
				Message: message,
				Details: details,
				Data:    data,
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
	exitProcess(exitCode(code))
}

func exitCode(code ErrorCode) int {
	switch code {
	case ErrTaskNotFound, ErrInvalidArgs, ErrNotOwner, ErrDependency, ErrInvalidTransition:
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
	case ErrIdempotencyConflict:
		return 8
	case ErrLeaseNotActive:
		return 9
	default:
		return 1
	}
}

// writeJSON writes a JSON-serializable value to stdout.
func writeJSON(v interface{}) {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fail(ErrStoreCorrupted, "failed to encode JSON: %v", err)
	}
	fmt.Println(string(output))
}
