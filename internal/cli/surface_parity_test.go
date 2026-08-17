package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cross-surface parity.
//
// The same coordination operation is implemented once for the CLI and again
// for JSON-RPC, and MCP is an adapter over the JSON-RPC dispatch table. Until
// those implementations share one core, nothing but a test forces them to
// agree. This file is that test.
//
// Each case describes one failure condition, runs it through all three
// surfaces against identical fixtures, and asserts that every surface reports
// the same protocol error identifier and leaves the store unchanged. The CLI
// case additionally asserts the documented exit status from
// docs/agent-protocol.md, which is the contract that regressed and that this
// harness exists to hold.

// parityFixture builds an identical starting graph on every surface.
type parityFixture func(t *testing.T, run func(args ...string))

// parityCase is one failure condition expressed for all three surfaces.
type parityCase struct {
	name string
	// fixture prepares the graph using CLI commands that must succeed.
	fixture parityFixture
	// cli is the failing command, without --json.
	cli []string
	// method and params are the JSON-RPC equivalent of cli.
	method string
	params string
	// mcpTool is the MCP tool name for the same operation. Empty means the
	// operation is not exposed over MCP.
	mcpTool string
	// wantCode is the documented protocol error identifier.
	wantCode ErrorCode
}

func parityCases() []parityCase {
	twoTasksOneClaimed := func(t *testing.T, run func(args ...string)) {
		run("add", "First")
		run("add", "Second")
		run("claim", "1", "--as", "w1")
	}
	blockedChain := func(t *testing.T, run func(args ...string)) {
		run("add", "Prerequisite")
		run("add", "Dependent", "--after", "1")
	}

	return []parityCase{
		{
			name:     "claim_missing_task",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"claim", "99", "--as", "w2"},
			method:   "todo.claim",
			params:   `{"id":99,"actor":"w2"}`,
			wantCode: ErrTaskNotFound,
		},
		{
			name:     "claim_task_held_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"claim", "1", "--as", "w2"},
			method:   "todo.claim",
			params:   `{"id":1,"actor":"w2"}`,
			wantCode: ErrAlreadyClaimed,
		},
		{
			name: "claim_completed_task",
			fixture: func(t *testing.T, run func(args ...string)) {
				run("add", "First")
				run("done", "1")
			},
			cli:      []string{"claim", "1", "--as", "w2"},
			method:   "todo.claim",
			params:   `{"id":1,"actor":"w2"}`,
			wantCode: ErrInvalidTransition,
		},
		{
			name:     "claim_task_with_incomplete_dependencies",
			fixture:  blockedChain,
			cli:      []string{"claim", "2", "--as", "w1"},
			method:   "todo.claim",
			params:   `{"id":2,"actor":"w1"}`,
			wantCode: ErrDependency,
		},
		{
			name:     "complete_missing_task",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"done", "99"},
			method:   "todo.done",
			params:   `{"ids":[99]}`,
			mcpTool:  "terminal_todo_complete",
			wantCode: ErrTaskNotFound,
		},
		{
			name:     "complete_task_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"done", "1", "--as", "w2"},
			method:   "todo.done",
			params:   `{"ids":[1],"actor":"w2"}`,
			mcpTool:  "terminal_todo_complete",
			wantCode: ErrNotOwner,
		},
		{
			name:     "complete_task_with_incomplete_dependencies",
			fixture:  blockedChain,
			cli:      []string{"done", "2"},
			method:   "todo.done",
			params:   `{"ids":[2]}`,
			mcpTool:  "terminal_todo_complete",
			wantCode: ErrDependency,
		},
		{
			name:     "release_task_that_holds_no_lease",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"release", "2", "--as", "w1"},
			method:   "todo.release",
			params:   `{"id":2,"actor":"w1"}`,
			mcpTool:  "terminal_todo_release",
			wantCode: ErrLeaseNotActive,
		},
		{
			name:     "release_task_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"release", "1", "--as", "w2"},
			method:   "todo.release",
			params:   `{"id":1,"actor":"w2"}`,
			mcpTool:  "terminal_todo_release",
			wantCode: ErrNotOwner,
		},
		{
			name:     "heartbeat_task_that_holds_no_lease",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"heartbeat", "2", "--as", "w1"},
			method:   "todo.heartbeat",
			params:   `{"id":2,"actor":"w1"}`,
			mcpTool:  "terminal_todo_heartbeat",
			wantCode: ErrLeaseNotActive,
		},
		{
			name:     "heartbeat_lease_owned_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"heartbeat", "1", "--as", "w2"},
			method:   "todo.heartbeat",
			params:   `{"id":1,"actor":"w2"}`,
			mcpTool:  "terminal_todo_heartbeat",
			wantCode: ErrNotOwner,
		},
		{
			name:     "block_task_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"block", "1", "--as", "w2", "--reason", "waiting on review"},
			method:   "todo.block",
			params:   `{"id":1,"actor":"w2","reason":"waiting on review"}`,
			mcpTool:  "terminal_todo_block",
			wantCode: ErrNotOwner,
		},
		{
			name:     "unblock_task_that_is_not_blocked",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"unblock", "2"},
			method:   "todo.unblock",
			params:   `{"id":2}`,
			wantCode: ErrInvalidTransition,
		},
		{
			name:     "update_task_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"update", "1", "--as", "w2", "--title", "Renamed"},
			method:   "todo.update",
			params:   `{"id":1,"actor":"w2","title":"Renamed"}`,
			mcpTool:  "terminal_todo_update",
			wantCode: ErrNotOwner,
		},
		{
			name:     "update_with_unknown_dependency",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"update", "2", "--add-dep", "99"},
			method:   "todo.update",
			params:   `{"id":2,"addDeps":["99"]}`,
			mcpTool:  "terminal_todo_update",
			wantCode: ErrTaskNotFound,
		},
		{
			name:     "log_against_task_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"log", "1", "--as", "w2", "--msg", "note"},
			method:   "todo.log",
			params:   `{"id":1,"actor":"w2","message":"note"}`,
			mcpTool:  "terminal_todo_log",
			wantCode: ErrNotOwner,
		},
		{
			name:     "decompose_parent_leased_by_another_agent",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"decompose", "1", "--as", "w2", "--into", `{"subtasks":[{"title":"Child"}]}`},
			method:   "todo.decompose",
			params:   `{"id":1,"actor":"w2","subtasks":[{"title":"Child"}]}`,
			mcpTool:  "terminal_todo_decompose",
			wantCode: ErrNotOwner,
		},
		{
			name:     "handoff_task_that_holds_no_lease",
			fixture:  twoTasksOneClaimed,
			cli:      []string{"handoff", "2", "--as", "w1", "--set", "finding=checksum must survive"},
			method:   "todo.handoff",
			params:   `{"id":2,"actor":"w1","extra":{"finding":"checksum must survive"}}`,
			mcpTool:  "terminal_todo_handoff",
			wantCode: ErrLeaseNotActive,
		},
	}
}

