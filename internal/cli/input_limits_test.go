package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistedInputLimitsCountUTF8Bytes(t *testing.T) {
	exact := strings.Repeat("é", maxTaskTitleBytes/2)
	require.Len(t, []byte(exact), maxTaskTitleBytes)
	assert.NoError(t, validateRequiredPersistedString("title", exact, maxTaskTitleBytes))
	assert.EqualError(t,
		validateRequiredPersistedString("title", exact+"é", maxTaskTitleBytes),
		"title must be at most 1024 UTF-8 bytes",
	)
	assert.EqualError(t,
		validatePersistedString("title", string([]byte{0xff}), maxTaskTitleBytes),
		"title must be valid UTF-8",
	)
}

func TestCLIRejectsOversizedPersistedInputBeforeWriting(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	initCommand := exec.Command(todo, "init")
	initCommand.Dir = root
	require.NoError(t, initCommand.Run())

	command := exec.Command(todo, "add", strings.Repeat("x", maxTaskTitleBytes+1), "--json")
	command.Dir = root
	output, err := command.CombinedOutput()
	require.Error(t, err)

	var envelope errorEnvelope
	require.NoError(t, json.Unmarshal(output, &envelope), string(output))
	assert.Equal(t, ErrInvalidArgs, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "title must be at most")

	persisted, err := store.Load(filepath.Join(root, ".terminal-todo", "tasks.bin"))
	require.NoError(t, err)
	assert.Empty(t, persisted.Tasks)
}

func TestServerRejectsOversizedPersistedInputBeforeWriting(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	s.AddTask("existing", nil)
	require.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	tests := []struct {
		method string
		params interface{}
	}{
		{"todo.add", addParams{Title: strings.Repeat("x", maxTaskTitleBytes+1)}},
		{"todo.add", addParams{Title: "work", Capabilities: numberedValues("cap", maxTaskCapabilities+1)}},
		{"todo.update", updateParams{ID: 1, Extra: map[string]string{"finding": strings.Repeat("x", maxMetadataValueBytes+1)}}},
		{"todo.handoff", handoffParams{ID: 1, Actor: "worker", Extra: map[string]string{"finding": strings.Repeat("x", maxMetadataValueBytes+1)}}},
		{"todo.block", blockParams{ID: 1, Reason: strings.Repeat("x", maxReasonBytes+1)}},
		{"todo.log", logParams{ID: 1, Message: strings.Repeat("x", maxLogMessageBytes+1)}},
		{"todo.done", doneParams{IDs: []uint64{1}, Actor: strings.Repeat("x", maxActorBytes+1)}},
	}
	for _, test := range tests {
		params, err := json.Marshal(test.params)
		require.NoError(t, err)
		_, rpcErr := srv.dispatch(test.method, params)
		require.NotNil(t, rpcErr, test.method)
		assert.Equal(t, rpcInvalidParams, rpcErr.Code, test.method)
	}

	persisted, err := store.Load(path)
	require.NoError(t, err)
	assert.Len(t, persisted.Tasks, 1)
	assert.Equal(t, "existing", persisted.Tasks[1].Title)
	assert.Empty(t, persisted.Tasks[1].Extra)
	assert.Empty(t, persisted.Tasks[1].Log)
	assert.Empty(t, persisted.Events)
}

func TestServerCanRepairLegacyOversizedStateButCannotGrowIt(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	s := store.NewTaskStore()
	task := s.AddTask(strings.Repeat("x", maxTaskTitleBytes+1), nil)
	task.Extra = make(map[string]string, maxTaskExtraEntries+1)
	for i := 0; i <= maxTaskExtraEntries; i++ {
		task.Extra[fmt.Sprintf("key-%03d", i)] = "legacy"
		task.Depends = append(task.Depends, fmt.Sprintf("todo://repo-%03d/1", i))
	}
	require.NoError(t, s.Save(path))

	srv := &server{initialized: true}
	newTitle := "repaired title"
	result, rpcErr := srv.handleUpdate(mustJSON(t, updateParams{
		ID:    task.ID,
		Title: &newTitle,
		Extra: map[string]string{"key-000": "updated"},
	}))
	require.Nil(t, rpcErr)
	assert.Equal(t, newTitle, result.(protocolTask).Title)

	_, rpcErr = srv.handleUpdate(mustJSON(t, updateParams{ID: task.ID, Extra: map[string]string{"new-key": "growth"}}))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "extra must contain at most")

	_, rpcErr = srv.handleUpdate(mustJSON(t, updateParams{ID: task.ID, AddDeps: []string{"todo://another-repo/1"}}))
	require.NotNil(t, rpcErr)
	assert.Equal(t, rpcInvalidParams, rpcErr.Code)
	assert.Contains(t, rpcErr.Message, "dependencies must contain at most")

	persisted, err := store.Load(path)
	require.NoError(t, err)
	assert.Equal(t, newTitle, persisted.Tasks[task.ID].Title)
	assert.Equal(t, "updated", persisted.Tasks[task.ID].Extra["key-000"])
	assert.NotContains(t, persisted.Tasks[task.ID].Extra, "new-key")
	assert.Len(t, persisted.Tasks[task.ID].Depends, maxTaskDependencies+1)
}

func TestMCPToolSchemasAdvertisePersistedInputLimits(t *testing.T) {
	tools := make(map[string]mcpTool)
	for _, tool := range terminalTodoMCPTools() {
		tools[tool.Name] = tool
	}

	addProperties := tools["terminal_todo_add"].InputSchema["properties"].(map[string]interface{})
	assert.Equal(t, maxTaskTitleBytes, addProperties["title"].(map[string]interface{})["maxLength"])
	assert.Equal(t, maxTaskDependencies, addProperties["after"].(map[string]interface{})["maxItems"])
	assert.Equal(t, maxTaskCapabilities, addProperties["capabilities"].(map[string]interface{})["maxItems"])
	assert.Equal(t, maxTaskTags, addProperties["tags"].(map[string]interface{})["maxItems"])

	updateProperties := tools["terminal_todo_update"].InputSchema["properties"].(map[string]interface{})
	assert.Equal(t, maxTaskExtraEntries, updateProperties["extra"].(map[string]interface{})["maxProperties"])
	logProperties := tools["terminal_todo_log"].InputSchema["properties"].(map[string]interface{})
	assert.Equal(t, maxLogMessageBytes, logProperties["message"].(map[string]interface{})["maxLength"])
}

func TestMCPRejectsOversizedPersistedInputWithoutWriting(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	path := filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")
	require.NoError(t, store.NewTaskStore().Save(path))

	arguments := mustJSON(t, addParams{Title: strings.Repeat("x", maxTaskTitleBytes+1)})
	srv := &mcpServer{backend: &server{initialized: true}}
	result, rpcErr := srv.callTool(mustJSON(t, mcpCallParams{
		Name:      "terminal_todo_add",
		Arguments: arguments,
	}))
	require.Nil(t, rpcErr)
	call := result.(mcpCallResult)
	assert.True(t, call.IsError)
	detail := call.StructuredContent.(map[string]interface{})
	assert.Equal(t, rpcInvalidParams, detail["code"])
	assert.Contains(t, detail["message"], "title must be at most")

	persisted, err := store.Load(path)
	require.NoError(t, err)
	assert.Empty(t, persisted.Tasks)
}

func numberedValues(prefix string, count int) []string {
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf("%s-%03d", prefix, i)
	}
	return values
}

func mustJSON(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}
