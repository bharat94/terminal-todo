package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In-process command execution.
//
// Command handlers write to stdout and end the process through fail, so the
// only way to observe one was to build a binary and run it. That made every
// CLI test a subprocess test: slow, and invisible to coverage because no
// statement in cmd_*.go was ever attributed to the test run.
//
// runCommand executes a handler in this process with stdout, stderr, and the
// exit status captured. It is not parallel-safe — it replaces process-global
// state — so these tests do not call t.Parallel.

type commandResult struct {
	Stdout string
	Stderr string
	Exit   int
}

// exitPanic carries the status through the stack from fail to runCommand.
type exitPanic struct{ status int }

// runCommand runs one CLI command against root and returns what it produced.
func runCommand(t *testing.T, root string, args ...string) commandResult {
	t.Helper()
	require.NotEmpty(t, args, "a command name is required")

	cmd, known := lookupCommand(args[0])
	require.True(t, known, "unknown command %q", args[0])

	previousRoot, previousActive := projectRoot, activeCommand
	previousExit := exitProcess
	previousArgs := os.Args
	defer func() {
		projectRoot, activeCommand = previousRoot, previousActive
		exitProcess = previousExit
		os.Args = previousArgs
	}()

	projectRoot = root
	activeCommand = cmd
	exitProcess = func(status int) { panic(exitPanic{status}) }
	// fail and writeJSON decide between human and structured output by
	// inspecting os.Args, so the process view has to match the call.
	os.Args = append([]string{"todo"}, args...)

	stdout, stderr, restore := captureOutput(t)
	result := commandResult{}

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				exit, ok := recovered.(exitPanic)
				if !ok {
					restore()
					panic(recovered)
				}
				result.Exit = exit.status
			}
		}()
		if err := cmd.validateArgs(args[1:]); err != nil {
			fail(ErrInvalidArgs, "%v", err)
		}
		cmd.Run(args[1:])
	}()

	restore()
	result.Stdout, result.Stderr = stdout(), stderr()
	return result
}

// captureOutput redirects the process streams into pipes. The returned
// accessors are valid only after restore has run.
func captureOutput(t *testing.T) (stdout func() string, stderr func() string, restore func()) {
	t.Helper()
	originalOut, originalErr := os.Stdout, os.Stderr

	outRead, outWrite, err := os.Pipe()
	require.NoError(t, err)
	errRead, errWrite, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout, os.Stderr = outWrite, errWrite

	outDone := make(chan string, 1)
	errDone := make(chan string, 1)
	go func() { data, _ := io.ReadAll(outRead); outDone <- string(data) }()
	go func() { data, _ := io.ReadAll(errRead); errDone <- string(data) }()

	var capturedOut, capturedErr string
	var restored bool
	restore = func() {
		if restored {
			return
		}
		restored = true
		outWrite.Close()
		errWrite.Close()
		os.Stdout, os.Stderr = originalOut, originalErr
		capturedOut, capturedErr = <-outDone, <-errDone
	}
	t.Cleanup(restore)
	return func() string { return capturedOut }, func() string { return capturedErr }, restore
}

// newProject creates an initialized store and returns its root.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".terminal-todo"), 0o700))

	previous := projectRoot
	projectRoot = root
	defer func() { projectRoot = previous }()
	require.NoError(t, store.NewTaskStore().Save(tasksBinPath()))
	return root
}

// mustRun runs a command that is expected to succeed.
func mustRun(t *testing.T, root string, args ...string) commandResult {
	t.Helper()
	result := runCommand(t, root, args...)
	require.Zero(t, result.Exit, "%v failed: %s%s", args, result.Stdout, result.Stderr)
	return result
}

func TestInProcessLifecycleWalkthrough(t *testing.T) {
	root := newProject(t)

	added := mustRun(t, root, "add", "Implement token validation", "--caps", "go,security", "--priority", "0.9")
	assert.Contains(t, added.Stdout, "Added task 1")

	mustRun(t, root, "add", "Add tests", "--after", "1")

	claimed := mustRun(t, root, "claim", "1", "--as", "w1", "--ttl", "30m")
	assert.Contains(t, claimed.Stdout, "claimed by w1")

	mustRun(t, root, "heartbeat", "1", "--as", "w1")
	mustRun(t, root, "log", "1", "--as", "w1", "--msg", "validated against staging")
	mustRun(t, root, "done", "1", "--as", "w1")

	// Completing the prerequisite must make the dependent claimable.
	mustRun(t, root, "claim", "2", "--as", "w1")
	mustRun(t, root, "done", "2", "--as", "w1")

	status := mustRun(t, root, "status", "--all")
	assert.Contains(t, status.Stdout, "Implement token validation")
}