func TestSurfaceParityForLifecycleFailures(t *testing.T) {
	todo := buildTodo(t)

	for _, tc := range parityCases() {
		t.Run(tc.name, func(t *testing.T) {
			cliCode, cliExit, cliState := runParityCLI(t, todo, tc)
			assert.Equal(t, tc.wantCode, cliCode, "CLI error code")
			assert.Equal(t, exitCode(tc.wantCode), cliExit,
				"CLI exit status for %s must match docs/agent-protocol.md", tc.wantCode)

			rpcCode, rpcState := runParityRPC(t, tc, false)
			assert.Equal(t, rpcCodeForErrorCode(tc.wantCode), rpcCode, "JSON-RPC error code")
			assert.Equal(t, cliState, rpcState, "JSON-RPC left different persisted state than the CLI")

			if tc.mcpTool == "" {
				return
			}
			mcpCode, mcpState := runParityRPC(t, tc, true)
			assert.Equal(t, rpcCodeForErrorCode(tc.wantCode), mcpCode, "MCP error code")
			assert.Equal(t, cliState, mcpState, "MCP left different persisted state than the CLI")
		})
	}
}

// runParityCLI executes the failing command through a real process and returns
// the reported error code, the process exit status, and the resulting state.
func runParityCLI(t *testing.T, todo string, tc parityCase) (ErrorCode, int, parityState) {
	t.Helper()
	root := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(todo, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "fixture %v: %s", args, out)
	}
	run("init")
	tc.fixture(t, run)

	cmd := exec.Command(todo, append(append([]string{}, tc.cli...), "--json")...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "expected %v to fail: %s", tc.cli, output)

	var envelope errorEnvelope
	require.NoError(t, json.Unmarshal(output, &envelope), "CLI error output: %s", output)

	var exit int
	if exitErr, ok := err.(*exec.ExitError); ok {
		exit = exitErr.ExitCode()
	}
	return envelope.Error.Code, exit, readParityState(t, root)
}

