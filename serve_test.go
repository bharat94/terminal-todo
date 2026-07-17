package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
)

func TestServerAcquireUsesSharedAtomicAllocator(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("RPC work", nil)
	assert.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	params := json.RawMessage(`{"actor":"rpc-agent","requestId":"rpc-request-1"}`)
	result, rpcErr := srv.dispatch("todo.acquire", params)
	assert.Nil(t, rpcErr)
	envelope, ok := result.(acquireEnvelope)
	assert.True(t, ok)
	assert.False(t, envelope.Replayed)
	assert.Equal(t, "rpc-request-1", envelope.RequestID)
	assert.Equal(t, "RPC work", envelope.Task.Title)
	assert.Equal(t, "rpc-agent", envelope.Task.Metadata.Owner)

	replayed, rpcErr := srv.dispatch("todo.acquire", params)
	assert.Nil(t, rpcErr)
	replayEnvelope, ok := replayed.(acquireEnvelope)
	assert.True(t, ok)
	assert.True(t, replayEnvelope.Replayed)
	assert.Equal(t, envelope.Task, replayEnvelope.Task)

	_, rpcErr = srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"rpc-agent","requestId":"rpc-request-1","ttl":"2h"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcIdempotencyConflict, rpcErr.Code)

	_, rpcErr = srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"rpc-agent"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)

	_, rpcErr = srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"rpc-agent","requestId":"rpc-request-2"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcNoWork, rpcErr.Code)
}

func TestServerRejectsInvalidTaskAndConfigInputAsInvalidParams(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	assert.NoError(t, store.NewTaskStore().Save(path))

	srv := &server{initialized: true}
	for _, test := range []struct {
		method string
		params string
	}{
		{"todo.add", `{"title":"work","priority":1.1}`},
		{"todo.add", `{"title":"   "}`},
		{"todo.update", `{"id":1,"priority":-0.1}`},
		{"todo.config.set", `{"key":"default_priority","value":"NaN"}`},
		{"todo.config.set", `{"key":"default_ttl","value":"forever"}`},
	} {
		_, rpcErr := srv.dispatch(test.method, json.RawMessage(test.params))
		assert.NotNil(t, rpcErr, "%s %s", test.method, test.params)
		assert.Equal(t, rpcInvalidParams, rpcErr.Code, "%s %s", test.method, test.params)
	}
}

