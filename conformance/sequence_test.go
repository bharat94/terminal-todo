package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSequenceKeepsFreshActorsSeparateAndResumesExplicitActor(t *testing.T) {
	host := Host{
		Name: "fake-host",
		Run:  sequenceHelperCommand("fresh"),
		ExtractSessionID: func(stream Stream, line []byte) (string, bool) {
			if stream != StreamStdout {
				return "", false
			}
			var value map[string]string
			if json.Unmarshal(line, &value) == nil && value["session_id"] != "" {
				return value["session_id"], true
			}
			return "", false
		},
	}
	host.Resume = func(sessionID, prompt string) (Command, error) {
		command := sequenceHelperCommand("resume")
		command.Env["FAKE_SESSION_ID"] = sessionID
		command.Prompt = prompt
		return command, nil
	}

	harnessRan := false
	evaluation := SequenceEvaluation{
		ID:      "multi_turn",
		Host:    host,
		Fixture: Fixture{Files: []FixtureFile{{Path: "state.txt", Content: []byte("start"), Mode: 0o600}}},
		Steps: []SequenceStep{
			{ID: "alpha-start", Actor: "alpha", Action: SequencePrompt, Prompt: "start alpha"},
			{ID: "advance", Action: SequenceHarness, Harness: func(_ context.Context, workspace string) error {
				harnessRan = true
				return os.WriteFile(filepath.Join(workspace, "state.txt"), []byte("advanced"), 0o600)
			}},
			{ID: "alpha-resume", Actor: "alpha", Action: SequenceResume, Prompt: "continue alpha"},
			{ID: "beta-start", Actor: "beta", Action: SequencePrompt, Prompt: "start beta"},
		},
		Normalizer: SequenceNormalizerFunc(func(_ context.Context, workspace string, turns []TurnResult) (Evidence, error) {
			evidence := EmptyEvidence(aggregateTurnExecutions(turns).Capture)
			for _, turn := range turns {
				evidence.AssistantMessages = append(evidence.AssistantMessages, turn.Actor+":"+Observation{Capture: turn.Execution.Capture}.Transcript(StreamStdout))
			}
			return evidence, nil
		}),
		Assertions: []Assertion{EvidenceCheck("turns", "all turns retain actor identity", 1, true, func(evidence Evidence) (bool, string) {
			joined := strings.Join(evidence.AssistantMessages, "\n")
			return strings.Contains(joined, "alpha") && strings.Contains(joined, "beta") && strings.Contains(joined, "advanced"), joined
		})},
	}

	report, err := (Runner{}).RunSequence(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusPassed, report.Status)
	assert.True(t, harnessRan)
	require.Len(t, report.Turns, 3)
	assert.Equal(t, []string{"alpha", "alpha", "beta"}, []string{report.Turns[0].Actor, report.Turns[1].Actor, report.Turns[2].Actor})
	assert.Equal(t, SequenceResume, report.Turns[1].Action)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "session-start")
	assert.Contains(t, string(encoded), `"session_id":"\u003credacted\u003e"`)
}

func TestRunSequenceRequiresFreshSessionBeforeResume(t *testing.T) {
	evaluation := SequenceEvaluation{
		ID:         "invalid_resume",
		Host:       Host{Name: "fake", Run: sequenceHelperCommand("fresh"), Resume: func(string, string) (Command, error) { return Command{}, nil }, ExtractSessionID: func(Stream, []byte) (string, bool) { return "id", true }},
		Steps:      []SequenceStep{{ID: "resume", Actor: "alpha", Action: SequenceResume, Prompt: "continue"}},
		Assertions: []Assertion{Contains("unused", StreamStdout, "ok", 1, true)},
	}
	_, err := (Runner{}).RunSequence(context.Background(), evaluation)
	assert.ErrorContains(t, err, "no fresh session")
}

func TestRunSequenceUsesOneScenarioTimeout(t *testing.T) {
	evaluation := SequenceEvaluation{
		ID: "sequence_timeout", Host: Host{Name: "fake", Run: sequenceHelperCommand("wait")},
		Steps:  []SequenceStep{{ID: "one", Actor: "a", Action: SequencePrompt, Prompt: "one"}, {ID: "two", Actor: "b", Action: SequencePrompt, Prompt: "two"}},
		Limits: Limits{Timeout: 100 * time.Millisecond}, Assertions: []Assertion{Contains("unused", StreamStdout, "ok", 1, true)},
	}
	report, err := (Runner{}).RunSequence(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
	require.NotEmpty(t, report.InfrastructureFailures)
	assert.Equal(t, FailureTimeout, report.InfrastructureFailures[0].Kind)
}

func TestRunSequenceStartsConcurrentActorsBehindBarrierAndKeepsReportOrder(t *testing.T) {
	evaluation := SequenceEvaluation{
		ID:   "concurrent_barrier",
		Host: Host{Name: "fake", Run: sequenceHelperCommand("concurrent-barrier")},
		Steps: []SequenceStep{{
			ID: "race", Action: SequenceConcurrent, Actors: []string{"alpha", "beta"}, Prompt: "contend as {actor}",
		}},
		Assertions: []Assertion{
			Contains("alpha", StreamStdout, `"actor":"alpha"`, 1, true),
			Contains("beta", StreamStdout, `"actor":"beta"`, 1, true),
		},
	}

	report, err := (Runner{}).RunSequence(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusPassed, report.Status)
	require.Len(t, report.Turns, 2)
	assert.Equal(t, []string{"alpha", "beta"}, []string{report.Turns[0].Actor, report.Turns[1].Actor})
	assert.Equal(t, []string{"race:alpha", "race:beta"}, []string{report.Turns[0].ID, report.Turns[1].ID})
	for _, turn := range report.Turns {
		assert.Equal(t, SequenceConcurrent, turn.Action)
	}
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "session-alpha")
	assert.NotContains(t, string(encoded), "session-beta")
}