func TestInProcessFailuresReportDocumentedCodesAndExitStatuses(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		args  []string
		code  ErrorCode
	}{
		{
			name: "claim_missing_task",
			args: []string{"claim", "99", "--as", "w1"},
			code: ErrTaskNotFound,
		},
		{
			name: "claim_held_by_another_agent",
			setup: func(t *testing.T, root string) {
				mustRun(t, root, "add", "Work")
				mustRun(t, root, "claim", "1", "--as", "w1")
			},
			args: []string{"claim", "1", "--as", "w2"},
			code: ErrAlreadyClaimed,
		},
		{
			name: "complete_owned_by_another_agent",
			setup: func(t *testing.T, root string) {
				mustRun(t, root, "add", "Work")
				mustRun(t, root, "claim", "1", "--as", "w1")
			},
			args: []string{"done", "1", "--as", "w2"},
			code: ErrNotOwner,
		},
		{
			name: "complete_with_incomplete_dependencies",
			setup: func(t *testing.T, root string) {
				mustRun(t, root, "add", "Prerequisite")
				mustRun(t, root, "add", "Dependent", "--after", "1")
			},
			args: []string{"done", "2"},
			code: ErrDependency,
		},
		{
			name:  "release_without_a_lease",
			setup: func(t *testing.T, root string) { mustRun(t, root, "add", "Work") },
			args:  []string{"release", "1", "--as", "w1"},
			code:  ErrLeaseNotActive,
		},
		{
			name:  "unblock_work_that_is_not_blocked",
			setup: func(t *testing.T, root string) { mustRun(t, root, "add", "Work") },
			args:  []string{"unblock", "1"},
			code:  ErrInvalidTransition,
		},
		{
			name:  "undeclared_flag",
			setup: func(t *testing.T, root string) { mustRun(t, root, "add", "Work") },
			args:  []string{"claim", "1", "--as", "w1", "--nope"},
			code:  ErrInvalidArgs,
		},
		{
			name:  "value_flag_without_a_value",
			setup: func(t *testing.T, root string) { mustRun(t, root, "add", "Work") },
			args:  []string{"claim", "1", "--as"},
			code:  ErrInvalidArgs,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			if tc.setup != nil {
				tc.setup(t, root)
			}

			result := runCommand(t, root, append(tc.args, "--json")...)
			assert.Equal(t, exitCode(tc.code), result.Exit,
				"exit status must match docs/agent-protocol.md")

			var envelope errorEnvelope
			require.NoError(t, json.Unmarshal([]byte(result.Stderr), &envelope),
				"error output: %s", result.Stderr)
			assert.Equal(t, tc.code, envelope.Error.Code)
		})
	}
}

func TestInProcessAcquireIsIdempotentForARepeatedRequestID(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Work")

	first := mustRun(t, root, "acquire", "--as", "w1", "--request-id", "req-1", "--json")
	var firstEnvelope acquireEnvelope
	require.NoError(t, json.Unmarshal([]byte(first.Stdout), &firstEnvelope))
	assert.False(t, firstEnvelope.Replayed)

	// A retried request with the same ID must replay rather than allocate a
	// second task. This is what makes a lost response safe.
	second := mustRun(t, root, "acquire", "--as", "w1", "--request-id", "req-1", "--json")
	var secondEnvelope acquireEnvelope
	require.NoError(t, json.Unmarshal([]byte(second.Stdout), &secondEnvelope))
	assert.True(t, secondEnvelope.Replayed)
	assert.Equal(t, firstEnvelope.Task.ID, secondEnvelope.Task.ID)
}

func TestInProcessAcquireReportsNoWorkWithDiagnostics(t *testing.T) {
	root := newProject(t)

	result := runCommand(t, root, "acquire", "--as", "w1", "--request-id", "req-1", "--json")
	assert.Equal(t, exitCode(ErrNoWork), result.Exit)

	var envelope errorEnvelope
	require.NoError(t, json.Unmarshal([]byte(result.Stderr), &envelope), result.Stderr)
	assert.Equal(t, ErrNoWork, envelope.Error.Code)
	assert.NotNil(t, envelope.Error.Data, "NO_WORK must explain which condition blocked the worker")
}

