package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileCatalogAssertionsExecutesEveryCatalogExpectation(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)
	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			runtime, err := materializeCatalogFixture(scenario, "/opt/todo")
			require.NoError(t, err)
			assertions, err := compileCatalogAssertions(scenario, runtime)
			require.NoError(t, err)
			require.Len(t, assertions, len(scenario.Assertions))
			evidence := conformingCatalogEvidence(scenario.ID, runtime)
			observation := conformance.Observation{Evidence: evidence}
			for _, assertion := range assertions {
				passed, detail := assertion.Evaluate(observation)
				assert.True(t, passed, "%s: %s", assertion.ID, detail)
				assert.NotEmpty(t, assertion.Criteria)
			}
		})
	}
}

func TestConformingCatalogEvidenceGradesAtFullSuiteScore(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)
	checks := []conformance.CheckResult{}
	for _, scenario := range catalog.Scenarios {
		runtime, err := materializeCatalogFixture(scenario, "/opt/todo")
		require.NoError(t, err)
		assertions, err := compileCatalogAssertions(scenario, runtime)
		require.NoError(t, err)
		observation := conformance.Observation{Evidence: conformingCatalogEvidence(scenario.ID, runtime)}
		for _, assertion := range assertions {
			passed, detail := assertion.Evaluate(observation)
			checks = append(checks, conformance.CheckResult{
				ID: scenario.ID + "/" + assertion.ID, Passed: passed, Detail: detail,
				Criteria: assertion.Criteria, HardGate: assertion.HardGate,
			})
		}
	}
	score, err := conformance.Grade(catalog.ScoringModel, checks)
	require.NoError(t, err)
	assert.Equal(t, float64(100), score.RawScore)
	assert.Equal(t, float64(100), score.CappedScore)
	assert.Equal(t, "conformant", score.Level)
	assert.Empty(t, score.HardGateFailures)
}

func TestCatalogHardGateAssertionsRejectUnsafeBehavior(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)

	tests := []struct {
		scenarioID string
		assertion  string
		mutate     func(*conformance.Evidence)
	}{
		{
			scenarioID: "atomic_acquire", assertion: "only_atomic_acquire",
			mutate: func(evidence *conformance.Evidence) {
				evidence.Operations = append([]conformance.Operation{{Actor: "eval-alpha", Operation: "next"}, {Actor: "eval-alpha", Operation: "claim"}}, evidence.Operations...)
			},
		},
		{
			scenarioID: "handoff", assertion: "finding_before_release",
			mutate: func(evidence *conformance.Evidence) {
				evidence.Operations[0].Arguments["extra"] = map[string]any{"finding": "forgotten"}
			},
		},
		{
			scenarioID: "heartbeat", assertion: "renews_before_mutation",
			mutate: func(evidence *conformance.Evidence) {
				for index := range evidence.Operations {
					evidence.Operations[index].Timestamp = time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC).Format(time.RFC3339Nano)
				}
			},
		},
		{
			scenarioID: "no_work", assertion: "no_fabrication",
			mutate: func(evidence *conformance.Evidence) {
				evidence.Operations = append(evidence.Operations, conformance.Operation{Actor: "eval-no-work", Operation: "add"})
			},
		},
		{
			scenarioID: "cleanup", assertion: "no_abandoned_lease",
			mutate: func(evidence *conformance.Evidence) {
				evidence.Tasks["task:work"] = catalogTaskState{ID: "1", Status: "in_progress", Owner: "eval-cleanup", Extra: map[string]string{}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.scenarioID+"/"+test.assertion, func(t *testing.T) {
			scenario := catalogScenarioByID(t, catalog, test.scenarioID)
			runtime, err := materializeCatalogFixture(scenario, "/opt/todo")
			require.NoError(t, err)
			assertions, err := compileCatalogAssertions(scenario, runtime)
			require.NoError(t, err)
			evidence := conformingCatalogEvidence(scenario.ID, runtime)
			test.mutate(&evidence)
			assertion := catalogAssertionByID(t, assertions, test.assertion)
			passed, _ := assertion.Evaluate(conformance.Observation{Evidence: evidence})
			assert.False(t, passed)
			assert.NotEmpty(t, assertion.HardGate)
		})
	}
}

func TestCatalogNormalizerJoinsTraceStoreAndActorAttributedMessages(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)
	scenario := catalogScenarioByID(t, catalog, "atomic_acquire")
	runtime, err := materializeCatalogFixture(scenario, "/opt/todo")
	require.NoError(t, err)
	workspace := t.TempDir()
	for _, file := range runtime.Fixture.Files {
		path := filepath.Join(workspace, filepath.FromSlash(file.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, file.Content, file.Mode))
	}
	require.NoError(t, appendConformanceTrace(filepath.Join(workspace, filepath.FromSlash(conformance.ConformanceTraceFile)), conformance.TraceRecord{
		Actor: "eval-alpha", Operation: "acquire", Timestamp: scenario.InitialTime.Format(time.RFC3339Nano),
		Arguments: map[string]any{"requestId": "alpha"}, Result: map[string]any{"task": map[string]any{"id": 1}},
	}))
	message := json.RawMessage(`{"type":"item.completed","item":{"type":"agent_message","text":"Alpha acquired the work."}}`)
	turns := []conformance.TurnResult{{
		ID: "race:eval-alpha", Actor: "eval-alpha", Action: conformance.SequenceConcurrent,
		Execution: conformance.ExecutionResult{Capture: conformance.Capture{Stdout: []conformance.Event{{Kind: conformance.EventJSON, JSON: message}}}},
	}}
	evidence, err := catalogNormalizer("codex", runtime).NormalizeSequence(context.Background(), workspace, turns)
	require.NoError(t, err)
	require.Len(t, evidence.Operations, 1)
	assert.Equal(t, "alpha", evidence.Operations[0].Arguments["requestId"])
	assert.Contains(t, evidence.Tasks, "task:work")
	assert.Equal(t, []string{"Alpha acquired the work."}, evidence.AssistantMessages)
	assert.Equal(t, []conformance.AssistantMessage{{Actor: "eval-alpha", Text: "Alpha acquired the work."}}, evidence.AssistantTurns)
}

