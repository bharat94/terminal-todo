package conformance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// RunSequence executes ordered fresh, resumed, and harness-controlled turns
// against one disposable project workspace.
func (r Runner) RunSequence(ctx context.Context, evaluation SequenceEvaluation) (Report, error) {
	report := newSequenceReport(evaluation)
	if err := validateSequenceEvaluation(evaluation); err != nil {
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
	sequenceCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	redactions := append([]string(nil), evaluation.Redactions...)
	redactions = append(redactions, workspace)
	for _, step := range evaluation.Steps {
		if step.Action == SequenceConcurrent {
			for _, actor := range step.Actors {
				redactions = append(redactions, concurrentActorPrompt(step.Prompt, actor))
			}
		} else {
			redactions = append(redactions, step.Prompt)
		}
	}
	redact := newRedactor(redactions)
	if evaluation.Host.Preflight != nil {
		preflight, failure := runCommand(sequenceCtx, workspace, *evaluation.Host.Preflight, limits, redact)
		report.Preflight = &preflight
		if failedSequenceCommand(&report, "preflight", preflight, failure, evaluation.Host.FailureRules, true) {
			return report, nil
		}
	}

	sessions := make(map[string]string)
	var capturedBytes int64
	for _, step := range evaluation.Steps {
		if step.Action == SequenceHarness {
			if err := step.Harness(sequenceCtx, workspace); err != nil {
				report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
					Kind: FailureNormalization, Disposition: DispositionFail, Phase: "harness:" + step.ID,
					Detail: redact.text(err.Error()),
				})
				report.Status = StatusError
				return report, nil
			}
			continue
		}
		if step.Action == SequenceConcurrent {
			remaining := limits.MaxOutputBytes - capturedBytes
			if remaining <= 0 {
				report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
					Kind: FailureOutputLimit, Disposition: DispositionFail, Phase: "host:" + step.ID,
					Detail: fmt.Sprintf("scenario output exceeded %d bytes", limits.MaxOutputBytes),
				})
				report.Status = StatusError
				return report, nil
			}
			outcomes := runConcurrentStep(sequenceCtx, workspace, evaluation.Host, step, limits, remaining, redact)
			for _, outcome := range outcomes {
				report.Turns = append(report.Turns, outcome.turn)
				capturedBytes += outcome.turn.Execution.Capture.BytesRead
				if outcome.sessionID != "" {
					sessions[outcome.turn.Actor] = outcome.sessionID
				}
			}
			if failConcurrentStep(&report, step.ID, outcomes, evaluation.Host.FailureRules) {
				return report, nil
			}
			continue
		}

		var command Command
		switch step.Action {
		case SequencePrompt:
			command = evaluation.Host.Run
			command.Prompt = step.Prompt
		case SequenceResume:
			command, err = evaluation.Host.Resume(sessions[step.Actor], step.Prompt)
			if err != nil {
				return report, fmt.Errorf("build resume command for step %q: %w", step.ID, err)
			}
		}
		command.Env = mergeEnvironment(evaluation.Host.Run.Env, command.Env)
		command.Env[ConformanceActorEnvironment] = step.Actor

		turnLimits := limits
		turnLimits.MaxOutputBytes = limits.MaxOutputBytes - capturedBytes
		if turnLimits.MaxOutputBytes <= 0 {
			report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
				Kind: FailureOutputLimit, Disposition: DispositionFail, Phase: "host:" + step.ID,
				Detail: fmt.Sprintf("scenario output exceeded %d bytes", limits.MaxOutputBytes),
			})
			report.Status = StatusError
			return report, nil
		}
		var extractedSessionID string
		var sessionMu sync.Mutex
		var observe func(Stream, []byte)
		if step.Action == SequencePrompt && evaluation.Host.ExtractSessionID != nil {
			observe = func(stream Stream, line []byte) {
				sessionMu.Lock()
				defer sessionMu.Unlock()
				if extractedSessionID != "" {
					return
				}
				if sessionID, matched := evaluation.Host.ExtractSessionID(stream, line); matched {
					extractedSessionID = sessionID
				}
			}
		}
		execution, failure := runCommandObserved(sequenceCtx, workspace, command, turnLimits, redact, observe)
		capturedBytes += execution.Capture.BytesRead
		report.Turns = append(report.Turns, TurnResult{ID: step.ID, Actor: step.Actor, Action: step.Action, Execution: execution})
		if failedSequenceCommand(&report, "host:"+step.ID, execution, failure, evaluation.Host.FailureRules, false) {
			return report, nil
		}
		if step.Action == SequencePrompt && evaluation.Host.ExtractSessionID != nil {
			sessionMu.Lock()
			sessionID := extractedSessionID
			sessionMu.Unlock()
			if sessionID == "" {
				report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
					Kind: FailureSession, Disposition: DispositionFail, Phase: "session:" + step.ID,
					Detail: "host stream did not include a resumable session identifier",
				})
				report.Status = StatusError
				return report, nil
			}
			sessions[step.Actor] = sessionID
		}
	}

	aggregated := aggregateTurnExecutions(report.Turns)
	report.Execution = &aggregated
	evidence := EmptyEvidence(aggregated.Capture)
	if evaluation.Normalizer != nil {
		evidence, err = evaluation.Normalizer.NormalizeSequence(sequenceCtx, workspace, report.Turns)
		if err != nil {
			report.InfrastructureFailures = append(report.InfrastructureFailures, InfrastructureFailure{
				Kind: FailureNormalization, Disposition: DispositionFail, Phase: "normalization",
				Detail: redact.text(err.Error()),
			})
			report.Status = StatusError
			return report, nil
		}
		evidence = normalizeEvidence(evidence)
	}
	report.Evidence = evidence
	gradeSequenceReport(&report, evaluation.Assertions, evaluation.MinimumScore, aggregated, evidence)
	return report, nil
}

