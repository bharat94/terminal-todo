package main

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bharat94/terminal-todo/store"

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

func TestCLI_HelpAndVersionWorkOutsideAProject(t *testing.T) {
	tmpDir := t.TempDir()
	todo := buildTodo(t)

	for _, args := range [][]string{{"--version"}, {"help"}, {"--help"}, {"-h"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, "%v: %s", args, string(out))
		assert.NotContains(t, string(out), "not in a project")
	}
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

func TestCLI_DoctorFixPreservesStableLockFile(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	lockPath := filepath.Join(tmpDir, ".terminal-todo", "tasks.bin.lock")
	assert.FileExists(t, lockPath)

	cmd := exec.Command(todo, "doctor", "--fix")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Lock files (persistent)")
	assert.FileExists(t, lockPath)
}

func TestCLI_DoctorFixRepairsPrivateStatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	stateDir := filepath.Join(tmpDir, ".terminal-todo")
	tasksPath := filepath.Join(stateDir, "tasks.bin")
	lockPath := tasksPath + ".lock"
	assert.NoError(t, os.Chmod(stateDir, 0755))
	assert.NoError(t, os.Chmod(tasksPath, 0644))
	assert.NoError(t, os.Chmod(lockPath, 0644))

	cmd := exec.Command(todo, "doctor", "--fix")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Local privacy permissions")
	assert.Contains(t, string(out), "fixed")

	for _, check := range []struct {
		path string
		mode os.FileMode
	}{
		{stateDir, 0700},
		{tasksPath, 0600},
		{lockPath, 0600},
	} {
		info, err := os.Stat(check.path)
		assert.NoError(t, err)
		assert.Equal(t, check.mode, info.Mode().Perm(), check.path)
	}
}

func TestDoctorDetectsStoreInvariantProblems(t *testing.T) {
	s := store.NewTaskStore()
	task := s.AddTask("broken", nil)
	task.Status = store.StatusInProgress
	task.Priority = float32(math.NaN())
	task.Depends = []string{"todo://local/99"}
	s.NextID = task.ID

	problems := storeInvariantProblems(s)
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "invalid priority")
	assert.Contains(t, joined, "without a complete ownership lease")
	assert.Contains(t, joined, "references missing local task 99")
	assert.Contains(t, joined, "must be greater than maximum task ID")
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

	for _, value := range []string{"high", "NaN", "+Inf", "1.1"} {
		cmd := exec.Command(todo, "add", "Invalid", "--priority", value)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.Error(t, err, value)
		assert.Contains(t, string(out), "--priority must be between 0 and 1", value)
	}
	for _, value := range []string{"-Inf", "-0.1"} {
		cmd := exec.Command(todo, "add", "Invalid", "--priority", value)
		cmd.Dir = tmpDir
		_, err := cmd.CombinedOutput()
		assert.Error(t, err, value)
	}
}

func TestCLI_UpdateAndConfigRejectNonFinitePriority(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	add := exec.Command(todo, "add", "Valid")
	add.Dir = tmpDir
	assert.NoError(t, add.Run())

	for _, args := range [][]string{
		{"update", "1", "--priority", "NaN"},
		{"config", "default_priority=+Inf"},
	} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.Error(t, err, "%v", args)
		assert.Contains(t, string(out), "priority must be between 0 and 1")
	}

	persisted, err := store.LoadCurrent(filepath.Join(tmpDir, ".terminal-todo", "tasks.bin"))
	assert.NoError(t, err)
	assert.Equal(t, float32(0.5), persisted.Tasks[1].Priority)
}

func TestCLI_UpdateAddsHandoffContext(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "add", "Investigate")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	cmd = exec.Command(todo, "update", "1", "--title", "Fix locking", "--priority", "0.95", "--caps", "go,concurrency,go", "--set", "finding=lost update", "--set", "file=store/store.go", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Fix locking"`)
	assert.Contains(t, string(out), `"priority": 0.95`)
	assert.Contains(t, string(out), `"finding": "lost update"`)
	assert.Contains(t, string(out), `"file": "store/store.go"`)
	assert.Equal(t, 1, strings.Count(string(out), `"go"`))
}

func TestCLI_UpdateEnforcesClaimOwner(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"add", "Owned"}, {"claim", "1", "--as", "agent-a"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		assert.NoError(t, cmd.Run())
	}

	cmd := exec.Command(todo, "update", "1", "--set", "finding=secret", "--as", "agent-b")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "claimed by agent-a")

	cmd = exec.Command(todo, "update", "1", "--set", "finding=shared", "--as", "agent-a")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
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

