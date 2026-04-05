package main

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLI_Init(t *testing.T) {
	tmpDir := t.TempDir()
	todo := buildTodo(t)

	cmd := exec.Command(todo, "init")
	cmd.Dir = tmpDir
	err := cmd.Run()
	assert.NoError(t, err)

	assert.FileExists(t, filepath.Join(tmpDir, ".terminal-todo", "tasks.bin"))
}

func TestCLI_AddTask(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Test task")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Added task 1")
}

func TestCLI_AddWithDependency(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 2", "--after", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 3", "--after", "1", "--after", "2")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Added task 3")
}

func TestCLI_Done(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "done", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Marked task 1 as done")
}

func TestCLI_Status(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "status")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Task 1")
}

func TestCLI_StatusJSON(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "status", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Task 1"`)
}

func TestCLI_Next(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 2", "--after", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "done", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "next")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Task 2")
}

func TestCLI_NextJSON(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "next", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"ready"`)
}

func TestCLI_Cat(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "cat", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Title:      Task 1")
}

func TestCLI_Depends(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 2", "--after", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "depends", "2")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Task 1")
}

func TestCLI_Dependents(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 2", "--after", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "dependents", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Task 2")
}

func TestCLI_ExportJSON(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "export")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Task 1"`)
}

func TestCLI_ExportMarkdown(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "export", "--markdown")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "## Pending")
}

func TestCLI_Prune(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "done", "1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "add", "Task 2")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "prune")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	statusCmd := exec.Command(todo, "status")
	statusCmd.Dir = tmpDir
	statusOut, _ := statusCmd.CombinedOutput()
	assert.NotContains(t, string(statusOut), "Task 1")
	assert.Contains(t, string(statusOut), "Task 2")
}

func TestCLI_Rm(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task 1")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "rm", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Removed task 1")
}

func buildTodo(t *testing.T) string {
	t.Helper()
	// Build the binary once if it doesn't exist or is old
	path := "/Users/bharatvaidhyanathan/Code/terminal-todo/todo"
	return path
}

func setupTestProject(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	todo := buildTodo(t)

	cmd := exec.Command(todo, "init")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to init: %v %s", err, out)
	}
	return tmpDir
}
