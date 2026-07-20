package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutationAffectedIsDeterministicUniqueAndBounded(t *testing.T) {
	ids := []uint64{3, 1, 3, 2}
	for id := uint64(4); id <= maxMutationReceiptIDs+5; id++ {
		ids = append(ids, id)
	}

	affected := newMutationAffected(ids)

	assert.Equal(t, maxMutationReceiptIDs+5, affected.Total)
	assert.Len(t, affected.IDs, maxMutationReceiptIDs)
	assert.Equal(t, uint64(1), affected.IDs[0])
	assert.Equal(t, uint64(maxMutationReceiptIDs), affected.IDs[len(affected.IDs)-1])
	assert.True(t, affected.Truncated)
}

func TestCLIReceiptBoundsHistoricTaskStateAndLeavesJSONDetailCompatible(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	runLifecycleCommand(t, todo, root, "init")
	runLifecycleCommand(t, todo, root, "add", "History-heavy task")

	largeValue := strings.Repeat("historic-context-", 4_000)
	var receipt mutationReceipt
	runLifecycleJSONCommand(t, todo, root, &receipt,
		"update", "1", "--set", "finding="+largeValue, "--receipt")
	assertMutationReceipt(t, receipt, "update", 1)

	receipt = mutationReceipt{}
	output := runReceiptJSONCommand(t, todo, root, &receipt,
		"log", "1", "--as", "worker", "--msg", largeValue, "--receipt")
	assertMutationReceipt(t, receipt, "log", 1)
	assert.Less(t, len(output), 2_000)
	assert.NotContains(t, string(output), largeValue)
	assert.NotContains(t, string(output), `"metadata"`)
	assert.NotContains(t, string(output), `"extra"`)

	receipt = mutationReceipt{}
	runLifecycleJSONCommand(t, todo, root, &receipt,
		"block", "1", "--reason", "waiting", "--receipt")
	assertMutationReceipt(t, receipt, "block", 1)
	assert.Equal(t, "blocked", receipt.Task.Status)

	receipt = mutationReceipt{}
	runLifecycleJSONCommand(t, todo, root, &receipt,
		"unblock", "1", "--as", "worker", "--receipt")
	assertMutationReceipt(t, receipt, "unblock", 1)
	assert.Equal(t, "pending", receipt.Task.Status)

	var detail taskEnvelope
	detailOutput := runReceiptJSONCommand(t, todo, root, &detail, "cat", "1", "--json")
	assert.Greater(t, len(detailOutput), len(output)*20)
	assert.Equal(t, largeValue, detail.Task.Metadata.Extra["finding"])
	require.NotEmpty(t, detail.Task.Metadata.Log)
	assert.Equal(t, largeValue, detail.Task.Metadata.Log[len(detail.Task.Metadata.Log)-3].Message)
}

func TestCLIBulkReceiptCapsIDsWhileReportingExactTotal(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	runLifecycleCommand(t, todo, root, "init")

	args := []string{"rm"}
	for id := 1; id <= maxMutationReceiptIDs+7; id++ {
		runLifecycleCommand(t, todo, root, "add", fmt.Sprintf("Disposable %d", id))
		args = append(args, fmt.Sprintf("%d", id))
	}
	args = append(args, "--receipt")

	var receipt mutationReceipt
	output := runReceiptJSONCommand(t, todo, root, &receipt, args...)
	assert.Equal(t, "remove", receipt.Operation)
	assert.Equal(t, maxMutationReceiptIDs+7, receipt.Affected.Total)
	assert.Len(t, receipt.Affected.IDs, maxMutationReceiptIDs)
	assert.True(t, receipt.Affected.Truncated)
	assert.False(t, receipt.DetailFollowUp.Available)
	assert.Less(t, len(output), 2_000)
}