func TestCLI_DependencyCompletionDoesNotEraseManualBlock(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	commands := [][]string{
		{"add", "Dependency"},
		{"add", "Manually blocked", "--after", "1"},
		{"block", "2", "--reason", "waiting for approval"},
		{"done", "1"},
	}
	for _, args := range commands {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "cat", "2", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"status": "blocked"`)
	assert.Contains(t, string(out), `"block_reason": "waiting for approval"`)

	cmd = exec.Command(todo, "next", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.NotContains(t, string(out), `"title": "Manually blocked"`)
}

func TestCLI_BlockReleasesLeaseAndUnblockClearsLegacyOwnership(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	for _, args := range [][]string{
		{"add", "Externally blocked work"},
		{"claim", "1", "--as", "worker", "--ttl", "1h"},
		{"block", "1", "--as", "worker", "--reason", "waiting for approval"},
	} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, "%v: %s", args, out)
	}

	path := filepath.Join(tmpDir, ".terminal-todo", "tasks.bin")
	persisted, err := store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Equal(t, store.StatusBlocked, persisted.Tasks[1].Status)
	assert.Empty(t, persisted.Tasks[1].Owner)
	assert.Zero(t, persisted.Tasks[1].LeaseExpires)

	// Simulate stale ownership written by an older binary. Unblock must repair
	// it instead of requiring an expired owner identity forever.
	persisted.Tasks[1].Owner = "legacy-worker"
	persisted.Tasks[1].LeaseExpires = uint64(time.Now().Add(-time.Hour).UnixMilli())
	assert.NoError(t, persisted.Save(path))

	cmd := exec.Command(todo, "unblock", "1", "--as", "coordinator")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	persisted, err = store.LoadCurrent(path)
	assert.NoError(t, err)
	assert.Equal(t, store.StatusPending, persisted.Tasks[1].Status)
	assert.Empty(t, persisted.Tasks[1].Owner)
	assert.Zero(t, persisted.Tasks[1].LeaseExpires)
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

func TestCLI_ClaimUsesConfiguredTTLAndRegistersAgent(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{{"config", "default_ttl=2h"}, {"add", "Configured lease"}} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "claim", "1", "--as", "agent-config")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "expires in 2h0m0s")

	cmd = exec.Command(todo, "agent-card", "--as", "agent-config", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"name": "agent-config"`)
	assert.Contains(t, string(out), `"current_load": 1`)
}

