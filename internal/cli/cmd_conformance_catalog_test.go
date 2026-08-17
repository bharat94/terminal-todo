package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/internal/projectclock"
	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializeCatalogFixtureBuildsDeterministicProjectAndIntegrationPolicy(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)
	heartbeat := catalogScenarioByID(t, catalog, "heartbeat")
	runtime, err := materializeCatalogFixture(heartbeat, "/opt/terminal todo/bin/todo")
	require.NoError(t, err)
	files := catalogFixtureFiles(runtime.Fixture)
	require.Contains(t, files, ".terminal-todo/tasks.bin")
	require.Contains(t, files, ".terminal-todo/agents.json")
	require.Contains(t, files, ".terminal-todo/conformance-clock")
	require.Contains(t, files, conformance.CodexProjectConfigFile)
	require.Contains(t, files, conformance.ClaudeProjectMCPConfigFile)
	require.Contains(t, files, ".agents/skills/terminal-todo/SKILL.md")
	require.Contains(t, files, ".claude/skills/terminal-todo/SKILL.md")
	assert.Equal(t, map[string]string{"actor:subject": "eval-heartbeat"}, runtime.ActorNames)
	assert.Equal(t, map[string]string{"task:work": "1"}, runtime.TaskIDs)

	storePath := filepath.Join(t.TempDir(), "tasks.bin")
	require.NoError(t, os.WriteFile(storePath, files[".terminal-todo/tasks.bin"].Content, 0o600))
	taskStore, err := store.Load(storePath)
	require.NoError(t, err)
	require.Len(t, taskStore.Tasks, 1)
	task := taskStore.Tasks[1]
	assert.Equal(t, store.StatusInProgress, task.Status)
	assert.Equal(t, "eval-heartbeat", task.Owner)
	assert.Equal(t, uint64(time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC).UnixMilli()), task.LeaseExpires)
	assert.Equal(t, uint64(heartbeat.InitialTime.UnixMilli()), task.Created)

	var registry AgentRegistry
	require.NoError(t, json.Unmarshal(files[".terminal-todo/agents.json"].Content, &registry))
	assert.Equal(t, []string{"go"}, registry.Agents["eval-heartbeat"].Capabilities)
	assertClaudeConformanceEnvironment(t, files[conformance.ClaudeProjectMCPConfigFile])

	discovery := catalogScenarioByID(t, catalog, "discovery")
	discoveryRuntime, err := materializeCatalogFixture(discovery, "/opt/todo")
	require.NoError(t, err)
	discoveryFiles := catalogFixtureFiles(discoveryRuntime.Fixture)
	assert.NotContains(t, discoveryFiles, ".agents/skills/terminal-todo/SKILL.md")
	assert.NotContains(t, discoveryFiles, ".claude/skills/terminal-todo/SKILL.md")
	assertClaudeConformanceEnvironment(t, discoveryFiles[conformance.ClaudeProjectMCPConfigFile])
}

func assertClaudeConformanceEnvironment(t *testing.T, file conformance.FixtureFile) {
	t.Helper()
	var config map[string]any
	require.NoError(t, json.Unmarshal(file.Content, &config))
	servers, ok := config["mcpServers"].(map[string]any)
	require.True(t, ok)
	server, ok := servers["terminal-todo"].(map[string]any)
	require.True(t, ok)
	environment, ok := server["env"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "${"+conformance.ConformanceTraceEnvironment+"}", environment[conformance.ConformanceTraceEnvironment])
	assert.Equal(t, "${"+conformance.ConformanceActorEnvironment+"}", environment[conformance.ConformanceActorEnvironment])
	assert.Equal(t, "${"+projectclock.EnvironmentVariable+"}", environment[projectclock.EnvironmentVariable])
}

func TestCatalogSequenceStepsCompileAllHarnessAndActorActions(t *testing.T) {
	catalog, err := conformance.LoadCatalog()
	require.NoError(t, err)

	heartbeat := catalogScenarioByID(t, catalog, "heartbeat")
	runtime, err := materializeCatalogFixture(heartbeat, "/opt/todo")
	require.NoError(t, err)
	steps, err := catalogSequenceSteps(heartbeat, runtime)
	require.NoError(t, err)
	require.Len(t, steps, 3)
	assert.Equal(t, conformance.SequencePrompt, steps[0].Action)
	assert.Equal(t, "eval-heartbeat", steps[0].Actor)
	assert.Contains(t, steps[0].Prompt, `coordination identity "eval-heartbeat"`)
	assert.Equal(t, conformance.SequenceHarness, steps[1].Action)
	assert.Equal(t, conformance.SequenceResume, steps[2].Action)

	workspace := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, ".terminal-todo"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, ".terminal-todo", "conformance-clock"), []byte(heartbeat.InitialTime.Format(time.RFC3339Nano)+"\n"), 0o600))
	require.NoError(t, steps[1].Harness(context.Background(), workspace))
	advanced, err := conformance.ReadClock(workspace)
	require.NoError(t, err)
	assert.Equal(t, heartbeat.InitialTime.Add(90*time.Second), advanced)

	atomic := catalogScenarioByID(t, catalog, "atomic_acquire")
	atomicRuntime, err := materializeCatalogFixture(atomic, "/opt/todo")
	require.NoError(t, err)
	steps, err = catalogSequenceSteps(atomic, atomicRuntime)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, conformance.SequenceConcurrent, steps[0].Action)
	assert.Equal(t, []string{"eval-alpha", "eval-beta"}, steps[0].Actors)
	assert.Contains(t, steps[0].Prompt, conformance.ConformanceActorPlaceholder)
}

func catalogScenarioByID(t *testing.T, catalog conformance.Catalog, id string) conformance.CatalogScenario {
	t.Helper()
	for _, scenario := range catalog.Scenarios {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("missing scenario %q", id)
	return conformance.CatalogScenario{}
}

func catalogFixtureFiles(fixture conformance.Fixture) map[string]conformance.FixtureFile {
	files := make(map[string]conformance.FixtureFile, len(fixture.Files))
	for _, file := range fixture.Files {
		files[file.Path] = file
	}
	return files
}
