package conformance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout        = 2 * time.Minute
	defaultMaxOutputBytes = 4 * 1024 * 1024
	defaultMaxEventBytes  = 256 * 1024
)

type Runner struct {
	TempRoot string
}

func (r Runner) Run(ctx context.Context, evaluation Evaluation) (Report, error) {
	report := newReport(evaluation)
	if err := validateEvaluation(evaluation); err != nil {
		return report, err
	}

	workspace, err := os.MkdirTemp(r.TempRoot, "terminal-todo-conformance-*")
	if err != nil {
		return report, fmt.Errorf("create conformance workspace: %w", err)
	}
	if evaluation.KeepWorkspace {
		report.Workspace = workspace
	} else {
		defer os.RemoveAll(workspace)
	}
	if err := materializeFixture(workspace, evaluation.Fixture); err != nil {
		return report, err
	}

	limits := normalizedLimits(evaluation.Limits)
	redactions := append([]string(nil), evaluation.Redactions...)
	redactions = append(redactions, workspace, evaluation.Host.Run.Prompt)
	redact := newRedactor(redactions)
	if evaluation.Host.Preflight != nil {
		preflight, failure := runCommand(ctx, workspace, *evaluation.Host.Preflight, limits, redact)
		report.Preflight = &preflight
		if failure != nil {
			failure.Phase = "preflight"
			if failure.Kind == FailureStart {
				failure.Disposition = DispositionSkip
			}
			report.InfrastructureFailures = append(report.InfrastructureFailures, *failure)
			report.Status = statusForFailures(report.InfrastructureFailures)
			return report, nil
		}
		if preflight.Process.ExitCode != 0 {
			failure := classifyFailure(
				"preflight",
				preflight,
				evaluation.Host.FailureRules,
				InfrastructureFailure{
					Kind:        FailurePreflight,
					Disposition: DispositionSkip,
					Phase:       "preflight",
					Detail:      fmt.Sprintf("preflight exited with code %d", preflight.Process.ExitCode),
				},
			)
			report.InfrastructureFailures = append(report.InfrastructureFailures, failure)
			report.Status = statusForFailures(report.InfrastructureFailures)
			return report, nil
		}
		if failure, matched := matchFailureRule("preflight", preflight, evaluation.Host.FailureRules); matched {
			report.InfrastructureFailures = append(report.InfrastructureFailures, failure)
			report.Status = statusForFailures(report.InfrastructureFailures)
			return report, nil
		}
	}

	execution, failure := runCommand(ctx, workspace, evaluation.Host.Run, limits, redact)
	report.Execution = &execution
	if failure != nil {
		failure.Phase = "host"
		report.InfrastructureFailures = append(report.InfrastructureFailures, *failure)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return report, nil
	}
	if execution.Process.ExitCode != 0 {
		failure := classifyFailure(
			"host",
			execution,
			evaluation.Host.FailureRules,
			InfrastructureFailure{
				Kind:        FailureExit,
				Disposition: DispositionFail,
				Phase:       "host",
				Detail:      fmt.Sprintf("host exited with code %d", execution.Process.ExitCode),
			},
		)
		report.InfrastructureFailures = append(report.InfrastructureFailures, failure)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return report, nil
	}
	if failure, matched := matchFailureRule("host", execution, evaluation.Host.FailureRules); matched {
		report.InfrastructureFailures = append(report.InfrastructureFailures, failure)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return report, nil
	}

	evidence := EmptyEvidence(execution.Capture)
	if evaluation.Normalizer != nil {
		evidence, err = evaluation.Normalizer.Normalize(ctx, workspace, execution.Capture)
		if err != nil {
			report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
				Kind:        FailureNormalization,
				Disposition: DispositionFail,
				Phase:       "normalization",
				Detail:      redact.text(err.Error()),
			})
			report.Status = StatusError
			return report, nil
		}
		evidence = normalizeEvidence(evidence)
	}
	report.Evidence = evidence

	observation := Observation{
		Process:  execution.Process,
		Capture:  execution.Capture,
		Evidence: evidence,
	}
	report.Checks, report.Score, _ = evaluateAssertions(evaluation.Assertions, observation)
	requiredPassed := true
	for _, check := range report.Checks {
		if check.Required && !check.Passed {
			requiredPassed = false
			break
		}
	}
	minimum := evaluation.MinimumScore
	if minimum == 0 {
		minimum = 100
	}
	if requiredPassed && report.Score.Percent >= minimum {
		report.Status = StatusPassed
	} else {
		report.Status = StatusFailed
	}
	return report, nil
}

func newReport(evaluation Evaluation) Report {
	return Report{
		SchemaVersion:          ReportSchemaVersion,
		ScenarioID:             evaluation.ID,
		Host:                   evaluation.Host.Name,
		HostVersion:            evaluation.Host.Version,
		Model:                  evaluation.Host.Model,
		IntegrationVersion:     evaluation.Host.IntegrationVersion,
		Transport:              evaluation.Host.Transport,
		Status:                 StatusError,
		Evidence:               EmptyEvidence(Capture{}),
		Checks:                 []CheckResult{},
		Score:                  Score{Scored: false},
		InfrastructureFailures: []InfrastructureFailure{},
	}
}

