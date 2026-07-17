package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBootstrapSummarizesObjectiveAndWorkerContext(t *testing.T) {
	s := store.NewTaskStore()
	objective := s.AddTask("Ship coordinated release", []string{"todo://local/2", "todo://local/3"})
	objective.Status = store.StatusBlocked
	objective.Priority = 1
	objective.Capabilities = []string{"product"}
	objective.BlockReason = "waiting for approval"

	completed := s.AddTask("Verify artifacts", nil)
	completed.Status = store.StatusCompleted
	completed.Completed = uint64(time.Now().UnixMilli())

	ready := s.AddTask("Write migration", nil)
	ready.Priority = 0.9
	ready.Capabilities = []string{"go"}

	owned := s.AddTask("Audit protocol", nil)
	owned.Status = store.StatusInProgress
	owned.Owner = "worker"
	owned.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())

	incompatible := s.AddTask("Review docs", nil)
	incompatible.Capabilities = []string{"docs"}

	blocked := s.AddTask("Publish", nil)
	blocked.Status = store.StatusBlocked
	blocked.BlockReason = "credentials unavailable"

	for i := 0; i < 7; i++ {
		s.AddEvent(store.EventTaskUpdated, ready.ID, "worker", map[string]string{
			"reason":     strings.Repeat("界", 200),
			"request_id": "private-idempotency-key",
		})
	}

	registry := &AgentRegistry{
		SchemaVersion: "1",
		Agents: map[string]AgentCard{
			"worker":      {Name: "worker", Capabilities: []string{"go"}, MaxLoad: 2},
			"docs-worker": {Name: "docs-worker", Capabilities: []string{"docs"}},
		},
	}
	result, err := buildBootstrap(
		s,
		registry,
		bootstrapParams{Actor: "worker", Limit: 2, EventLimit: 3},
		false,
		nil,
	)
	require.NoError(t, err)

	require.NotNil(t, result.Objective)
	assert.Equal(t, objective.ID, result.Objective.ID)
	assert.Equal(t, "inferred_top_level", result.ObjectiveSource)
	assert.Equal(t, "objective:1", result.Progress.Scope)
	assert.Equal(t, 3, result.Progress.Total)
	assert.Equal(t, 1, result.Progress.Completed)
	assert.Equal(t, 1, result.Progress.Pending)
	assert.Equal(t, 1, result.Progress.Blocked)
	assert.Equal(t, 33, result.Progress.Percent)

	assert.Equal(t, []string{"go"}, result.Worker.Capabilities)
	assert.Equal(t, "agent_card", result.Worker.CapabilitySource)
	assert.Equal(t, 1, result.Worker.CurrentLoad)
	assert.Equal(t, 2, result.Worker.MaxLoad)
	assert.False(t, result.Worker.AtCapacity)

	assert.Equal(t, 1, result.OwnedWork.Total)
	assert.Equal(t, owned.ID, result.OwnedWork.Items[0].ID)
	assert.Equal(t, 1, result.ReadyWork.Total)
	assert.Equal(t, ready.ID, result.ReadyWork.Items[0].ID)
	assert.NotEqual(t, incompatible.ID, result.ReadyWork.Items[0].ID)

	assert.Equal(t, 2, result.Blockers.ExplicitTotal)
	assert.Equal(t, 1, result.Blockers.DependencyBlockedTotal)
	assert.Equal(t, []string{"todo://local/3"}, result.Blockers.PrimaryDependencies)

	assert.Equal(t, 4, result.CapabilityDemand.UnclaimedTasks)
	assert.Equal(t, 1, result.CapabilityDemand.WithoutCaps)
	assert.Equal(t, 3, result.CapabilityDemand.Total)
	assert.Len(t, result.CapabilityDemand.Items, 2)
	assert.Len(t, result.RecentEvents.Items, 3)
	assert.Equal(t, uint64(7), result.RecentEvents.Items[0].ID)
	assert.LessOrEqual(t, len(result.RecentEvents.Items[0].Detail), maxBootstrapDetailBytes)
	assert.True(t, strings.HasSuffix(result.RecentEvents.Items[0].Detail, "..."))
	assert.NotContains(t, result.RecentEvents.Items[0].Detail, "private-idempotency-key")

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Less(t, len(encoded), 20_000)
}

