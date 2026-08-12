// Package conformance provides the process isolation, bounded event capture,
// evidence, and scoring primitives used by terminal-todo's real-agent
// conformance evaluations. Host-specific command construction and event
// normalization live outside this package.
package conformance

import (
	"context"
	"encoding/json"
	"io/fs"
	"time"
)

const ReportSchemaVersion = "1"

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
	StreamAny    Stream = "any"
)

type EventKind string

const (
	EventJSON EventKind = "json"
	EventText EventKind = "text"
)

// Event is one bounded, redacted line emitted by a host process. Sequence is
// local to a stream, so stdout and stderr remain deterministic even when the
// operating system schedules their readers differently.
type Event struct {
	Sequence uint64          `json:"sequence"`
	Stream   Stream          `json:"stream"`
	Kind     EventKind       `json:"kind"`
	JSON     json.RawMessage `json:"json,omitempty"`
	Text     string          `json:"text,omitempty"`
}

func (e Event) Content() string {
	if e.Kind == EventJSON {
		return string(e.JSON)
	}
	return e.Text
}

type Capture struct {
	Stdout    []Event `json:"stdout"`
	Stderr    []Event `json:"stderr"`
	BytesRead int64   `json:"bytes_read"`
}

func (c Capture) Events(stream Stream) []Event {
	switch stream {
	case StreamStdout:
		return append([]Event(nil), c.Stdout...)
	case StreamStderr:
		return append([]Event(nil), c.Stderr...)
	default:
		events := make([]Event, 0, len(c.Stdout)+len(c.Stderr))
		events = append(events, c.Stdout...)
		events = append(events, c.Stderr...)
		return events
	}
}

type FixtureFile struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

type Fixture struct {
	Files []FixtureFile
}

// Command is executed directly, without a shell. The placeholders
// {workspace} and {prompt} are expanded in args, environment values, and
// stdin. Environment inherits the runner process unless CleanEnv is true.
type Command struct {
	Executable string
	Args       []string
	Env        map[string]string
	Stdin      string
	Prompt     string
	CleanEnv   bool
}

type FailureDisposition string

const (
	DispositionSkip FailureDisposition = "skip"
	DispositionFail FailureDisposition = "fail"
)

type FailureKind string

const (
	FailurePreflight      FailureKind = "preflight"
	FailureStart          FailureKind = "start"
	FailureTimeout        FailureKind = "timeout"
	FailureCancelled      FailureKind = "cancelled"
	FailureExit           FailureKind = "exit"
	FailureAuthentication FailureKind = "authentication"
	FailureApproval       FailureKind = "approval"
	FailureOutputLimit    FailureKind = "output_limit"
	FailureNormalization  FailureKind = "normalization"
	FailureSession        FailureKind = "session"
)

// FailureRule lets a host adapter classify vendor-specific auth, approval, or
// launch output without coupling the core runner to a particular host.
type FailureRule struct {
	Kind        FailureKind
	Disposition FailureDisposition
	Stream      Stream
	Contains    string
}

type Host struct {
	Name               string
	Version            string
	Model              string
	Transport          string
	IntegrationVersion string
	Preflight          *Command
	Run                Command
	Resume             ResumeCommand
	ExtractSessionID   SessionIDExtractor
	FailureRules       []FailureRule
}

// ResumeCommand constructs a direct argv invocation for a persisted host
// session. Implementations must not invoke a shell.
type ResumeCommand func(sessionID, prompt string) (Command, error)

// SessionIDExtractor inspects one raw machine-readable output line. The
// runner retains a matched identifier only long enough to construct a resume
// command; raw identifiers never enter report captures or evidence.
type SessionIDExtractor func(Stream, []byte) (string, bool)

type Limits struct {
	Timeout        time.Duration
	MaxOutputBytes int64
	MaxEventBytes  int
}