func TestServerHeartbeatRenewsOnlyTheOwnersActiveLease(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	task := s.AddTask("RPC long-running work", nil)
	task.Status = store.StatusInProgress
	task.Owner = "rpc-agent"
	task.LeaseExpires = uint64(time.Now().Add(15 * time.Minute).UnixMilli())
	previousExpiry := task.LeaseExpires
	assert.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	result, rpcErr := srv.dispatch("todo.heartbeat", json.RawMessage(`{"id":1,"actor":"rpc-agent","ttl":"2h"}`))
	assert.Nil(t, rpcErr)
	envelope, ok := result.(taskEnvelope)
	assert.True(t, ok)
	assert.Equal(t, "rpc-agent", envelope.Task.Metadata.Owner)

	persisted, err := store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Greater(t, persisted.Tasks[1].LeaseExpires, previousExpiry)
	assert.Equal(t, store.EventLeaseRenewed, persisted.Events[len(persisted.Events)-1].Type)

	_, rpcErr = srv.dispatch("todo.heartbeat", json.RawMessage(`{"id":1,"actor":"other-agent"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcNotOwner, rpcErr.Code)

	_, rpcErr = srv.dispatch("todo.heartbeat", json.RawMessage(`{"id":999,"actor":"rpc-agent"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcTaskNotFound, rpcErr.Code)

	persisted.Tasks[1].Status = store.StatusPending
	persisted.Tasks[1].Owner = ""
	persisted.Tasks[1].LeaseExpires = 0
	assert.NoError(t, persisted.Save(path))
	_, rpcErr = srv.dispatch("todo.heartbeat", json.RawMessage(`{"id":1,"actor":"rpc-agent"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcLeaseNotActive, rpcErr.Code)
}

func TestServerBlockReleasesLeaseAndUnblockRepairsLegacyOwnership(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	task := s.AddTask("RPC blocked work", nil)
	task.Status = store.StatusInProgress
	task.Owner = "rpc-agent"
	task.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())
	assert.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	_, rpcErr := srv.dispatch("todo.block", json.RawMessage(`{"id":1,"actor":"rpc-agent","reason":"waiting"}`))
	assert.Nil(t, rpcErr)

	persisted, err := store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Equal(t, store.StatusBlocked, persisted.Tasks[1].Status)
	assert.Empty(t, persisted.Tasks[1].Owner)
	assert.Zero(t, persisted.Tasks[1].LeaseExpires)

	persisted.Tasks[1].Owner = "legacy-agent"
	persisted.Tasks[1].LeaseExpires = uint64(time.Now().Add(-time.Hour).UnixMilli())
	assert.NoError(t, persisted.Save(path))

	_, rpcErr = srv.dispatch("todo.unblock", json.RawMessage(`{"id":1,"actor":"coordinator"}`))
	assert.Nil(t, rpcErr)
	persisted, err = store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Equal(t, store.StatusPending, persisted.Tasks[1].Status)
	assert.Empty(t, persisted.Tasks[1].Owner)
	assert.Zero(t, persisted.Tasks[1].LeaseExpires)
}

func TestServerDecomposeReleasesParentLeaseAndAttributesActor(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	task := s.AddTask("Owned RPC objective", nil)
	task.Status = store.StatusInProgress
	task.Owner = "planner"
	task.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())
	task.BlockReason = "stale manual blocker"
	assert.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	_, rpcErr := srv.dispatch("todo.decompose", json.RawMessage(`{"id":1,"actor":"other-agent","subtasks":[{"title":"Unauthorized"}]}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcNotOwner, rpcErr.Code)

	result, rpcErr := srv.dispatch("todo.decompose", json.RawMessage(`{"id":1,"actor":"planner","subtasks":[{"title":"Build"}]}`))
	assert.Nil(t, rpcErr)
	decomposed, ok := result.(decomposeResult)
	assert.True(t, ok)
	assert.Equal(t, "pending", decomposed.Parent.Status)
	assert.Empty(t, decomposed.Parent.Metadata.Owner)
	assert.Nil(t, decomposed.Parent.Metadata.LeaseExpires)
	assert.Empty(t, decomposed.Parent.Metadata.BlockReason)

	persisted, err := store.LoadCurrent(path)
	assert.NoError(t, err)
	event := persisted.Events[len(persisted.Events)-1]
	assert.Equal(t, store.EventTaskDecomposed, event.Type)
	assert.Equal(t, "planner", event.Actor)
}

func TestServerNotificationDoesNotEmitResponse(t *testing.T) {
	var output bytes.Buffer
	srv := &server{initialized: true, encoder: json.NewEncoder(&output)}
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"method\":\"todo.ping\",\"params\":{}}\n")
	assert.NoError(t, srv.readRequests(input))
	assert.Empty(t, output.String())
}

func TestServerPingAdvertisesProtocolAndCapabilities(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	srv := &server{initialized: true}
	result, rpcErr := srv.dispatch("todo.ping", json.RawMessage(`{}`))

	assert.Nil(t, rpcErr)
	ping, ok := result.(pingResult)
	assert.True(t, ok)
	assert.Equal(t, protocolVersion, ping.ProtocolVersion)
	assert.Equal(t, projectRoot, ping.Project)
	assert.True(t, ping.Initialized)
	assert.Contains(t, ping.Capabilities, "lease_heartbeat")
	assert.Contains(t, ping.Capabilities, "atomic_acquire")
	assert.Contains(t, ping.Capabilities, "idempotent_acquire")
	assert.Contains(t, ping.Capabilities, "cross_repository_dependencies")
}

func TestServerRejectsUnknownTopLevelRequestFields(t *testing.T) {
	var output bytes.Buffer
	srv := &server{initialized: true, encoder: json.NewEncoder(&output)}
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"todo.ping\",\"extra\":true}\n")
	assert.NoError(t, srv.readRequests(input))
	assert.Contains(t, output.String(), `"code":-32600`)
}

func TestServerAcceptsRequestsLargerThanDefaultScannerLimit(t *testing.T) {
	var output bytes.Buffer
	srv := &server{initialized: true, encoder: json.NewEncoder(&output)}
	padding := strings.Repeat("x", 70*1024)
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"todo.ping","params":{"unknown":"` + padding + `"}}` + "\n")
	assert.NoError(t, srv.readRequests(input))
	assert.Contains(t, output.String(), `"code":-32602`)
}

func TestServerAcquireReportsAgentCapacity(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	active := s.AddTask("Active work", nil)
	active.Owner = "busy-agent"
	active.Status = store.StatusInProgress
	active.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())
	s.AddTask("Ready work", nil)
	assert.NoError(t, s.Save(path))
	assert.NoError(t, saveAgentRegistry(&AgentRegistry{
		SchemaVersion: "1",
		Agents: map[string]AgentCard{
			"busy-agent": {Name: "busy-agent", MaxLoad: 1},
		},
	}))

	srv := &server{initialized: true}
	_, rpcErr := srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"busy-agent","requestId":"busy-request"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcAgentCapacity, rpcErr.Code)
	assert.Equal(t, -32011, rpcErr.Code)
}

func TestServerAgentCardPersistsProfileAndReportsCurrentLoad(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	active := s.AddTask("Active", nil)
	active.Status = store.StatusInProgress
	active.Owner = "rpc-agent"
	active.LeaseExpires = uint64(time.Now().Add(time.Hour).UnixMilli())
	ready := s.AddTask("Go work", nil)
	ready.Capabilities = []string{"go"}
	assert.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	result, rpcErr := srv.dispatch("todo.agentCard", json.RawMessage(`{
		"actor":"rpc-agent",
		"caps":["go","testing","go"],
		"desc":"RPC worker",
		"maxLoad":2
	}`))
	assert.Nil(t, rpcErr)
	card := result.(agentCardResult)
	assert.Equal(t, []string{"go", "testing"}, card.Caps)
	assert.Equal(t, "RPC worker", card.Desc)
	assert.Equal(t, 2, card.MaxLoad)
	assert.Equal(t, 1, card.CurrentLoad)
	assert.NotEmpty(t, card.CreatedAt)

	registry, err := loadAgentRegistry()
	assert.NoError(t, err)
	assert.Equal(t, []string{"go", "testing"}, registry.Agents["rpc-agent"].Capabilities)
	assert.Equal(t, 2, registry.Agents["rpc-agent"].MaxLoad)

	queried, rpcErr := srv.dispatch("todo.agentCard", json.RawMessage(`{"actor":"rpc-agent"}`))
	assert.Nil(t, rpcErr)
	assert.Equal(t, card.Actor, queried.(agentCardResult).Actor)
	assert.Equal(t, 1, queried.(agentCardResult).CurrentLoad)

	acquired, rpcErr := srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"rpc-agent","requestId":"registered-profile"}`))
	assert.Nil(t, rpcErr)
	assert.Equal(t, "Go work", acquired.(acquireEnvelope).Task.Title)

	_, rpcErr = srv.dispatch("todo.agentCard", json.RawMessage(`{"caps":["go"]}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	_, rpcErr = srv.dispatch("todo.agentCard", json.RawMessage(`{"actor":"bad","maxLoad":-1}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
}

func TestServerRejectsUnknownAndTrailingParams(t *testing.T) {
	srv := &server{initialized: true}

	_, rpcErr := srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"agent","requestId":"request-1","capabilites":["go"]}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "unknown field")

	_, rpcErr = srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"agent","requestId":"request-1"} {}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "trailing JSON data")

	_, rpcErr = srv.dispatch("todo.acquire", json.RawMessage(`{"actor":"agent","requestId":"request-1","wait":"1s"}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "unknown field")
}

func TestServerCompactSupportsDryRunAndStrictRetention(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	for i := 0; i < 4; i++ {
		s.AddEvent(store.EventTaskUpdated, 1, "test", nil)
	}
	assert.NoError(t, s.Save(path))
	srv := &server{initialized: true}

	result, rpcErr := srv.dispatch("todo.compact", json.RawMessage(`{"keepEvents":2,"dryRun":true}`))
	assert.Nil(t, rpcErr)
	preview := result.(compactResult)
	assert.Equal(t, 2, preview.EventsRemoved)
	assert.True(t, preview.DryRun)
	unchanged, err := store.Load(path)
	assert.NoError(t, err)
	assert.Len(t, unchanged.Events, 4)

	result, rpcErr = srv.dispatch("todo.compact", json.RawMessage(`{"keepEvents":2}`))
	assert.Nil(t, rpcErr)
	applied := result.(compactResult)
	assert.Equal(t, 2, applied.EventsRemoved)
	compacted, err := store.Load(path)
	assert.NoError(t, err)
	assert.Len(t, compacted.Events, 2)
	assert.Equal(t, uint64(5), compacted.NextEventID)

	_, rpcErr = srv.dispatch("todo.compact", json.RawMessage(`{"keepEvents":-1}`))
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
}