func TestCLI_HeartbeatRenewsOnlyTheOwnersActiveLease(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	for _, args := range [][]string{
		{"add", "Long-running task"},
		{"claim", "1", "--as", "agent-a", "--ttl", "15m"},
	} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	storePath := filepath.Join(tmpDir, ".terminal-todo", "tasks.bin")
	before, err := store.LoadCurrent(storePath)
	assert.NoError(t, err)
	beforeExpiry := before.Tasks[1].LeaseExpires

	cmd := exec.Command(todo, "heartbeat", "1", "--as", "agent-a", "--ttl", "2h", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"schema_version": "1"`)
	assert.Contains(t, string(out), `"status": "in_progress"`)
	assert.Contains(t, string(out), `"owner": "agent-a"`)

	after, err := store.LoadCurrent(storePath)
	assert.NoError(t, err)
	assert.Greater(t, after.Tasks[1].LeaseExpires, beforeExpiry)
	assert.Equal(t, store.EventLeaseRenewed, after.Events[len(after.Events)-1].Type)

	cmd = exec.Command(todo, "heartbeat", "1", "--as", "agent-b", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.Error(t, err)
	var ownerExit *exec.ExitError
	assert.True(t, errors.As(err, &ownerExit))
	assert.Equal(t, 1, ownerExit.ExitCode())
	assert.Contains(t, string(out), `"code": "NOT_OWNER"`)

	cmd = exec.Command(todo, "release", "1", "--as", "agent-a")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	cmd = exec.Command(todo, "heartbeat", "1", "--as", "agent-a", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.Error(t, err)
	var inactiveExit *exec.ExitError
	assert.True(t, errors.As(err, &inactiveExit))
	assert.Equal(t, 9, inactiveExit.ExitCode())
	assert.Contains(t, string(out), `"code": "LEASE_NOT_ACTIVE"`)
}

func TestCLI_CoreMutationsReturnVersionedJSON(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	commands := []struct {
		args   []string
		checks []string
	}{
		{[]string{"add", "Machine task", "--json"}, []string{`"schema_version": "1"`, `"status": "pending"`}},
		{[]string{"claim", "1", "--as", "json-agent", "--json"}, []string{`"status": "in_progress"`, `"owner": "json-agent"`}},
		{[]string{"heartbeat", "1", "--as", "json-agent", "--json"}, []string{`"status": "in_progress"`, `"owner": "json-agent"`}},
		{[]string{"release", "1", "--as", "json-agent", "--error", "retry me", "--json"}, []string{`"status": "pending"`, `"retry_count": 1`, `"last_error": "retry me"`}},
		{[]string{"claim", "1", "--as", "json-agent", "--json"}, []string{`"status": "in_progress"`}},
		{[]string{"done", "1", "--as", "json-agent", "--json"}, []string{`"status": "completed"`}},
	}
	for _, test := range commands {
		cmd := exec.Command(todo, test.args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
		for _, check := range test.checks {
			assert.Contains(t, string(out), check)
		}
	}
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

func TestCLI_AcquireAtomicallySelectsCompatibleWorkAndEnforcesCapacity(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	commands := [][]string{
		{"agent-card", "--as", "allocator", "--caps", "go", "--max-load", "1"},
		{"add", "Low priority", "--priority", "0.2", "--caps", "go"},
		{"add", "High priority", "--priority", "0.9", "--caps", "go"},
		{"add", "Wrong capability", "--priority", "1", "--caps", "python"},
	}
	for _, args := range commands {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "acquire", "--as", "allocator", "--request-id", "allocation-1", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"request_id": "allocation-1"`)
	assert.Contains(t, string(out), `"replayed": false`)
	assert.Contains(t, string(out), `"title": "High priority"`)
	assert.Contains(t, string(out), `"owner": "allocator"`)

	cmd = exec.Command(todo, "acquire", "--as", "allocator", "--request-id", "allocation-1", "--wait", "1s", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"replayed": true`)
	assert.Contains(t, string(out), `"title": "High priority"`)

	cmd = exec.Command(todo, "acquire", "--as", "allocator", "--request-id", "allocation-1", "--ttl", "2h", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.Error(t, err)
	var conflictExit *exec.ExitError
	assert.True(t, errors.As(err, &conflictExit))
	assert.Equal(t, 8, conflictExit.ExitCode())
	assert.Contains(t, string(out), `"code": "IDEMPOTENCY_CONFLICT"`)

	cmd = exec.Command(todo, "acquire", "--as", "allocator", "--request-id", "allocation-2", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.Error(t, err)
	var exitErr *exec.ExitError
	assert.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 7, exitErr.ExitCode())
	assert.Contains(t, string(out), `"code": "AGENT_AT_CAPACITY"`)
	assert.Contains(t, string(out), "reached max load 1")

	persisted, loadErr := store.LoadCurrent(filepath.Join(tmpDir, ".terminal-todo", "tasks.bin"))
	assert.NoError(t, loadErr)
	assert.Len(t, persisted.Acquisitions, 1)
	claimedEvents := 0
	for _, event := range persisted.Events {
		if event.Type == store.EventTaskClaimed {
			claimedEvents++
		}
	}
	assert.Equal(t, 1, claimedEvents)
}

func TestCLI_ConcurrentAcquireHasSingleWinner(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	cmd := exec.Command(todo, "add", "Only task")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	type acquireResult struct {
		err error
		out []byte
	}
	results := make(chan acquireResult, 2)
	for _, actor := range []string{"agent-a", "agent-b"} {
		go func(actor string) {
			acquire := exec.Command(todo, "acquire", "--as", actor, "--request-id", "request-"+actor, "--json")
			acquire.Dir = tmpDir
			out, err := acquire.CombinedOutput()
			results <- acquireResult{err: err, out: out}
		}(actor)
	}
	successes := 0
	failures := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			successes++
			continue
		}
		failures++
		var exitErr *exec.ExitError
		assert.True(t, errors.As(result.err, &exitErr))
		assert.Equal(t, 6, exitErr.ExitCode())
		assert.Contains(t, string(result.out), `"code": "NO_WORK"`)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
}

func TestCLI_AcquireWaitsForCompatibleReadyWork(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	cmd := exec.Command(todo, "add", "Wrong capability", "--caps", "python")
	cmd.Dir = tmpDir
	assert.NoError(t, cmd.Run())

	type acquireResult struct {
		err error
		out []byte
	}
	result := make(chan acquireResult, 1)
	go func() {
		acquire := exec.Command(todo, "acquire", "--as", "waiting-agent", "--request-id", "waiting-request", "--capabilities", "go", "--wait", "3s", "--json")
		acquire.Dir = tmpDir
		out, err := acquire.CombinedOutput()
		result <- acquireResult{err: err, out: out}
	}()

	time.Sleep(200 * time.Millisecond)
	cmd = exec.Command(todo, "add", "New Go work", "--caps", "go")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	acquired := <-result
	assert.NoError(t, acquired.err, string(acquired.out))
	assert.Contains(t, string(acquired.out), `"title": "New Go work"`)
	assert.Contains(t, string(acquired.out), `"owner": "waiting-agent"`)
}

func TestCLI_AcquireWaitTimesOutWithNoWork(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	started := time.Now()

	cmd := exec.Command(todo, "acquire", "--as", "waiting-agent", "--request-id", "timeout-request", "--wait", "100ms", "--json")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()

	assert.Error(t, err)
	assert.True(t, time.Since(started) >= 75*time.Millisecond)
	var exitErr *exec.ExitError
	assert.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 6, exitErr.ExitCode())
	assert.Contains(t, string(out), `"code": "NO_WORK"`)
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

func TestCLI_DecomposeAuthorizesOwnerAndReleasesParentLease(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	for _, args := range [][]string{
		{"add", "Owned objective"},
		{"claim", "1", "--as", "planner", "--ttl", "1h"},
	} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "decompose", "1", "--as", "other-agent", "--into", `{"subtasks":[{"title":"Unauthorized"}]}`)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "claimed by planner")

	cmd = exec.Command(todo, "decompose", "1", "--as", "planner", "--into", `{"subtasks":[{"title":"Build"},{"title":"Test"}]}`)
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))

	cmd = exec.Command(todo, "cat", "1", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"status": "pending"`)
	assert.NotContains(t, string(out), `"owner"`)
	assert.NotContains(t, string(out), `"lease_expires"`)

	cmd = exec.Command(todo, "events", "--json")
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"type": "decomposed"`)
	assert.Contains(t, string(out), `"actor": "planner"`)
}

func TestCLI_LinkResolvesCrossRepositoryDependency(t *testing.T) {
	todo := buildTodo(t)
	frontend := t.TempDir()
	backend := t.TempDir()
	for _, project := range []string{frontend, backend} {
		cmd := exec.Command(todo, "init")
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		assert.NoError(t, err, string(out))
	}

	cmd := exec.Command(todo, "add", "Backend API")
	cmd.Dir = backend
	assert.NoError(t, cmd.Run())
	cmd = exec.Command(todo, "link", "backend", backend)
	cmd.Dir = frontend
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	cmd = exec.Command(todo, "add", "Frontend integration", "--after", "todo://backend/1")
	cmd.Dir = frontend
	assert.NoError(t, cmd.Run())
	cmd = exec.Command(todo, "status", "--all", "--json")
	cmd.Dir = frontend
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"alias": "local"`)
	assert.Contains(t, string(out), `"alias": "backend"`)
	assert.Contains(t, string(out), `"title": "Backend API"`)
	assert.Contains(t, string(out), `"title": "Frontend integration"`)

	cmd = exec.Command(todo, "next", "--json")
	cmd.Dir = frontend
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.NotContains(t, string(out), `"title": "Frontend integration"`)
	assert.Contains(t, string(out), `"todo://backend/1"`)

	cmd = exec.Command(todo, "done", "1")
	cmd.Dir = backend
	assert.NoError(t, cmd.Run())
	cmd = exec.Command(todo, "next", "--json")
	cmd.Dir = frontend
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"title": "Frontend integration"`)
	assert.NotContains(t, string(out), `"todo://backend/1"`)

	cmd = exec.Command(todo, "claim", "1", "--as", "frontend-agent")
	cmd.Dir = frontend
	out, err = cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
}

func TestCLI_LinkRejectsCurrentProject(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)

	cmd := exec.Command(todo, "link", "self", tmpDir)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "cannot link a project to itself")
}

func TestCLI_ConcurrentLinksPreserveBothRepositories(t *testing.T) {
	todo := buildTodo(t)
	root := t.TempDir()
	serviceA := t.TempDir()
	serviceB := t.TempDir()
	for _, project := range []string{root, serviceA, serviceB} {
		cmd := exec.Command(todo, "init")
		cmd.Dir = project
		assert.NoError(t, cmd.Run())
	}

	errs := make(chan error, 2)
	for alias, path := range map[string]string{"service-a": serviceA, "service-b": serviceB} {
		go func(alias, path string) {
			cmd := exec.Command(todo, "link", alias, path)
			cmd.Dir = root
			_, err := cmd.CombinedOutput()
			errs <- err
		}(alias, path)
	}
	assert.NoError(t, <-errs)
	assert.NoError(t, <-errs)

	cmd := exec.Command(todo, "status", "--all", "--json")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), `"alias": "service-a"`)
	assert.Contains(t, string(out), `"alias": "service-b"`)
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

func TestCLI_RmRefusesActiveLease(t *testing.T) {
	tmpDir := setupTestProject(t)
	todo := buildTodo(t)
	for _, args := range [][]string{
		{"add", "Owned work"},
		{"claim", "1", "--as", "worker", "--ttl", "1h"},
	} {
		cmd := exec.Command(todo, args...)
		cmd.Dir = tmpDir
		assert.NoError(t, cmd.Run())
	}

	cmd := exec.Command(todo, "rm", "1")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "active lease is owned by worker")
}

func buildTodo(t *testing.T) string {
	t.Helper()
	name := "todo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
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