func TestBuildBootstrapEnforcesBoundsAndExplicitObjective(t *testing.T) {
	s := store.NewTaskStore()
	objective := s.AddTask(strings.Repeat("界", 200), nil)
	for i := 0; i < 30; i++ {
		task := s.AddTask(strings.Repeat("ready ", 100), nil)
		task.Priority = float32(i) / 30
		task.Capabilities = []string{}
		s.AddEvent(store.EventTaskUpdated, task.ID, strings.Repeat("actor", 100), map[string]string{"title": strings.Repeat("x", 1000)})
	}
	registry := &AgentRegistry{Agents: map[string]AgentCard{}}

	result, err := buildBootstrap(
		s,
		registry,
		bootstrapParams{Actor: "new-worker", ObjectiveID: objective.ID},
		false,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "explicit", result.ObjectiveSource)
	assert.LessOrEqual(t, len(result.Objective.Title), maxBootstrapTitleBytes)
	assert.Len(t, result.ReadyWork.Items, defaultBootstrapLimit)
	assert.Len(t, result.RecentEvents.Items, defaultBootstrapEventLimit)

	_, err = buildBootstrap(
		s,
		registry,
		bootstrapParams{Actor: "new-worker", Limit: maxBootstrapLimit + 1},
		false,
		nil,
	)
	assert.ErrorContains(t, err, "limit must be between")

	_, err = buildBootstrap(
		s,
		registry,
		bootstrapParams{Actor: "new-worker", ObjectiveID: 999},
		false,
		nil,
	)
	assert.ErrorContains(t, err, "objective task 999 not found")
}

func TestSelectBootstrapObjectiveLatestCompletedExcludesPendingOrphans(t *testing.T) {
	s := store.NewTaskStore()
	completed := s.AddTask("Last completed work", nil)
	completed.Status = store.StatusCompleted
	completed.Completed = 100
	orphan := s.AddTask("Orphaned child", nil)
	orphan.Lineage = "todo://local/999"

	objective, source, err := selectBootstrapObjective(s, 0)

	require.NoError(t, err)
	require.NotNil(t, objective)
	assert.Equal(t, completed.ID, objective.ID)
	assert.Equal(t, "latest_completed", source)
}

func TestSelectBootstrapObjectiveDoesNotLabelPendingOrphanAsCompleted(t *testing.T) {
	s := store.NewTaskStore()
	orphan := s.AddTask("Orphaned child", nil)
	orphan.Lineage = "todo://local/999"

	objective, source, err := selectBootstrapObjective(s, 0)

	require.NoError(t, err)
	assert.Nil(t, objective)
	assert.Equal(t, "none", source)
}

func TestServerAndMCPExposeBootstrap(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("Ready through protocol", nil)
	require.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	result, rpcErr := srv.dispatch("todo.bootstrap", json.RawMessage(`{
		"actor":"protocol-worker",
		"capabilities":[],
		"limit":1,
		"eventLimit":1
	}`))
	require.Nil(t, rpcErr)
	brief := result.(bootstrapResult)
	assert.Equal(t, "protocol-worker", brief.Worker.Actor)
	assert.Equal(t, 1, brief.ReadyWork.Total)
	assert.Len(t, brief.ReadyWork.Items, 1)

	_, rpcErr = srv.dispatch("todo.bootstrap", json.RawMessage(`{"actor":"protocol-worker","limit":21}`))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)

	mcp := &mcpServer{backend: srv, initializeSeen: true, initialized: true}
	callResult, rpcErr := mcp.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_bootstrap",
		"arguments":{"actor":"protocol-worker","capabilities":[]}
	}`))
	require.Nil(t, rpcErr)
	call := callResult.(mcpCallResult)
	assert.False(t, call.IsError)
	assert.Equal(t, "Objective 1 [pending]; 0/1 complete; 0 owned, 1 compatible ready, 0 blocked.", call.Content[0].Text)
	assert.IsType(t, bootstrapResult{}, call.StructuredContent)
}

func TestCLI_BootstrapReturnsBoundedJSONBrief(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	path := filepath.Join(tmpDir, ".terminal-todo", "tasks.bin")

	s, err := store.LoadCurrent(path)
	require.NoError(t, err)
	objective := s.AddTask("CLI objective", nil)
	objective.Priority = 1
	s.AddEvent(store.EventTaskCreated, objective.ID, "", map[string]string{"title": objective.Title})
	require.NoError(t, s.Save(path))

	cmd := exec.Command(todo, "bootstrap", "--as", "cli-worker", "--objective", "1", "--limit", "1", "--events", "1", "--json")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	var brief bootstrapResult
	require.NoError(t, json.Unmarshal(output, &brief))
	assert.Equal(t, "cli-worker", brief.Worker.Actor)
	require.NotNil(t, brief.Objective)
	assert.Equal(t, uint64(1), brief.Objective.ID)
	assert.Len(t, brief.ReadyWork.Items, 1)
	assert.Len(t, brief.RecentEvents.Items, 1)
}
