package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_LifecycleMutationsReturnVersionedJSON(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	runLifecycleCommand(t, todo, root, "init")
	runLifecycleCommand(t, todo, root, "add", "Parent")

	var taskResult taskEnvelope
	runLifecycleJSONCommand(t, todo, root, &taskResult,
		"block", "1", "--reason", "waiting for approval", "--json")
	assert.Equal(t, protocolVersion, taskResult.SchemaVersion)
	assert.Equal(t, "blocked", taskResult.Task.Status)
	assert.Equal(t, "waiting for approval", taskResult.Task.Metadata.BlockReason)

	taskResult = taskEnvelope{}
	runLifecycleJSONCommand(t, todo, root, &taskResult,
		"unblock", "1", "--as", "coordinator", "--json")
	assert.Equal(t, protocolVersion, taskResult.SchemaVersion)
	assert.Equal(t, "pending", taskResult.Task.Status)
	assert.Empty(t, taskResult.Task.Metadata.BlockReason)

	taskResult = taskEnvelope{}
	runLifecycleJSONCommand(t, todo, root, &taskResult,
		"log", "1", "--as", "coordinator", "--msg", "handoff ready", "--json")
	assert.Equal(t, protocolVersion, taskResult.SchemaVersion)
	require.NotEmpty(t, taskResult.Task.Metadata.Log)
	assert.Equal(t, "handoff ready", taskResult.Task.Metadata.Log[len(taskResult.Task.Metadata.Log)-1].Message)

	var decomposed decomposeEnvelope
	runLifecycleJSONCommand(t, todo, root, &decomposed,
		"decompose", "1", "--as", "coordinator", "--into",
		`{"subtasks":[{"title":"Build","caps":["go"]},{"title":"Verify","caps":["test"]}]}`,
		"--json")
	assert.Equal(t, protocolVersion, decomposed.SchemaVersion)
	assert.Equal(t, uint64(1), decomposed.Parent.ID)
	require.Len(t, decomposed.Subtasks, 2)
	assert.Equal(t, "Build", decomposed.Subtasks[0].Title)
	assert.Equal(t, []string{"go"}, decomposed.Subtasks[0].Metadata.Capabilities)
	assert.Equal(t, []string{"todo://local/2", "todo://local/3"}, decomposed.Parent.Depends)

	runLifecycleCommand(t, todo, root, "add", "Disposable")
	var removed tasksEnvelope
	runLifecycleJSONCommand(t, todo, root, &removed, "rm", "4", "--json")
	assert.Equal(t, protocolVersion, removed.SchemaVersion)
	require.Len(t, removed.Tasks, 1)
	assert.Equal(t, uint64(4), removed.Tasks[0].ID)
	assert.Equal(t, "Disposable", removed.Tasks[0].Title)

	runLifecycleCommand(t, todo, root, "done", "2")
	runLifecycleCommand(t, todo, root, "done", "3")
	runLifecycleCommand(t, todo, root, "done", "1")
	var pruned tasksEnvelope
	runLifecycleJSONCommand(t, todo, root, &pruned, "prune", "--json")
	assert.Equal(t, protocolVersion, pruned.SchemaVersion)
	require.Len(t, pruned.Tasks, 3)
	assert.Equal(t, []uint64{1, 2, 3}, []uint64{
		pruned.Tasks[0].ID,
		pruned.Tasks[1].ID,
		pruned.Tasks[2].ID,
	})

	var compacted compactEnvelope
	runLifecycleJSONCommand(t, todo, root, &compacted,
		"compact", "--keep-events", "0", "--dry-run", "--json")
	assert.Equal(t, protocolVersion, compacted.SchemaVersion)
	assert.True(t, compacted.DryRun)
	assert.Positive(t, compacted.EventsRemoved)
}

func TestCLI_LifecycleJSONErrorsUseVersionedEnvelope(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	runLifecycleCommand(t, todo, root, "init")

	cmd := exec.Command(todo, "block", "999", "--reason", "missing", "--json")
	cmd.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.Error(t, err)
	assert.Empty(t, stdout.String())

	var response errorEnvelope
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &response), stderr.String())
	assert.Equal(t, protocolVersion, response.SchemaVersion)
	assert.Equal(t, ErrTaskNotFound, response.Error.Code)
	assert.Contains(t, response.Error.Message, "task 999 not found")
	assert.NotContains(t, stderr.String(), "Error:")
}

func runLifecycleCommand(t *testing.T, todo, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(todo, args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%v: %s", args, output)
	return output
}

func runLifecycleJSONCommand(t *testing.T, todo, root string, result interface{}, args ...string) {
	t.Helper()
	output := runLifecycleCommand(t, todo, root, args...)
	require.NoError(t, json.Unmarshal(output, result), "%v: %s", args, output)
	assert.NotContains(t, string(output), "Decomposing task")
	assert.NotContains(t, string(output), "Removed task")
}