func TestRunSequenceEnforcesOneOutputBudgetAcrossConcurrentActors(t *testing.T) {
	evaluation := SequenceEvaluation{
		ID:   "concurrent_output",
		Host: Host{Name: "fake", Run: sequenceHelperCommand("concurrent-flood")},
		Steps: []SequenceStep{{
			ID: "flood", Action: SequenceConcurrent, Actors: []string{"alpha", "beta"}, Prompt: "emit",
		}},
		Limits: Limits{MaxOutputBytes: 120},
		Assertions: []Assertion{
			Contains("unused", StreamStdout, "x", 1, true),
		},
	}

	report, err := (Runner{}).RunSequence(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureOutputLimit, report.InfrastructureFailures[0].Kind)
	retained := int64(0)
	for _, turn := range report.Turns {
		for _, event := range turn.Execution.Capture.Events(StreamAny) {
			retained += int64(len(event.Content()))
		}
	}
	assert.LessOrEqual(t, retained, int64(120))
}

func TestRunSequenceCancelsConcurrentPeersAfterSubstantiveFailure(t *testing.T) {
	evaluation := SequenceEvaluation{
		ID:   "concurrent_failure",
		Host: Host{Name: "fake", Run: sequenceHelperCommand("concurrent-failure")},
		Steps: []SequenceStep{{
			ID: "fail", Action: SequenceConcurrent, Actors: []string{"beta", "alpha"}, Prompt: "run",
		}},
		Limits: Limits{Timeout: 10 * time.Second},
		Assertions: []Assertion{
			Contains("unused", StreamStdout, "ok", 1, true),
		},
	}

	started := time.Now()
	report, err := (Runner{}).RunSequence(context.Background(), evaluation)
	elapsed := time.Since(started)
	require.NoError(t, err)
	assert.Less(t, elapsed, 3*time.Second)
	assert.Equal(t, StatusFailed, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureExit, report.InfrastructureFailures[0].Kind)
	assert.Equal(t, "host:fail:alpha", report.InfrastructureFailures[0].Phase)
	require.Len(t, report.Turns, 2)
	assert.True(t, report.Turns[0].Execution.Process.Cancelled)
}

func TestRunSequenceRejectsInvalidConcurrentActors(t *testing.T) {
	base := SequenceEvaluation{
		ID: "invalid_concurrent", Host: Host{Name: "fake", Run: sequenceHelperCommand("fresh")},
		Assertions: []Assertion{Contains("unused", StreamStdout, "ok", 1, true)},
	}
	for _, step := range []SequenceStep{
		{ID: "few", Action: SequenceConcurrent, Actors: []string{"alpha"}, Prompt: "run"},
		{ID: "duplicate", Action: SequenceConcurrent, Actors: []string{"alpha", "alpha"}, Prompt: "run"},
		{ID: "empty", Action: SequenceConcurrent, Actors: []string{"alpha", ""}, Prompt: "run"},
	} {
		evaluation := base
		evaluation.Steps = []SequenceStep{step}
		_, err := (Runner{}).RunSequence(context.Background(), evaluation)
		assert.Error(t, err, step.ID)
	}
}

func sequenceHelperCommand(mode string) Command {
	return Command{
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestSequenceHelperProcess", "--", mode},
		Env:        map[string]string{"GO_WANT_SEQUENCE_HELPER": "1"},
		Stdin:      ConformancePromptPlaceholder,
	}
}

func TestSequenceHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SEQUENCE_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	state, _ := os.ReadFile(filepath.Join(os.Getenv("TERMINAL_TODO_CONFORMANCE_WORKSPACE"), "state.txt"))
	switch mode {
	case "fresh":
		fmt.Printf("{\"session_id\":%q,\"state\":%q}\n", "session-"+strings.ReplaceAll(string(state), " ", "-"), string(state))
	case "resume":
		fmt.Printf("{\"session_id\":%q,\"state\":%q}\n", os.Getenv("FAKE_SESSION_ID"), string(state))
	case "wait":
		time.Sleep(time.Second)
	case "concurrent-barrier":
		actor := os.Getenv(ConformanceActorEnvironment)
		prompt, _ := io.ReadAll(os.Stdin)
		if !strings.Contains(string(prompt), actor) || strings.Contains(string(prompt), ConformanceActorPlaceholder) {
			fmt.Fprintln(os.Stderr, "actor prompt was not specialized")
			os.Exit(8)
		}
		markerDir := filepath.Join(os.Getenv("TERMINAL_TODO_CONFORMANCE_WORKSPACE"), "concurrent-markers")
		if err := os.MkdirAll(markerDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
		if err := os.WriteFile(filepath.Join(markerDir, actor), []byte("ready"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(6)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			markers, err := os.ReadDir(markerDir)
			if err == nil && len(markers) >= 2 {
				break
			}
			if time.Now().After(deadline) {
				fmt.Fprintln(os.Stderr, "peer did not reach concurrent barrier")
				os.Exit(7)
			}
			time.Sleep(5 * time.Millisecond)
		}
		if actor == "alpha" {
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Printf("{\"actor\":%q,\"session_id\":%q}\n", actor, "session-"+actor)
	case "concurrent-flood":
		fmt.Println(strings.Repeat("x", 80))
	case "concurrent-failure":
		if os.Getenv(ConformanceActorEnvironment) == "alpha" {
			time.Sleep(50 * time.Millisecond)
			fmt.Fprintln(os.Stderr, "alpha failed")
			os.Exit(9)
		}
		time.Sleep(5 * time.Second)
		fmt.Println("ok")
	}
	os.Exit(0)
}
