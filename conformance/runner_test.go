package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerCapturesRedactsNormalizesAndScores(t *testing.T) {
	secret := "s\"ecret-token"
	evaluation := baseEvaluation("emit")
	evaluation.Fixture.Files = []FixtureFile{{
		Path:    "nested/fixture.txt",
		Content: []byte("fixture-ready"),
	}}
	evaluation.Redactions = []string{secret}
	evaluation.Host.Run.Env["FAKE_SECRET"] = secret
	evaluation.Normalizer = NormalizerFunc(func(_ context.Context, workspace string, capture Capture) (Evidence, error) {
		assert.DirExists(t, workspace)
		var operation Operation
		require.NoError(t, json.Unmarshal(capture.Stdout[0].JSON, &operation))
		evidence := EmptyEvidence(capture)
		evidence.Operations = append(evidence.Operations, operation)
		evidence.AssistantMessages = []string{capture.Stdout[1].Text}
		return evidence, nil
	})
	evaluation.Assertions = []Assertion{
		EvidenceCheck("acquire", "uses atomic acquisition", 70, true, func(evidence Evidence) (bool, string) {
			return len(evidence.Operations) == 1 && evidence.Operations[0].Operation == "acquire", ""
		}),
		Contains("fixture", StreamStdout, "fixture-ready", 20, true),
		Excludes("secret", StreamAny, secret, 10, true),
	}

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusPassed, report.Status)
	assert.Equal(t, 100.0, report.Score.Percent)
	require.NotNil(t, report.Execution)
	assert.Len(t, report.Execution.Capture.Stdout, 3)
	assert.Len(t, report.Execution.Capture.Stderr, 1)
	assert.Equal(t, EventJSON, report.Execution.Capture.Stdout[0].Kind)
	assert.Contains(t, string(report.Execution.Capture.Stdout[0].JSON), redactedValue)
	assert.NotContains(t, string(report.Execution.Capture.Stdout[0].JSON), secret)
	assert.Equal(t, "diagnostic", report.Execution.Capture.Stderr[0].Text)
	assert.Empty(t, report.Workspace)

	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), secret)
}

func TestRunnerRedactsWorkspacePromptAndHostSessionIdentifiers(t *testing.T) {
	evaluation := baseEvaluation("sensitive-event")
	evaluation.Host.Run.Prompt = "controlled prompt"

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	require.NotNil(t, report.Execution)
	transcript := Observation{Capture: report.Execution.Capture}.Transcript(StreamStdout)
	assert.NotContains(t, transcript, "controlled prompt")
	assert.NotContains(t, transcript, "session-secret")
	assert.NotContains(t, transcript, "terminal-todo-conformance-")
	assert.Contains(t, transcript, redactedValue)
}

func TestRunnerKeepsWorkspaceOnlyWhenRequested(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.KeepWorkspace = true

	report, err := (Runner{TempRoot: t.TempDir()}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	require.NotEmpty(t, report.Workspace)
	assert.DirExists(t, report.Workspace)
	t.Cleanup(func() { _ = os.RemoveAll(report.Workspace) })
}

func TestRunnerTimeoutIsStructuredFailure(t *testing.T) {
	evaluation := baseEvaluation("wait")
	evaluation.Limits.Timeout = 30 * time.Millisecond

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureTimeout, report.InfrastructureFailures[0].Kind)
	assert.True(t, report.Execution.Process.TimedOut)
	assert.False(t, report.Score.Scored)
}

func TestRunnerCancellationIsDistinctFromTimeout(t *testing.T) {
	evaluation := baseEvaluation("wait")
	evaluation.Limits.Timeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	report, err := (Runner{}).Run(ctx, evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureCancelled, report.InfrastructureFailures[0].Kind)
	assert.True(t, report.Execution.Process.Cancelled)
	assert.False(t, report.Execution.Process.TimedOut)
}

func TestRunnerOutputLimitsAreStructuredFailures(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		maxOutput      int64
		maxEvent       int
		detailContains string
	}{
		{name: "total output", mode: "flood", maxOutput: 128, maxEvent: 1024, detailContains: "output exceeded"},
		{name: "single event", mode: "long-line", maxOutput: 4096, maxEvent: 64, detailContains: "event exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := baseEvaluation(test.mode)
			evaluation.Limits.MaxOutputBytes = test.maxOutput
			evaluation.Limits.MaxEventBytes = test.maxEvent

			report, err := (Runner{}).Run(context.Background(), evaluation)
			require.NoError(t, err)
			assert.Equal(t, StatusFailed, report.Status)
			require.Len(t, report.InfrastructureFailures, 1)
			assert.Equal(t, FailureOutputLimit, report.InfrastructureFailures[0].Kind)
			assert.Contains(t, report.InfrastructureFailures[0].Detail, test.detailContains)
		})
	}
}