func normalizedLimits(limits Limits) Limits {
	if limits.Timeout == 0 {
		limits.Timeout = defaultTimeout
	}
	if limits.MaxOutputBytes == 0 {
		limits.MaxOutputBytes = defaultMaxOutputBytes
	}
	if limits.MaxEventBytes == 0 {
		limits.MaxEventBytes = defaultMaxEventBytes
	}
	return limits
}

func validateEvaluation(evaluation Evaluation) error {
	switch {
	case strings.TrimSpace(evaluation.ID) == "":
		return errors.New("conformance evaluation ID is required")
	case strings.TrimSpace(evaluation.Host.Name) == "":
		return errors.New("conformance host name is required")
	case strings.TrimSpace(evaluation.Host.Run.Executable) == "":
		return errors.New("conformance host executable is required")
	case evaluation.Limits.Timeout < 0:
		return errors.New("conformance timeout cannot be negative")
	case evaluation.Limits.MaxOutputBytes < 0:
		return errors.New("conformance maximum output cannot be negative")
	case evaluation.Limits.MaxEventBytes < 0:
		return errors.New("conformance maximum event size cannot be negative")
	case evaluation.MinimumScore < 0 || evaluation.MinimumScore > 100:
		return errors.New("conformance minimum score must be between 0 and 100")
	}
	ids := make(map[string]struct{}, len(evaluation.Assertions))
	for _, assertion := range evaluation.Assertions {
		if strings.TrimSpace(assertion.ID) == "" {
			return errors.New("conformance assertion ID is required")
		}
		if _, exists := ids[assertion.ID]; exists {
			return fmt.Errorf("duplicate conformance assertion ID %q", assertion.ID)
		}
		ids[assertion.ID] = struct{}{}
		if assertion.Weight <= 0 {
			return fmt.Errorf("conformance assertion %q weight must be positive", assertion.ID)
		}
		if assertion.Evaluate == nil {
			return fmt.Errorf("conformance assertion %q evaluator is required", assertion.ID)
		}
	}
	for _, rule := range evaluation.Host.FailureRules {
		if rule.Contains == "" {
			return errors.New("conformance failure rule pattern is required")
		}
		if rule.Disposition != DispositionSkip && rule.Disposition != DispositionFail {
			return fmt.Errorf("invalid conformance failure disposition %q", rule.Disposition)
		}
	}
	return nil
}

func materializeFixture(root string, fixture Fixture) error {
	files := append([]FixtureFile(nil), fixture.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		clean := filepath.Clean(file.Path)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("fixture path %q must stay within the workspace", file.Path)
		}
		target := filepath.Join(root, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create fixture directory for %q: %w", file.Path, err)
		}
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, file.Content, mode); err != nil {
			return fmt.Errorf("write fixture %q: %w", file.Path, err)
		}
	}
	return nil
}

func runCommand(
	parent context.Context,
	workspace string,
	spec Command,
	limits Limits,
	redact redactor,
) (ExecutionResult, *InfrastructureFailure) {
	ctx, cancel := context.WithTimeout(parent, limits.Timeout)
	defer cancel()

	executable := expand(spec.Executable, workspace, spec.Prompt)
	args := make([]string, len(spec.Args))
	for i, arg := range spec.Args {
		args[i] = expand(arg, workspace, spec.Prompt)
	}
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workspace
	command.WaitDelay = 2 * time.Second
	command.Stdin = strings.NewReader(expand(spec.Stdin, workspace, spec.Prompt))
	command.Env = commandEnvironment(spec, workspace)

	stdout, err := command.StdoutPipe()
	if err != nil {
		return ExecutionResult{}, &InfrastructureFailure{
			Kind: FailureStart, Disposition: DispositionFail, Detail: redact.text(err.Error()),
		}
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return ExecutionResult{}, &InfrastructureFailure{
			Kind: FailureStart, Disposition: DispositionFail, Detail: redact.text(err.Error()),
		}
	}
	if err := command.Start(); err != nil {
		return ExecutionResult{}, &InfrastructureFailure{
			Kind: FailureStart, Disposition: DispositionFail, Detail: redact.text(err.Error()),
		}
	}

	collector := newEventCollector(limits.MaxOutputBytes, redact)
	readErrors := make(chan error, 2)
	go func() { readErrors <- collector.read(stdout, StreamStdout, limits.MaxEventBytes, cancel) }()
	go func() { readErrors <- collector.read(stderr, StreamStderr, limits.MaxEventBytes, cancel) }()

	firstReadError := <-readErrors
	secondReadError := <-readErrors
	// StdoutPipe and StderrPipe require their readers to reach EOF before Wait
	// closes the descriptors. Reversing this order intermittently turns a
	// successful short-lived process into a platform-specific "file already
	// closed" capture error.
	waitErr := command.Wait()
	capture := collector.capture()
	result := ExecutionResult{
		Process: ProcessResult{ExitCode: exitCode(waitErr)},
		Capture: capture,
	}

	readErr := firstNonNil(firstReadError, secondReadError)
	if readErr != nil {
		return result, &InfrastructureFailure{
			Kind:        FailureOutputLimit,
			Disposition: DispositionFail,
			Detail:      redact.text(readErr.Error()),
		}
	}
	if ctx.Err() != nil {
		result.Process.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.Process.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		kind := FailureTimeout
		detail := "host command timed out"
		if result.Process.Cancelled {
			kind = FailureCancelled
			detail = "host command was cancelled"
		}
		return result, &InfrastructureFailure{
			Kind: kind, Disposition: DispositionFail, Detail: detail,
		}
	}
	return result, nil
}