// runParityRPC executes the same operation in process, through either the
// JSON-RPC dispatch table or the MCP adapter over it.
func runParityRPC(t *testing.T, tc parityCase, throughMCP bool) (int, parityState) {
	t.Helper()
	root := t.TempDir()

	previousRoot := projectRoot
	projectRoot = root
	defer func() { projectRoot = previousRoot }()

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".terminal-todo"), 0o700))
	require.NoError(t, store.NewTaskStore().Save(tasksBinPath()))

	backend := &server{initialized: true}
	fixtureViaRPC(t, backend, tc)

	var rpcErr *rpcError
	if throughMCP {
		srv := &mcpServer{backend: backend, initializeSeen: true, initialized: true}
		call, err := json.Marshal(map[string]interface{}{
			"name":      tc.mcpTool,
			"arguments": json.RawMessage(tc.params),
		})
		require.NoError(t, err)
		result, dispatchErr := srv.dispatch("tools/call", call)
		if dispatchErr != nil {
			rpcErr = dispatchErr
		} else {
			// A tool that fails inside the call reports the error in the
			// result rather than as a protocol-level error.
			callResult, ok := result.(mcpCallResult)
			require.True(t, ok, "unexpected MCP result type %T", result)
			require.True(t, callResult.IsError, "expected %s to fail", tc.mcpTool)
			rpcErr = mcpCallResultError(t, callResult)
		}
	} else {
		_, rpcErr = backend.dispatch(tc.method, json.RawMessage(tc.params))
	}
	require.NotNil(t, rpcErr, "expected %s to fail", tc.method)
	return rpcErr.Code, readParityState(t, root)
}

// fixtureViaRPC replays the CLI fixture through JSON-RPC so both surfaces
// start from the same graph.
func fixtureViaRPC(t *testing.T, backend *server, tc parityCase) {
	t.Helper()
	tc.fixture(t, func(args ...string) {
		t.Helper()
		method, params := rpcEquivalent(t, args)
		_, rpcErr := backend.dispatch(method, json.RawMessage(params))
		require.Nil(t, rpcErr, "fixture %v: %+v", args, rpcErr)
	})
}

// rpcEquivalent translates the small set of fixture commands into JSON-RPC.
// Keeping it deliberately narrow means an unsupported fixture fails loudly
// instead of silently diverging from the CLI arrangement.
func rpcEquivalent(t *testing.T, args []string) (string, string) {
	t.Helper()
	switch {
	case len(args) == 1 && args[0] == "init":
		return "todo.init", `{}`
	case len(args) == 2 && args[0] == "add":
		return "todo.add", fmt.Sprintf(`{"title":%q}`, args[1])
	case len(args) == 4 && args[0] == "add" && args[2] == "--after":
		return "todo.add", fmt.Sprintf(`{"title":%q,"after":[%q]}`, args[1], args[3])
	case len(args) == 2 && args[0] == "done":
		return "todo.done", fmt.Sprintf(`{"ids":[%s]}`, args[1])
	case len(args) == 4 && args[0] == "claim" && args[2] == "--as":
		return "todo.claim", fmt.Sprintf(`{"id":%s,"actor":%q}`, args[1], args[3])
	default:
		t.Fatalf("fixture command has no JSON-RPC equivalent: %v", args)
		return "", ""
	}
}

// mcpCallResultError recovers the protocol error an MCP tool reported inside a
// successful call envelope.
func mcpCallResultError(t *testing.T, result mcpCallResult) *rpcError {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var decoded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded), "MCP error content: %s", encoded)
	return &rpcError{Code: decoded.Code, Message: decoded.Message}
}

// parityState is the observable outcome a failed operation must not change:
// task lifecycle fields and the number of recorded events.
type parityState struct {
	Tasks  map[uint64]parityTask
	Events int
}

type parityTask struct {
	Status      string
	Owner       string
	Leased      bool
	RetryCount  uint32
	BlockReason string
	Depends     int
}

func readParityState(t *testing.T, root string) parityState {
	t.Helper()
	s, err := store.LoadCurrent(filepath.Join(root, ".terminal-todo", "tasks.bin"))
	require.NoError(t, err)

	state := parityState{Tasks: make(map[uint64]parityTask, len(s.Tasks)), Events: len(s.Events)}
	for id, task := range s.Tasks {
		state.Tasks[id] = parityTask{
			Status:      string(task.Status),
			Owner:       task.Owner,
			Leased:      task.LeaseExpires > 0,
			RetryCount:  task.RetryCount,
			BlockReason: task.BlockReason,
			Depends:     len(task.Depends),
		}
	}
	return state
}

// TestMCPOmitsRaceProneAllocation pins a deliberate design decision rather
// than an oversight. The MCP surface exposes atomic acquire and omits claim,
// because selecting with next and then claiming separately is exactly the race
// the allocator exists to prevent. unblock is likewise a coordinator action
// rather than a worker one. The parity harness skips MCP for both operations,
// so the omission needs its own assertion.
func TestMCPOmitsRaceProneAllocation(t *testing.T) {
	tools := mcpToolMethods()
	for _, absent := range []string{"terminal_todo_claim", "terminal_todo_next", "terminal_todo_unblock"} {
		_, present := tools[absent]
		assert.False(t, present, "%s must stay off the MCP surface", absent)
	}
	assert.Equal(t, "todo.acquire", tools["terminal_todo_acquire"],
		"atomic acquisition is the supported MCP allocation primitive")
}
