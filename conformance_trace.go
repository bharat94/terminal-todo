package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/lock"
)

func recordConformanceToolCall(method string, arguments json.RawMessage, result any, callErr *rpcError) {
	path := strings.TrimSpace(os.Getenv(conformance.ConformanceTraceEnvironment))
	if path == "" {
		return
	}
	record := conformance.TraceRecord{
		Actor:     strings.TrimSpace(os.Getenv(conformance.ConformanceActorEnvironment)),
		Operation: strings.TrimPrefix(method, "todo."),
		Arguments: map[string]any{},
	}
	_ = json.Unmarshal(arguments, &record.Arguments)
	if record.Actor == "" {
		record.Actor = traceActorFromArguments(record.Arguments)
	}
	if callErr != nil {
		record.Error = &conformance.DomainError{
			Code:    conformanceDomainErrorCode(callErr.Code),
			Message: callErr.Message,
			Data:    traceObject(callErr.Data),
		}
	} else {
		record.Result = traceObject(result)
	}
	_ = appendConformanceTrace(path, record)
}

func appendConformanceTrace(path string, record conformance.TraceRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lk, err := lock.Open(path)
	if err != nil {
		return err
	}
	defer lk.Close()
	if err := lk.AcquireWithTimeout(lock.Write, 5*time.Second); err != nil {
		return err
	}
	defer lk.Release()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(record)
}

func traceActorFromArguments(arguments map[string]any) string {
	for _, key := range []string{"actor", "as"} {
		if value, ok := arguments[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func traceObject(value any) map[string]any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) == nil {
		return object
	}
	return map[string]any{"value": value}
}

func conformanceDomainErrorCode(code int) string {
	switch code {
	case rpcTaskNotFound:
		return string(ErrTaskNotFound)
	case rpcNotInitialized:
		return string(ErrNotInitialized)
	case rpcCycleDetected:
		return string(ErrCycleDetected)
	case rpcAlreadyClaimed:
		return string(ErrAlreadyClaimed)
	case rpcNotOwner:
		return string(ErrNotOwner)
	case rpcDependency:
		return string(ErrDependency)
	case rpcStoreCorrupted:
		return string(ErrStoreCorrupted)
	case rpcLockContention:
		return string(ErrLockContention)
	case rpcSchemaVersion:
		return string(ErrSchemaVersion)
	case rpcNoWork:
		return string(ErrNoWork)
	case rpcAgentCapacity:
		return string(ErrAgentAtCapacity)
	case rpcIdempotencyConflict:
		return string(ErrIdempotencyConflict)
	case rpcLeaseNotActive:
		return string(ErrLeaseNotActive)
	default:
		return string(ErrInvalidArgs)
	}
}
