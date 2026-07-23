package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConformanceOptionsRequiresExplicitRun(t *testing.T) {
	options, err := parseConformanceOptions(nil)
	require.NoError(t, err)
	assert.False(t, options.Run)
	assert.Equal(t, []string{"codex", "claude"}, options.Hosts)
	assert.Equal(t, defaultEvalTimeout, options.Timeout)

	options, err = parseConformanceOptions([]string{
		"--host", "codex",
		"--run",
		"--json",
		"--include-events",
		"--keep-workspace",
		"--timeout", "90s",
		"--model", "gpt-test",
	})
	require.NoError(t, err)
	assert.True(t, options.Run)
	assert.True(t, options.JSON)
	assert.True(t, options.IncludeEvents)
	assert.True(t, options.KeepWorkspace)
	assert.Equal(t, []string{"codex"}, options.Hosts)
	assert.Equal(t, "gpt-test", options.Model)

	_, err = parseConformanceOptions([]string{"--host", "unknown"})
	assert.ErrorContains(t, err, "codex, claude, or all")
	_, err = parseConformanceOptions([]string{"--timeout", "31m"})
	assert.ErrorContains(t, err, "between")
}

func TestCompactConformanceTranscriptsPreservesCountsAndResults(t *testing.T) {
	report := conformanceCommandReport{
		Reports: []conformance.Report{{
			Preflight: &conformance.ExecutionResult{Capture: conformance.Capture{
				Stdout:    []conformance.Event{jsonConformanceEvent(`{"ok":true}`)},
				BytesRead: 11,
			}},
			Execution: &conformance.ExecutionResult{Capture: conformance.Capture{
				Stderr:    []conformance.Event{{Kind: conformance.EventText, Text: "detail"}},
				BytesRead: 6,
			}},
			Evidence: conformance.Evidence{
				HostEvents: []conformance.Event{jsonConformanceEvent(`{"ok":true}`)},
			},
			Checks: []conformance.CheckResult{{ID: "completed", Passed: true}},
		}},
	}
	compacted := compactConformanceTranscripts(report)
	assert.Empty(t, compacted.Reports[0].Preflight.Capture.Stdout)
	assert.Equal(t, int64(11), compacted.Reports[0].Preflight.Capture.BytesRead)
	assert.Empty(t, compacted.Reports[0].Execution.Capture.Stderr)
	assert.Empty(t, compacted.Reports[0].Evidence.HostEvents)
	assert.Equal(t, "completed", compacted.Reports[0].Checks[0].ID)
	assert.Contains(t, compacted.Notice, "--include-events")
}

func TestLifecycleFixtureContainsIsolatedStoreAndHostConfigs(t *testing.T) {
	fixture, err := lifecycleFixture("/opt/terminal todo/bin/todo")
	require.NoError(t, err)
	files := make(map[string]conformance.FixtureFile)
	for _, file := range fixture.Files {
		files[file.Path] = file
	}
	require.Contains(t, files, ".terminal-todo/tasks.bin")
	require.Contains(t, files, conformance.CodexProjectConfigFile)
	require.Contains(t, files, conformance.ClaudeProjectMCPConfigFile)
	assert.Contains(t, string(files[conformance.CodexProjectConfigFile].Content), `command = "/opt/terminal todo/bin/todo"`)

	var claudeConfig map[string]any
	require.NoError(t, json.Unmarshal(files[conformance.ClaudeProjectMCPConfigFile].Content, &claudeConfig))
	assert.Contains(t, claudeConfig, "mcpServers")

	storePath := filepath.Join(t.TempDir(), "tasks.bin")
	require.NoError(t, os.WriteFile(storePath, files[".terminal-todo/tasks.bin"].Content, 0o600))
	taskStore, err := store.Load(storePath)
	require.NoError(t, err)
	require.Len(t, taskStore.Tasks, 1)
	assert.Equal(t, "Persist the conformance lifecycle marker", taskStore.Tasks[1].Title)
}

func TestLifecycleNormalizerGradesPersistedStateAndQuietOutcome(t *testing.T) {
	workspace := t.TempDir()
	storePath := filepath.Join(workspace, ".terminal-todo", "tasks.bin")
	taskStore := store.NewTaskStore()
	task := taskStore.AddTask("fixture", nil)
	task.Owner = ""
	task.LeaseExpires = 0
	task.Status = store.StatusCompleted
	task.Extra["conformance_marker"] = "marker"
	taskStore.AddEvent(store.EventTaskClaimed, task.ID, "eval-codex-marker", nil)
	taskStore.AddEvent(store.EventTaskCompleted, task.ID, "eval-codex-marker", nil)
	require.NoError(t, taskStore.Save(storePath))

	codexEvent := json.RawMessage(`{"type":"item.completed","item":{"type":"agent_message","text":"Lifecycle evaluation completed successfully."}}`)
	capture := conformance.Capture{Stdout: []conformance.Event{{
		Sequence: 1,
		Stream:   conformance.StreamStdout,
		Kind:     conformance.EventJSON,
		JSON:     codexEvent,
	}}}
	evidence, err := lifecycleNormalizer("codex").Normalize(context.Background(), workspace, capture)
	require.NoError(t, err)
	assert.Equal(t, []string{"Lifecycle evaluation completed successfully."}, evidence.AssistantMessages)

	observation := conformance.Observation{Capture: capture, Evidence: evidence}
	for _, check := range lifecycleAssertions("eval-codex-marker", "marker") {
		passed, detail := check.Evaluate(observation)
		assert.True(t, passed, "%s: %s", check.ID, detail)
	}
}

func TestExtractHostAssistantMessagesUsesOnlyFinalOutcome(t *testing.T) {
	codexCapture := conformance.Capture{Stdout: []conformance.Event{
		jsonConformanceEvent(`{"type":"item.completed","item":{"type":"agent_message","text":"first"}}`),
		jsonConformanceEvent(`{"type":"item.completed","item":{"type":"mcp_tool_call","text":"ignored"}}`),
		jsonConformanceEvent(`{"type":"item.completed","item":{"type":"agent_message","text":"final"}}`),
	}}
	assert.Equal(t, []string{"final"}, extractHostAssistantMessages("codex", codexCapture))

	claudeCapture := conformance.Capture{Stdout: []conformance.Event{
		jsonConformanceEvent(`{"type":"assistant","message":{"content":[{"type":"text","text":"intermediate"}]}}`),
		jsonConformanceEvent(`{"type":"result","result":"final result"}`),
	}}
	assert.Equal(t, []string{"final result"}, extractHostAssistantMessages("claude", claudeCapture))
}

func jsonConformanceEvent(value string) conformance.Event {
	return conformance.Event{
		Stream: conformance.StreamStdout,
		Kind:   conformance.EventJSON,
		JSON:   json.RawMessage(value),
	}
}