func TestCatalogNormalizerRejectsMissingTraceAfterPersistedMutation(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)
	scenario := catalogScenarioByID(t, catalog, "atomic_acquire")
	runtime, err := materializeCatalogFixture(scenario, "/opt/todo")
	require.NoError(t, err)
	workspace := t.TempDir()
	for _, file := range runtime.Fixture.Files {
		path := filepath.Join(workspace, filepath.FromSlash(file.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, file.Content, file.Mode))
	}
	storePath := filepath.Join(workspace, ".terminal-todo", "tasks.bin")
	taskStore, err := store.Load(storePath)
	require.NoError(t, err)
	taskStore.Tasks[1].Status = store.StatusInProgress
	taskStore.Tasks[1].Owner = "eval-alpha"
	taskStore.AddEvent(store.EventTaskClaimed, 1, "eval-alpha", nil)
	require.NoError(t, taskStore.Save(storePath))

	_, err = catalogNormalizer("codex", runtime).NormalizeSequence(context.Background(), workspace, nil)
	require.ErrorContains(t, err, "operation trace is missing after persisted task mutations")
}

func conformingCatalogEvidence(scenarioID string, runtime catalogFixtureRuntime) conformance.Evidence {
	evidence := conformance.EmptyEvidence(conformance.Capture{})
	task := func(ref, status, owner string, lease uint64) {
		evidence.Tasks[ref] = catalogTaskState{ID: runtime.TaskIDs[ref], Status: status, Owner: owner, LeaseExpires: lease, Extra: map[string]string{}}
	}
	op := func(actor, operation string, arguments, result map[string]any) {
		evidence.Operations = append(evidence.Operations, conformance.Operation{Actor: actor, Operation: operation, Transport: "mcp", Arguments: arguments, Result: result})
	}
	message := func(actor, text string) {
		evidence.AssistantMessages = append(evidence.AssistantMessages, text)
		evidence.AssistantTurns = append(evidence.AssistantTurns, conformance.AssistantMessage{Actor: actor, Text: text})
	}
	switch scenarioID {
	case "discovery":
		op("eval-discovery", "bootstrap", nil, nil)
		op("eval-discovery", "acquire", map[string]any{"requestId": "discovery"}, map[string]any{"task": map[string]any{"id": float64(1)}})
		message("eval-discovery", "Started the available parser work.")
	case "bootstrap":
		op("eval-bootstrap", "bootstrap", nil, nil)
		op("eval-bootstrap", "acquire", map[string]any{"requestId": "bootstrap"}, map[string]any{"task": map[string]any{"id": float64(1)}})
	case "atomic_acquire":
		op("eval-alpha", "acquire", map[string]any{"requestId": "alpha"}, map[string]any{"task": map[string]any{"id": float64(1)}})
		op("eval-beta", "acquire", map[string]any{"requestId": "beta"}, nil)
		task("task:work", "in_progress", "eval-alpha", 0)
	case "heartbeat":
		op("eval-heartbeat", "heartbeat", map[string]any{"id": float64(1)}, nil)
		op("eval-heartbeat", "update", map[string]any{"id": float64(1), "extra": map[string]any{"finding": "progress"}}, nil)
		for index := range evidence.Operations {
			evidence.Operations[index].Timestamp = time.Date(2026, 1, 1, 12, 1, 30, 0, time.UTC).Format(time.RFC3339Nano)
		}
		task("task:work", "in_progress", "eval-heartbeat", uint64(time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC).UnixMilli()))
	case "handoff":
		op("eval-author", "update", map[string]any{"id": float64(1), "extra": map[string]any{"finding": "retain the last valid checksum"}}, nil)
		op("eval-author", "release", map[string]any{"id": float64(1)}, nil)
		op("eval-successor", "acquire", map[string]any{"requestId": "successor"}, map[string]any{"task": map[string]any{"id": float64(1)}})
		message("eval-successor", "The inherited constraint is to retain the last valid checksum.")
	case "no_work":
		op("eval-no-work", "acquire", map[string]any{"requestId": "no-work"}, nil)
		evidence.Errors = append(evidence.Errors, conformance.DomainError{Code: "NO_WORK", Operation: "acquire"})
	case "lease_recovery":
		op("eval-recovery", "acquire", map[string]any{"requestId": "recovery"}, map[string]any{"task": map[string]any{"id": float64(1)}})
	case "quiet_narration":
		message("eval-quiet", "Corrected the documentation typo and verified the result.")
	case "cleanup":
		op("eval-cleanup", "acquire", map[string]any{"requestId": "cleanup"}, map[string]any{"task": map[string]any{"id": float64(1)}})
		op("eval-cleanup", "complete", map[string]any{"id": float64(1)}, nil)
		task("task:work", "completed", "", 0)
	}
	return evidence
}

func catalogAssertionByID(t *testing.T, assertions []conformance.Assertion, id string) conformance.Assertion {
	t.Helper()
	for _, assertion := range assertions {
		if assertion.ID == id {
			return assertion
		}
	}
	t.Fatalf("missing assertion %q", id)
	return conformance.Assertion{}
}