func validateSequenceEvaluation(evaluation SequenceEvaluation) error {
	base := Evaluation{ID: evaluation.ID, Host: evaluation.Host, Fixture: evaluation.Fixture, Limits: evaluation.Limits, Assertions: evaluation.Assertions, MinimumScore: evaluation.MinimumScore}
	if err := validateEvaluation(base); err != nil {
		return err
	}
	if len(evaluation.Steps) == 0 {
		return errors.New("conformance sequence requires at least one step")
	}
	seen := make(map[string]struct{}, len(evaluation.Steps))
	started := make(map[string]bool)
	for _, step := range evaluation.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return errors.New("conformance sequence step ID is required")
		}
		if _, duplicate := seen[step.ID]; duplicate {
			return fmt.Errorf("duplicate conformance sequence step ID %q", step.ID)
		}
		seen[step.ID] = struct{}{}
		switch step.Action {
		case SequencePrompt:
			if strings.TrimSpace(step.Actor) == "" || strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("prompt step %q requires actor and prompt", step.ID)
			}
			started[step.Actor] = true
		case SequenceResume:
			if strings.TrimSpace(step.Actor) == "" || strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("resume step %q requires actor and prompt", step.ID)
			}
			if !started[step.Actor] {
				return fmt.Errorf("resume step %q has no fresh session for actor %q", step.ID, step.Actor)
			}
			if evaluation.Host.Resume == nil || evaluation.Host.ExtractSessionID == nil {
				return fmt.Errorf("resume step %q requires host session support", step.ID)
			}
		case SequenceHarness:
			if step.Harness == nil {
				return fmt.Errorf("harness step %q requires an action", step.ID)
			}
		case SequenceConcurrent:
			if len(step.Actors) < 2 || strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("concurrent step %q requires at least two actors and a prompt", step.ID)
			}
			actors := make(map[string]struct{}, len(step.Actors))
			for _, actor := range step.Actors {
				if strings.TrimSpace(actor) == "" {
					return fmt.Errorf("concurrent step %q contains an empty actor", step.ID)
				}
				if _, duplicate := actors[actor]; duplicate {
					return fmt.Errorf("concurrent step %q repeats actor %q", step.ID, actor)
				}
				actors[actor] = struct{}{}
				started[actor] = true
			}
		default:
			return fmt.Errorf("sequence step %q has invalid action %q", step.ID, step.Action)
		}
	}
	return nil
}

