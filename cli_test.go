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

func TestCLI_InitDoesNotOverwriteExistingTasks(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Keep me")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "init")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Already initialized")

	cmd = exec.Command(todo, "status")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Keep me")
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

func TestCLI_AddTaskMetadata(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Review locking", "--priority", "0.9", "--caps", "go, concurrency")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	cmd = exec.Command(todo, "cat", "1", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Review locking"`)
	assert.Contains(t, string(out), `"priority": 0.9`)
	assert.Contains(t, string(out), `"capabilities": [`)
	assert.Contains(t, string(out), `"concurrency"`)
}

func TestCLI_AddRejectsInvalidPriority(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Invalid", "--priority", "high")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "--priority must be between 0 and 1")
}

func TestCLI_RejectsUnknownOptions(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task", "--prioritty", "0.9")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "unknown option --prioritty for add")
}

func TestCLI_RejectsMalformedIDs(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Task")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "done", "1junk")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "task ID required")
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

func TestCLI_AddRejectsMalformedDependencyURI(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Blocked forever", "--after", "todo://local/nope")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "invalid task ID in URI")
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

func TestCLI_ClaimedTaskRequiresOwnerToCompleteOrRelease(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"add", "Owned task"}, {"claim", "1", "--as", "agent-a"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	for _, action := range []string{"done", "release"} {
		cmd := exec.Command(todo, action, "1", "--as", "agent-b")
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.Error(t, err)
		assert.Contains(t, string(out), "claimed by agent-a")
	}

	cmd := exec.Command(todo, "release", "1", "--as", "agent-a")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Released task 1")
}

func TestCLI_NumericOwnerIsNotParsedAsTaskID(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"add", "First"}, {"add", "Second"}, {"claim", "1", "--as", "2"}, {"done", "1", "--as", "2"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "cat", "2", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"status": "pending"`)
}

func TestCLI_ConcurrentClaimsHaveSingleWinner(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	cmd := exec.Command(todo, "add", "Contended task")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	type result struct {
		output string
		err    error
	}
	results := make(chan result, 2)
	for _, owner := range []string{"agent-a", "agent-b"} {
		go func(owner string) {
			claim := exec.Command(todo, "claim", "1", "--as", owner)
			claim.Dir = tmpDir
			out, err := claim.CombinedOutput()
			results <- result{output: string(out), err: err}
		}(owner)
	}

	successes := 0
	failures := 0
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err == nil {
			successes++
		} else {
			failures++
			assert.Contains(t, got.output, "already claimed by")
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
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
	assert.Contains(t, string(out), `"schema_version": "1"`)
	assert.Contains(t, string(out), `"status": "pending"`)
	assert.Contains(t, string(out), `"metadata": {`)
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
	assert.Contains(t, string(out), `"available_tasks"`)
	assert.Contains(t, string(out), `"blocked_summary"`)
	assert.Contains(t, string(out), `"schema_version": "1"`)
}

func TestMatchesCapabilitiesRequiresAllTaskCapabilities(t *testing.T) {
	assert.True(t, matchesCapabilities([]string{"go", "testing"}, []string{"testing", "go", "docs"}))
	assert.False(t, matchesCapabilities([]string{"go", "testing"}, []string{"go"}))
	assert.True(t, matchesCapabilities(nil, nil))
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

func TestCLI_LineageReportsRecursiveProgress(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	commands := [][]string{
		{"add", "Ship objective"},
		{"decompose", "1", "--into", `{"subtasks":[{"title":"Build"},{"title":"Test"}]}`},
		{"decompose", "2", "--into", `{"subtasks":[{"title":"Implement core"}]}`},
		{"done", "4"},
		{"done", "2"},
	}
	for _, args := range commands {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "lineage", "1", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"schema_version": "1"`)
	assert.Contains(t, string(out), `"title": "Ship objective"`)
	assert.Contains(t, string(out), `"title": "Implement core"`)
	assert.Contains(t, string(out), `"total": 4`)
	assert.Contains(t, string(out), `"completed": 2`)
	assert.Contains(t, string(out), `"percent_complete": 50`)
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

func TestCLI_PrunePreservesSatisfiedDependency(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"add", "Foundation"}, {"add", "Follow-up", "--after", "1"}, {"done", "1"}, {"prune"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "next", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Follow-up"`)

	cmd = exec.Command(todo, "cat", "2", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"depends": []`)
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

func TestCLI_RmRefusesDanglingDependency(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"add", "Foundation"}, {"add", "Follow-up", "--after", "1"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		assert.NoError(t, cmd.Run())
	}

	cmd := exec.Command(todo, "rm", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "task 2 depends on it")
}

func buildTodo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "todo")
	cmd := exec.Command("go", "build", "-o", path, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build todo: %v\n%s", err, out)
	}
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
