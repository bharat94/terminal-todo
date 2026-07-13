package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"terminal-todo/store"

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
}