func TestMCPMutationReceiptOmitsHistoryAndCatRetainsIt(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	largeValue := strings.Repeat("handoff-detail-", 4_000)
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	task := s.AddTask("Large handoff", nil)
	task.Extra = map[string]string{"finding": largeValue}
	for i := 0; i < 200; i++ {
		task.Log = append(task.Log, store.LogEntry{
			Timestamp: uint64(time.Now().UnixMilli()),
			Agent:     "worker",
			Message:   largeValue,
		})
	}
	require.NoError(t, s.Save(path))

	srv := &mcpServer{
		backend:        &server{initialized: true},
		initializeSeen: true,
		initialized:    true,
	}
	result, rpcErr := srv.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_acquire",
		"arguments":{"actor":"receipt-worker","requestId":"bounded-acquire","receipt":true}
	}`))
	require.Nil(t, rpcErr)
	call := result.(mcpCallResult)
	require.False(t, call.IsError)
	receipt, ok := call.StructuredContent.(mutationReceipt)
	require.True(t, ok)
	assertMutationReceipt(t, receipt, "acquire", 1)
	require.NotNil(t, receipt.Replayed)
	assert.False(t, *receipt.Replayed)
	require.NotNil(t, receipt.Task.LeaseExpires)

	encoded, err := json.Marshal(receipt)
	require.NoError(t, err)
	assert.Less(t, len(encoded), 2_000)
	assert.NotContains(t, string(encoded), largeValue)
	assert.NotContains(t, string(encoded), "bounded-acquire")
	assert.NotContains(t, string(encoded), "request_id")
	assert.NotContains(t, string(encoded), `"log"`)
	assert.NotContains(t, string(encoded), `"extra"`)

	result, rpcErr = srv.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_cat",
		"arguments":{"id":1}
	}`))
	require.Nil(t, rpcErr)
	detail := result.(mcpCallResult).StructuredContent.(protocolTask)
	assert.Equal(t, largeValue, detail.Metadata.Extra["finding"])
	assert.Len(t, detail.Metadata.Log, 201)
	assert.Equal(t, largeValue, detail.Metadata.Log[0].Message)
}

func TestNativeDecomposeReceiptContainsAllBoundedChildren(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("Parent", nil)
	require.NoError(t, s.Save(path))
	srv := &server{initialized: true}

	subtasks := make([]map[string]interface{}, maxMutationReceiptIDs)
	for i := range subtasks {
		subtasks[i] = map[string]interface{}{"title": fmt.Sprintf("Child %d", i+1)}
	}
	params, err := json.Marshal(map[string]interface{}{
		"id":       1,
		"subtasks": subtasks,
		"receipt":  true,
	})
	require.NoError(t, err)

	result, rpcErr := srv.dispatch("todo.decompose", params)
	require.Nil(t, rpcErr)
	receipt := result.(mutationReceipt)
	assert.Equal(t, "decompose", receipt.Operation)
	assert.Equal(t, maxMutationReceiptIDs, receipt.Affected.Total)
	assert.Len(t, receipt.Affected.IDs, maxMutationReceiptIDs)
	assert.False(t, receipt.Affected.Truncated)
	assert.Equal(t, uint64(1), receipt.Task.ID)
	assert.True(t, receipt.DetailFollowUp.Available)
	assert.Equal(t, "todo.lineage", receipt.DetailFollowUp.NativeMethod)
}

func TestNativeDecomposeRejectsMoreThanReceiptBound(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("Parent", nil)
	require.NoError(t, s.Save(path))

	subtasks := make([]map[string]interface{}, maxMutationReceiptIDs+1)
	for i := range subtasks {
		subtasks[i] = map[string]interface{}{"title": fmt.Sprintf("Child %d", i+1)}
	}
	params, err := json.Marshal(map[string]interface{}{
		"id":       1,
		"subtasks": subtasks,
		"receipt":  true,
	})
	require.NoError(t, err)

	result, rpcErr := (&server{initialized: true}).dispatch("todo.decompose", params)
	assert.Nil(t, result)
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "at most 20")
}

func TestDecomposeBoundMatchesCLIAndMCPSchema(t *testing.T) {
	subtasks := make([]map[string]interface{}, maxMutationReceiptIDs+1)
	for i := range subtasks {
		subtasks[i] = map[string]interface{}{"title": fmt.Sprintf("Child %d", i+1)}
	}

	todo := buildTodo(t)
	root := t.TempDir()
	runLifecycleCommand(t, todo, root, "init")
	runLifecycleCommand(t, todo, root, "add", "Parent")
	payload, err := json.Marshal(map[string]interface{}{"subtasks": subtasks})
	require.NoError(t, err)
	cmd := exec.Command(todo, "decompose", "1", "--into", string(payload), "--receipt")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	var envelope errorEnvelope
	require.NoError(t, json.Unmarshal(output, &envelope), string(output))
	assert.Equal(t, ErrInvalidArgs, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "at most 20")

	var decomposeTool mcpTool
	for _, tool := range terminalTodoMCPTools() {
		if tool.Name == "terminal_todo_decompose" {
			decomposeTool = tool
			break
		}
	}
	properties := decomposeTool.InputSchema["properties"].(map[string]interface{})
	subtaskSchema := properties["subtasks"].(map[string]interface{})
	assert.Equal(t, maxMutationReceiptIDs, subtaskSchema["maxItems"])
}

