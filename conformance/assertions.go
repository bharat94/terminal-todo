package conformance

import (
	"fmt"
	"strings"
)

type Observation struct {
	Process  ProcessResult
	Capture  Capture
	Evidence Evidence
}

func (o Observation) Transcript(stream Stream) string {
	events := o.Capture.Events(stream)
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, event.Content())
	}
	return strings.Join(lines, "\n")
}

type Assertion struct {
	ID          string
	Description string
	Criteria    []string
	HardGate    string
	Weight      float64
	Required    bool
	Evaluate    func(Observation) (passed bool, detail string)
}

type CheckResult struct {
	ID          string   `json:"id"`
	Description string   `json:"description,omitempty"`
	Passed      bool     `json:"passed"`
	Required    bool     `json:"required"`
	Weight      float64  `json:"weight"`
	Criteria    []string `json:"criteria"`
	HardGate    string   `json:"hard_gate,omitempty"`
	Detail      string   `json:"detail,omitempty"`
}

func (a Assertion) WithCriteria(criteria ...string) Assertion {
	a.Criteria = append([]string(nil), criteria...)
	return a
}

func (a Assertion) WithHardGate(hardGate string) Assertion {
	a.HardGate = hardGate
	return a
}

func Contains(id string, stream Stream, value string, weight float64, required bool) Assertion {
	return Assertion{
		ID:          id,
		Description: fmt.Sprintf("%s contains expected text", stream),
		Weight:      weight,
		Required:    required,
		Evaluate: func(o Observation) (bool, string) {
			if strings.Contains(o.Transcript(stream), value) {
				return true, ""
			}
			return false, fmt.Sprintf("%s did not contain %q", stream, value)
		},
	}
}

func Excludes(id string, stream Stream, value string, weight float64, required bool) Assertion {
	return Assertion{
		ID:          id,
		Description: fmt.Sprintf("%s excludes forbidden text", stream),
		Weight:      weight,
		Required:    required,
		Evaluate: func(o Observation) (bool, string) {
			if !strings.Contains(o.Transcript(stream), value) {
				return true, ""
			}
			return false, fmt.Sprintf("%s contained forbidden text %q", stream, value)
		},
	}
}

func Ordered(id string, stream Stream, values []string, weight float64, required bool) Assertion {
	expected := append([]string(nil), values...)
	return Assertion{
		ID:          id,
		Description: fmt.Sprintf("%s contains expected text in order", stream),
		Weight:      weight,
		Required:    required,
		Evaluate: func(o Observation) (bool, string) {
			remaining := o.Transcript(stream)
			for _, value := range expected {
				index := strings.Index(remaining, value)
				if index < 0 {
					return false, fmt.Sprintf("%s did not contain %q in the expected order", stream, value)
				}
				remaining = remaining[index+len(value):]
			}
			return true, ""
		},
	}
}

func EvidenceCheck(
	id, description string,
	weight float64,
	required bool,
	evaluate func(Evidence) (bool, string),
) Assertion {
	return Assertion{
		ID:          id,
		Description: description,
		Weight:      weight,
		Required:    required,
		Evaluate: func(o Observation) (bool, string) {
			return evaluate(o.Evidence)
		},
	}
}