func TestInProcessHandoffPreservesSuccessorContext(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Work")
	mustRun(t, root, "claim", "1", "--as", "author")

	mustRun(t, root, "handoff", "1", "--as", "author", "--set", "finding=retain the last valid checksum")

	detail := mustRun(t, root, "cat", "1", "--json")
	var envelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(detail.Stdout), &envelope))
	assert.Equal(t, "retain the last valid checksum", envelope.Task.Metadata.Extra["finding"])
	assert.Empty(t, envelope.Task.Metadata.Owner, "handoff must yield the lease")
	// A handoff is a deliberate yield, not a failed attempt.
	assert.Zero(t, envelope.Task.Metadata.RetryCount)
}

func TestInProcessReleaseChargesARetry(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Work")
	mustRun(t, root, "claim", "1", "--as", "w1")
	mustRun(t, root, "release", "1", "--as", "w1", "--error", "flaky upstream")

	detail := mustRun(t, root, "cat", "1", "--json")
	var envelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(detail.Stdout), &envelope))
	assert.Equal(t, uint32(1), envelope.Task.Metadata.RetryCount)
}

func TestInProcessDecomposeYieldsTheParentAndCreatesChildren(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Fix the race")
	mustRun(t, root, "claim", "1", "--as", "planner")

	mustRun(t, root, "decompose", "1", "--as", "planner",
		`--into`, `{"subtasks":[{"title":"Reproduce","caps":["go"]},{"title":"Fix"}]}`)

	detail := mustRun(t, root, "cat", "1", "--json")
	var parent taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(detail.Stdout), &parent))
	assert.Len(t, parent.Task.Depends, 2)
	assert.Empty(t, parent.Task.Metadata.Owner)

	child := mustRun(t, root, "cat", "2", "--json")
	var childEnvelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(child.Stdout), &childEnvelope))
	assert.Equal(t, []string{"go"}, childEnvelope.Task.Metadata.Capabilities)
}

func TestInProcessBlockAndUnblockRoundTrip(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Work")
	mustRun(t, root, "claim", "1", "--as", "w1")

	mustRun(t, root, "block", "1", "--as", "w1", "--reason", "upstream API is down")
	blocked := mustRun(t, root, "cat", "1", "--json")
	var blockedEnvelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(blocked.Stdout), &blockedEnvelope))
	assert.Equal(t, "blocked", blockedEnvelope.Task.Status)
	assert.Empty(t, blockedEnvelope.Task.Metadata.Owner)

	mustRun(t, root, "unblock", "1")
	unblocked := mustRun(t, root, "cat", "1", "--json")
	var unblockedEnvelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(unblocked.Stdout), &unblockedEnvelope))
	assert.Equal(t, "pending", unblockedEnvelope.Task.Status)
}

func TestInProcessBootstrapSummarizesWithoutClaiming(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Objective")
	mustRun(t, root, "add", "Child", "--after", "1")

	result := mustRun(t, root, "bootstrap", "--as", "w1", "--json")
	assert.Contains(t, result.Stdout, "\"objective\"")

	// The brief must not take ownership of anything.
	detail := mustRun(t, root, "cat", "1", "--json")
	var envelope taskEnvelope
	require.NoError(t, json.Unmarshal([]byte(detail.Stdout), &envelope))
	assert.Empty(t, envelope.Task.Metadata.Owner)
}

func TestInProcessReadCommandsRenderWithoutMutating(t *testing.T) {
	root := newProject(t)
	mustRun(t, root, "add", "Prerequisite")
	mustRun(t, root, "add", "Dependent", "--after", "1", "--tag", "backend")

	before := readParityState(t, root)
	for _, args := range [][]string{
		{"status"}, {"status", "--json"}, {"status", "--all"},
		{"cat", "1"}, {"cat", "1", "--json"},
		{"next"}, {"next", "--json"},
		{"depends", "2"}, {"dependents", "1"},
		{"lineage", "1"}, {"graph"}, {"graph", "--dot"}, {"graph", "--json"},
		{"search", "Dependent"}, {"caps"}, {"my", "--as", "w1"},
		{"events"}, {"events", "--json"}, {"export"}, {"export", "--markdown"},
		{"what-if", "1", "--done"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			result := mustRun(t, root, args...)
			assert.NotEmpty(t, strings.TrimSpace(result.Stdout), "produced no output")
		})
	}
	assert.Equal(t, before, readParityState(t, root), "a read command mutated state")
}