func TestMutationReceiptParityAcrossCLIJSONRPCAndMCP(t *testing.T) {
	todo := buildTodo(t)
	cliRoot := t.TempDir()
	runLifecycleCommand(t, todo, cliRoot, "init")
	var cliReceipt mutationReceipt
	runReceiptJSONCommand(t, todo, cliRoot, &cliReceipt, "add", "Parity task", "--receipt")

	callAdd := func(root string, throughMCP bool) mutationReceipt {
		oldRoot := projectRoot
		projectRoot = root
		defer func() { projectRoot = oldRoot }()
		require.NoError(t, store.NewTaskStore().Save(filepath.Join(root, ".terminal-todo", "tasks.bin")))

		if !throughMCP {
			result, rpcErr := (&server{initialized: true}).dispatch(
				"todo.add",
				json.RawMessage(`{"title":"Parity task","receipt":true}`),
			)
			require.Nil(t, rpcErr)
			return result.(mutationReceipt)
		}

		srv := &mcpServer{
			backend:        &server{initialized: true},
			initializeSeen: true,
			initialized:    true,
		}
		result, rpcErr := srv.dispatch("tools/call", json.RawMessage(`{
			"name":"terminal_todo_add",
			"arguments":{"title":"Parity task","receipt":true}
		}`))
		require.Nil(t, rpcErr)
		call := result.(mcpCallResult)
		require.False(t, call.IsError)
		return call.StructuredContent.(mutationReceipt)
	}

	assert.Equal(t, cliReceipt, callAdd(t.TempDir(), false))
	assert.Equal(t, cliReceipt, callAdd(t.TempDir(), true))
}

func TestMutationMCPToolsAdvertiseReceiptOptIn(t *testing.T) {
	mutations := map[string]bool{
		"terminal_todo_add":       true,
		"terminal_todo_acquire":   true,
		"terminal_todo_heartbeat": true,
		"terminal_todo_update":    true,
		"terminal_todo_log":       true,
		"terminal_todo_decompose": true,
		"terminal_todo_block":     true,
		"terminal_todo_release":   true,
		"terminal_todo_complete":  true,
	}
	for _, tool := range terminalTodoMCPTools() {
		if !mutations[tool.Name] {
			continue
		}
		properties := tool.InputSchema["properties"].(map[string]interface{})
		receipt, ok := properties["receipt"].(map[string]interface{})
		require.True(t, ok, tool.Name)
		assert.Equal(t, "boolean", receipt["type"])
		delete(mutations, tool.Name)
	}
	assert.Empty(t, mutations)
}

func assertMutationReceipt(t *testing.T, receipt mutationReceipt, operation string, taskID uint64) {
	t.Helper()
	assert.Equal(t, protocolVersion, receipt.SchemaVersion)
	assert.Equal(t, operation, receipt.Operation)
	require.NotNil(t, receipt.Task)
	assert.Equal(t, taskID, receipt.Task.ID)
	assert.Equal(t, mutationAffected{Total: 1, IDs: []uint64{taskID}}, receipt.Affected)
	assert.True(t, receipt.DetailFollowUp.Available)
	assert.Equal(t, "todo.cat", receipt.DetailFollowUp.NativeMethod)
	assert.Equal(t, "terminal_todo_cat", receipt.DetailFollowUp.MCPTool)
}

func runReceiptJSONCommand(t *testing.T, todo, root string, result interface{}, args ...string) []byte {
	t.Helper()
	output := runLifecycleCommand(t, todo, root, args...)
	require.NoError(t, json.Unmarshal(output, result), "%v: %s", args, output)
	return output
}