func commandEnvironment(spec Command, workspace string) []string {
	var environment []string
	if !spec.CleanEnv {
		environment = append(environment, os.Environ()...)
	}
	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+expand(spec.Env[key], workspace, spec.Prompt))
	}
	environment = append(environment, "TERMINAL_TODO_CONFORMANCE_WORKSPACE="+workspace)
	return environment
}

func expand(value, workspace, prompt string) string {
	value = strings.ReplaceAll(value, "{workspace}", workspace)
	return strings.ReplaceAll(value, "{prompt}", prompt)
}

func exitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

type eventCollector struct {
	mu         sync.Mutex
	maxBytes   int64
	bytesRead  int64
	stdout     []Event
	stderr     []Event
	redact     redactor
	limitError error
}

func newEventCollector(maxBytes int64, redact redactor) *eventCollector {
	return &eventCollector{
		maxBytes: maxBytes,
		stdout:   []Event{},
		stderr:   []Event{},
		redact:   redact,
	}
}

func (c *eventCollector) read(reader io.Reader, stream Stream, maxEventBytes int, cancel context.CancelFunc) error {
	scanner := bufio.NewScanner(reader)
	initial := 64 * 1024
	if maxEventBytes < initial {
		initial = maxEventBytes
	}
	scanner.Buffer(make([]byte, initial), maxEventBytes)
	var sequence uint64
	for scanner.Scan() {
		sequence++
		if err := c.add(stream, sequence, scanner.Bytes()); err != nil {
			cancel()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		cancel()
		return fmt.Errorf("host %s event exceeded %d bytes: %w", stream, maxEventBytes, err)
	}
	return nil
}

func (c *eventCollector) add(stream Stream, sequence uint64, line []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bytesRead += int64(len(line))
	if c.bytesRead > c.maxBytes {
		if c.limitError == nil {
			c.limitError = fmt.Errorf("host output exceeded %d bytes", c.maxBytes)
		}
		return c.limitError
	}
	event := c.redact.event(line, stream, sequence)
	if stream == StreamStdout {
		c.stdout = append(c.stdout, event)
	} else {
		c.stderr = append(c.stderr, event)
	}
	return nil
}

func (c *eventCollector) capture() Capture {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Capture{
		Stdout:    append([]Event{}, c.stdout...),
		Stderr:    append([]Event{}, c.stderr...),
		BytesRead: c.bytesRead,
	}
}

func classifyFailure(
	phase string,
	execution ExecutionResult,
	rules []FailureRule,
	fallback InfrastructureFailure,
) InfrastructureFailure {
	if failure, matched := matchFailureRule(phase, execution, rules); matched {
		return failure
	}
	return fallback
}

func matchFailureRule(
	phase string,
	execution ExecutionResult,
	rules []FailureRule,
) (InfrastructureFailure, bool) {
	for _, rule := range rules {
		transcript := Observation{Capture: execution.Capture}.Transcript(rule.Stream)
		if strings.Contains(strings.ToLower(transcript), strings.ToLower(rule.Contains)) {
			return InfrastructureFailure{
				Kind:        rule.Kind,
				Disposition: rule.Disposition,
				Phase:       phase,
				Detail:      fmt.Sprintf("host output matched %s failure rule", rule.Kind),
			}, true
		}
	}
	return InfrastructureFailure{}, false
}

func statusForFailures(failures []InfrastructureFailure) Status {
	for _, failure := range failures {
		if failure.Disposition == DispositionFail {
			return StatusFailed
		}
	}
	return StatusSkipped
}

func normalizeEvidence(evidence Evidence) Evidence {
	if evidence.Operations == nil {
		evidence.Operations = []Operation{}
	}
	if evidence.Tasks == nil {
		evidence.Tasks = map[string]any{}
	}
	if evidence.Events == nil {
		evidence.Events = []json.RawMessage{}
	}
	if evidence.Errors == nil {
		evidence.Errors = []DomainError{}
	}
	if evidence.AssistantMessages == nil {
		evidence.AssistantMessages = []string{}
	}
	if evidence.HostEvents == nil {
		evidence.HostEvents = []Event{}
	}
	return evidence
}

func firstNonNil(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