type Evaluation struct {
	ID            string
	Host          Host
	Fixture       Fixture
	Limits        Limits
	Redactions    []string
	Normalizer    Normalizer
	Assertions    []Assertion
	MinimumScore  float64
	KeepWorkspace bool
}

type SequenceAction string

const (
	SequencePrompt  SequenceAction = "prompt"
	SequenceResume  SequenceAction = "resume"
	SequenceHarness SequenceAction = "harness"
)

// SequenceStep is one ordered host turn or harness-controlled transition.
// Prompt starts a fresh session for Actor; Resume continues that actor's
// recorded session. Harness steps never contact a model.
type SequenceStep struct {
	ID      string
	Actor   string
	Action  SequenceAction
	Prompt  string
	Harness func(context.Context, string) error
}

// SequenceEvaluation executes multiple isolated actor sessions against one
// disposable project. It intentionally excludes concurrency; concurrent
// orchestration composes on this ordered foundation separately.
type SequenceEvaluation struct {
	ID            string
	Host          Host
	Fixture       Fixture
	Steps         []SequenceStep
	Limits        Limits
	Redactions    []string
	Normalizer    SequenceNormalizer
	Assertions    []Assertion
	MinimumScore  float64
	KeepWorkspace bool
}

type ProcessResult struct {
	ExitCode  int  `json:"exit_code"`
	TimedOut  bool `json:"timed_out"`
	Cancelled bool `json:"cancelled"`
}

type ExecutionResult struct {
	Process ProcessResult `json:"process"`
	Capture Capture       `json:"capture"`
}

type TurnResult struct {
	ID        string          `json:"id"`
	Actor     string          `json:"actor"`
	Action    SequenceAction  `json:"action"`
	Execution ExecutionResult `json:"execution"`
}

// SequenceNormalizer converts actor-attributed turn captures and final
// fixture state into the stable evidence model.
type SequenceNormalizer interface {
	NormalizeSequence(context.Context, string, []TurnResult) (Evidence, error)
}

type SequenceNormalizerFunc func(context.Context, string, []TurnResult) (Evidence, error)

func (f SequenceNormalizerFunc) NormalizeSequence(ctx context.Context, workspace string, turns []TurnResult) (Evidence, error) {
	return f(ctx, workspace, turns)
}

type InfrastructureFailure struct {
	Kind        FailureKind        `json:"kind"`
	Disposition FailureDisposition `json:"disposition"`
	Phase       string             `json:"phase"`
	Detail      string             `json:"detail"`
}

type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

type Report struct {
	SchemaVersion          string                  `json:"schema_version"`
	ScenarioID             string                  `json:"scenario_id"`
	Host                   string                  `json:"host"`
	HostVersion            string                  `json:"host_version,omitempty"`
	Model                  string                  `json:"model,omitempty"`
	IntegrationVersion     string                  `json:"integration_version,omitempty"`
	Transport              string                  `json:"transport,omitempty"`
	Status                 Status                  `json:"status"`
	Workspace              string                  `json:"workspace,omitempty"`
	Preflight              *ExecutionResult        `json:"preflight,omitempty"`
	Execution              *ExecutionResult        `json:"execution,omitempty"`
	Turns                  []TurnResult            `json:"turns,omitempty"`
	Evidence               Evidence                `json:"evidence"`
	Checks                 []CheckResult           `json:"checks"`
	Score                  Score                   `json:"score"`
	InfrastructureFailures []InfrastructureFailure `json:"infrastructure_failures"`
}

// Normalizer converts host-specific captured events and the resulting fixture
// state into the stable evidence model consumed by assertions.
// Implementations receive already-redacted data and may inspect the disposable
// workspace before Runner removes it.
type Normalizer interface {
	Normalize(context.Context, string, Capture) (Evidence, error)
}

type NormalizerFunc func(context.Context, string, Capture) (Evidence, error)

func (f NormalizerFunc) Normalize(ctx context.Context, workspace string, capture Capture) (Evidence, error) {
	return f(ctx, workspace, capture)
}
