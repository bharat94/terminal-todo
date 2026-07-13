package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

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
	params := json.RawMessage(`{"actor":"rpc-agent"}`)
	result, rpcErr := srv.dispatch("todo.acquire", params)
	assert.Nil(t, rpcErr)
	envelope, ok := result.(taskEnvelope)
	assert.True(t, ok)
	assert.Equal(t, "RPC work", envelope.Task.Title)
	assert.Equal(t, "rpc-agent", envelope.Task.Metadata.Owner)

	_, rpcErr = srv.dispatch("todo.acquire", params)
	assert.NotNil(t, rpcErr)
	assert.Equal(t, rpcDependency, rpcErr.Code)
}