type concurrentTurnOutcome struct {
	turn        TurnResult
	failure     *InfrastructureFailure
	sessionID   string
	substantive bool
}

const ConformanceActorEnvironment = "TERMINAL_TODO_CONFORMANCE_ACTOR"

func runConcurrentStep(
	ctx context.Context,
	workspace string,
	host Host,
	step SequenceStep,
	limits Limits,
	remainingOutput int64,
	redact redactor,
) []concurrentTurnOutcome {
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	budget := newOutputBudget(remainingOutput)
	outcomes := make([]concurrentTurnOutcome, len(step.Actors))
	results := make(chan struct {
		index   int
		outcome concurrentTurnOutcome
	}, len(step.Actors))
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(len(step.Actors))

	for index, actor := range step.Actors {
		go func(index int, actor string) {
			command := host.Run
			command.Prompt = concurrentActorPrompt(step.Prompt, actor)
			command.Env = cloneEnvironment(command.Env)
			command.Env[ConformanceActorEnvironment] = actor

			var sessionID string
			var sessionMu sync.Mutex
			var observe func(Stream, []byte)
			if host.ExtractSessionID != nil {
				observe = func(stream Stream, line []byte) {
					sessionMu.Lock()
					defer sessionMu.Unlock()
					if sessionID != "" {
						return
					}
					if extracted, matched := host.ExtractSessionID(stream, line); matched {
						sessionID = extracted
					}
				}
			}

			ready.Done()
			<-start
			execution, failure := runCommandBudgeted(groupCtx, workspace, command, limits, redact, observe, budget)
			sessionMu.Lock()
			extractedSessionID := sessionID
			sessionMu.Unlock()
			if failure == nil && execution.Process.ExitCode == 0 && host.ExtractSessionID != nil && extractedSessionID == "" {
				failure = &InfrastructureFailure{Kind: FailureSession, Disposition: DispositionFail, Detail: "host stream did not include a resumable session identifier"}
			}
			substantive := concurrentCommandSubstantiveFailure(execution, failure, host.FailureRules)
			if substantive {
				cancel()
			}
			results <- struct {
				index   int
				outcome concurrentTurnOutcome
			}{index: index, outcome: concurrentTurnOutcome{
				turn:    TurnResult{ID: step.ID + ":" + actor, Actor: actor, Action: SequenceConcurrent, Execution: execution},
				failure: failure, sessionID: extractedSessionID, substantive: substantive,
			}}
		}(index, actor)
	}
	ready.Wait()
	close(start)
	for range step.Actors {
		result := <-results
		outcomes[result.index] = result.outcome
	}
	return outcomes
}

func concurrentCommandSubstantiveFailure(execution ExecutionResult, failure *InfrastructureFailure, rules []FailureRule) bool {
	if failure != nil {
		return failure.Kind != FailureCancelled
	}
	if execution.Process.ExitCode != 0 {
		return true
	}
	_, matched := matchFailureRule("host", execution, rules)
	return matched
}

func failConcurrentStep(report *Report, stepID string, outcomes []concurrentTurnOutcome, rules []FailureRule) bool {
	for _, substantiveOnly := range []bool{true, false} {
		for _, outcome := range outcomes {
			failed := outcome.failure != nil || outcome.turn.Execution.Process.ExitCode != 0
			if !failed {
				_, failed = matchFailureRule("host", outcome.turn.Execution, rules)
			}
			if !failed || (substantiveOnly && !outcome.substantive) || (!substantiveOnly && outcome.substantive) {
				continue
			}
			phase := "host:" + stepID + ":" + outcome.turn.Actor
			failedSequenceCommand(report, phase, outcome.turn.Execution, outcome.failure, rules, false)
			return true
		}
	}
	return false
}

