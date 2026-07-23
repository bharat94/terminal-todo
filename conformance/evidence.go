package conformance

import "encoding/json"

// Evidence is the vendor-neutral record graded by scenario assertions.
// Host adapters may leave fields empty when the host did not expose them.
type Evidence struct {
	Operations        []Operation       `json:"operations"`
	Tasks             map[string]any    `json:"tasks"`
	Events            []json.RawMessage `json:"events"`
	Errors            []DomainError     `json:"errors"`
	AssistantMessages []string          `json:"assistant_messages"`
	HostEvents        []Event           `json:"host_events"`
}

type Operation struct {
	Actor     string         `json:"actor,omitempty"`
	Operation string         `json:"operation"`
	Transport string         `json:"transport,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
}

type DomainError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message,omitempty"`
	Operation string         `json:"operation,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

func EmptyEvidence(capture Capture) Evidence {
	return Evidence{
		Operations:        []Operation{},
		Tasks:             map[string]any{},
		Events:            []json.RawMessage{},
		Errors:            []DomainError{},
		AssistantMessages: []string{},
		HostEvents:        capture.Events(StreamAny),
	}
}
