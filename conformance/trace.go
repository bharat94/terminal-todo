package conformance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharat94/terminal-todo/lock"
)

const (
	ConformanceTraceEnvironment = "TERMINAL_TODO_CONFORMANCE_TRACE"
	ConformanceTraceFile        = ".terminal-todo/conformance-trace.jsonl"
)

// TraceRecord is one synthetic-project coordination call captured at the MCP
// boundary. It is enabled only for opt-in conformance workspaces.
type TraceRecord struct {
	Actor     string         `json:"actor,omitempty"`
	Operation string         `json:"operation"`
	Timestamp string         `json:"timestamp,omitempty"`
	Arguments map[string]any `json:"arguments"`
	Result    map[string]any `json:"result,omitempty"`
	Error     *DomainError   `json:"error,omitempty"`
}

// ReadTrace loads a conformance workspace's normalized operation and domain
// error evidence in recorded call order.
func ReadTrace(workspace string) ([]Operation, []DomainError, error) {
	path := filepath.Join(workspace, filepath.FromSlash(ConformanceTraceFile))
	lk, err := lock.Open(path)
	if err == nil {
		_ = lk.AcquireWithTimeout(lock.Read, 5*time.Second)
		defer func() { _ = lk.Release(); _ = lk.Close() }()
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Operation{}, []DomainError{}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open conformance trace: %w", err)
	}
	defer file.Close()

	operations := []Operation{}
	errorsFound := []DomainError{}
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		var record TraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, nil, fmt.Errorf("parse conformance trace line %d: %w", line, err)
		}
		operations = append(operations, Operation{
			Actor: record.Actor, Operation: record.Operation, Transport: "mcp",
			Timestamp: record.Timestamp, Arguments: record.Arguments, Result: record.Result,
		})
		if record.Error != nil {
			domainError := *record.Error
			domainError.Operation = record.Operation
			errorsFound = append(errorsFound, domainError)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read conformance trace: %w", err)
	}
	return operations, errorsFound, nil
}