func TestRunnerClassifiesHostFailures(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		rule       FailureRule
		wantKind   FailureKind
		wantStatus Status
	}{
		{
			name: "authentication exit",
			mode: "auth",
			rule: FailureRule{
				Kind: FailureAuthentication, Disposition: DispositionSkip,
				Stream: StreamStderr, Contains: "not logged in",
			},
			wantKind: FailureAuthentication, wantStatus: StatusSkipped,
		},
		{
			name: "approval zero exit",
			mode: "approval",
			rule: FailureRule{
				Kind: FailureApproval, Disposition: DispositionSkip,
				Stream: StreamStdout, Contains: "pending approval",
			},
			wantKind: FailureApproval, wantStatus: StatusSkipped,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluation := baseEvaluation(test.mode)
			evaluation.Host.FailureRules = []FailureRule{test.rule}
			report, err := (Runner{}).Run(context.Background(), evaluation)
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, report.Status)
			require.Len(t, report.InfrastructureFailures, 1)
			assert.Equal(t, test.wantKind, report.InfrastructureFailures[0].Kind)
			assert.False(t, report.Score.Scored)
		})
	}
}

func TestRunnerPreflightFailureSkipsHost(t *testing.T) {
	evaluation := baseEvaluation("must-not-run")
	preflight := helperCommand("preflight-fail")
	evaluation.Host.Preflight = &preflight

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusSkipped, report.Status)
	assert.NotNil(t, report.Preflight)
	assert.Nil(t, report.Execution)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailurePreflight, report.InfrastructureFailures[0].Kind)
}

func TestRunnerMissingExecutableIsStructured(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.Host.Run.Executable = filepath.Join(t.TempDir(), "missing-host")

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureStart, report.InfrastructureFailures[0].Kind)
}

func TestRunnerNormalizationFailureIsStructuredAndRedacted(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.Redactions = []string{"private"}
	evaluation.Normalizer = NormalizerFunc(func(context.Context, string, Capture) (Evidence, error) {
		return Evidence{}, fmt.Errorf("could not parse private response")
	})

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusError, report.Status)
	require.Len(t, report.InfrastructureFailures, 1)
	assert.Equal(t, FailureNormalization, report.InfrastructureFailures[0].Kind)
	assert.NotContains(t, report.InfrastructureFailures[0].Detail, "private")
}

func TestRunnerRejectsUnsafeFixturePath(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.Fixture.Files = []FixtureFile{{Path: "../escape", Content: []byte("no")}}

	_, err := (Runner{}).Run(context.Background(), evaluation)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stay within")
}

func TestRunnerRequiredChecksAndMinimumScore(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.MinimumScore = 75
	evaluation.Assertions = []Assertion{
		Contains("present", StreamStdout, "ok", 80, true),
		Contains("optional", StreamStdout, "missing", 20, false),
	}

	report, err := (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusPassed, report.Status)
	assert.Equal(t, 80.0, report.Score.Percent)

	evaluation.Assertions[0] = Contains("present", StreamStdout, "absent", 80, true)
	report, err = (Runner{}).Run(context.Background(), evaluation)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, report.Status)
}

func TestValidateEvaluationRejectsInvalidAssertions(t *testing.T) {
	evaluation := baseEvaluation("success")
	evaluation.Assertions = []Assertion{
		{ID: "same", Weight: 1, Evaluate: func(Observation) (bool, string) { return true, "" }},
		{ID: "same", Weight: 1, Evaluate: func(Observation) (bool, string) { return true, "" }},
	}
	_, err := (Runner{}).Run(context.Background(), evaluation)
	assert.ErrorContains(t, err, "duplicate")
}

func baseEvaluation(mode string) Evaluation {
	return Evaluation{
		ID: "test_scenario",
		Host: Host{
			Name: "fake-host",
			Run:  helperCommand(mode),
		},
		Assertions: []Assertion{
			Contains("success", StreamStdout, "ok", 1, true),
		},
	}
}

func helperCommand(mode string) Command {
	return Command{
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestConformanceHelperProcess", "--", mode},
		Env: map[string]string{
			"GO_WANT_CONFORMANCE_HELPER": "1",
		},
	}
}

func TestConformanceHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CONFORMANCE_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		fmt.Println("ok")
	case "emit":
		workspace := os.Getenv("TERMINAL_TODO_CONFORMANCE_WORKSPACE")
		fixture, err := os.ReadFile(filepath.Join(workspace, "nested", "fixture.txt"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		secret := os.Getenv("FAKE_SECRET")
		encoded, _ := json.Marshal(map[string]any{
			"operation": "acquire",
			"arguments": map[string]any{"token": secret},
		})
		fmt.Println(string(encoded))
		fmt.Printf("assistant used %s\n", secret)
		fmt.Println(string(fixture))
		fmt.Fprintln(os.Stderr, "diagnostic")
	case "wait":
		time.Sleep(time.Second)
	case "flood":
		for i := 0; i < 50; i++ {
			fmt.Println(strings.Repeat("x", 32))
		}
	case "long-line":
		fmt.Println(strings.Repeat("x", 1024))
	case "auth":
		fmt.Fprintln(os.Stderr, "Not logged in; authenticate first")
		os.Exit(17)
	case "approval":
		fmt.Println("Pending approval")
	case "sensitive-event":
		fmt.Printf(
			"{\"session_id\":\"session-secret\",\"prompt\":%q,\"workspace\":%q,\"ok\":true}\n",
			"controlled prompt",
			os.Getenv("TERMINAL_TODO_CONFORMANCE_WORKSPACE"),
		)
	case "preflight-fail":
		fmt.Fprintln(os.Stderr, "host is unavailable")
		os.Exit(2)
	case "must-not-run":
		panic("host command ran after failed preflight")
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode")
		os.Exit(4)
	}
	os.Exit(0)
}