func cloneEnvironment(environment map[string]string) map[string]string {
	clone := make(map[string]string, len(environment)+1)
	for key, value := range environment {
		clone[key] = value
	}
	return clone
}

func mergeEnvironment(base, override map[string]string) map[string]string {
	merged := cloneEnvironment(base)
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func concurrentActorPrompt(prompt, actor string) string {
	return strings.ReplaceAll(prompt, ConformanceActorPlaceholder, actor)
}

func newSequenceReport(evaluation SequenceEvaluation) Report {
	return Report{
		SchemaVersion: ReportSchemaVersion, ScenarioID: evaluation.ID, Host: evaluation.Host.Name,
		HostVersion: evaluation.Host.Version, Model: evaluation.Host.Model,
		IntegrationVersion: evaluation.Host.IntegrationVersion, Transport: evaluation.Host.Transport,
		Status: StatusError, Evidence: EmptyEvidence(Capture{}), Checks: []CheckResult{},
		Score: Score{Scored: false}, Turns: []TurnResult{}, InfrastructureFailures: []InfrastructureFailure{},
	}
}

func failedSequenceCommand(report *Report, phase string, execution ExecutionResult, failure *InfrastructureFailure, rules []FailureRule, preflight bool) bool {
	if failure != nil {
		failure.Phase = phase
		if preflight && failure.Kind == FailureStart {
			failure.Disposition = DispositionSkip
		}
		report.InfrastructureFailures = append(report.InfrastructureFailures, *failure)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return true
	}
	if execution.Process.ExitCode != 0 {
		disposition, kind := DispositionFail, FailureExit
		if preflight {
			disposition, kind = DispositionSkip, FailurePreflight
		}
		classified := classifyFailure(phase, execution, rules, InfrastructureFailure{Kind: kind, Disposition: disposition, Phase: phase, Detail: fmt.Sprintf("command exited with code %d", execution.Process.ExitCode)})
		report.InfrastructureFailures = append(report.InfrastructureFailures, classified)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return true
	}
	if classified, matched := matchFailureRule(phase, execution, rules); matched {
		report.InfrastructureFailures = append(report.InfrastructureFailures, classified)
		report.Status = statusForFailures(report.InfrastructureFailures)
		return true
	}
	return false
}

func aggregateTurnExecutions(turns []TurnResult) ExecutionResult {
	result := ExecutionResult{Process: ProcessResult{ExitCode: 0}, Capture: Capture{Stdout: []Event{}, Stderr: []Event{}}}
	var stdoutSequence, stderrSequence uint64
	for _, turn := range turns {
		result.Capture.BytesRead += turn.Execution.Capture.BytesRead
		for _, event := range turn.Execution.Capture.Stdout {
			stdoutSequence++
			event.Sequence = stdoutSequence
			result.Capture.Stdout = append(result.Capture.Stdout, event)
		}
		for _, event := range turn.Execution.Capture.Stderr {
			stderrSequence++
			event.Sequence = stderrSequence
			result.Capture.Stderr = append(result.Capture.Stderr, event)
		}
	}
	return result
}

func gradeSequenceReport(report *Report, assertions []Assertion, minimum float64, execution ExecutionResult, evidence Evidence) {
	observation := Observation{Process: execution.Process, Capture: execution.Capture, Evidence: evidence}
	report.Checks, report.Score, _ = evaluateAssertions(assertions, observation)
	requiredPassed := true
	for _, check := range report.Checks {
		if check.Required && !check.Passed {
			requiredPassed = false
			break
		}
	}
	if minimum == 0 {
		minimum = 100
	}
	if requiredPassed && report.Score.Percent >= minimum {
		report.Status = StatusPassed
	} else {
		report.Status = StatusFailed
	}
}
