package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/require"
)

// Protocol decoding is the boundary where a coordination request stops being
// the host's data and becomes this process's. Every JSON-RPC method and every
// MCP tool call arrives through it, from agent runtimes this project does not
// control.
//
// The property under test is narrow and absolute: a malformed, hostile, or
// merely surprising request must produce an error, never a panic and never a
// partial mutation. A panic on the stdio server takes down a worker's whole
// coordination channel, and a partial mutation corrupts state that other
// workers are reading.

// fuzzProject builds a disposable store for one fuzz iteration.
func fuzzProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".terminal-todo"), 0o700))

	previous := projectRoot
	projectRoot = root
	defer func() { projectRoot = previous }()

	s := store.NewTaskStore()
	s.AddTask("Prerequisite", nil)
	s.AddTask("Dependent", []string{"todo://local/1"})
	require.NoError(t, s.Save(tasksBinPath()))
	return root
}

// dispatchMethods is every JSON-RPC method the server accepts.
func dispatchMethods() []string {
	methods := make([]string, 0, len(mcpToolMethods()))
	seen := map[string]bool{}
	for _, method := range mcpToolMethods() {
		if !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	// Methods that are reachable natively but deliberately absent from MCP.
	for _, method := range []string{
		"todo.claim", "todo.unblock", "todo.next", "todo.done", "todo.search",
		"todo.depends", "todo.dependents", "todo.lineage", "todo.whatIf",
		"todo.graph", "todo.config.get", "todo.config.set", "todo.prune",
		"todo.compact", "todo.export", "todo.link", "todo.unlink",
		"todo.backup", "todo.restore", "todo.doctor", "todo.agentCard",
		"todo.caps", "todo.my",
	} {
		if !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	return methods
}

func FuzzJSONRPCDispatch(f *testing.F) {
	for _, params := range []string{
		`{}`, `null`, `[]`, `""`, `0`, `true`,
		`{"id":1,"actor":"w1"}`,
		`{"id":0,"actor":""}`,
		`{"id":-1}`,
		`{"id":18446744073709551616}`,
		`{"ids":[1,1,1]}`,
		`{"ids":null}`,
		`{"title":""}`,
		`{"title":"x","after":["todo://local/01"]}`,
		`{"id":1,"actor":"w1","ttl":"-1s"}`,
		`{"id":1,"actor":"w1","ttl":"not-a-duration"}`,
		`{"id":1,"actor":"   "}`,
		`{"id":1,"actor":"a\u0000b"}`,
		`{"id":1,"extra":{"":""}}`,
		`{"id":1,"addDeps":["todo://../../etc/passwd/1"]}`,
		`{"id":1,"subtasks":[]}`,
		`{"actor":"w1","requestId":""}`,
		`{"priority":1e400}`,
		`{"unknown_field":true}`,
	} {
		// Seed every dispatchable method, so the corpus covers the whole
		// protocol surface rather than the handful that are easiest to reach.
		for _, method := range dispatchMethods() {
			f.Add(method, params)
		}
	}

	f.Fuzz(func(t *testing.T, method, params string) {
		if !json.Valid([]byte(params)) {
			// Transport-level JSON framing is the reader's concern, not the
			// dispatcher's; feeding it invalid JSON tests nothing here.
			return
		}

		root := fuzzProject(t)
		previousRoot := projectRoot
		projectRoot = root
		defer func() { projectRoot = previousRoot }()

		srv := &server{initialized: true}
		// A panic here fails the test by escaping; that is the property.
		result, rpcErr := srv.dispatch(method, json.RawMessage(params))

		if rpcErr != nil {
			if result != nil {
				t.Fatalf("%s returned both a result and an error", method)
			}
			// Every reported failure must carry a code a client can branch on.
			if rpcErr.Code == 0 {
				t.Fatalf("%s reported an error with no code: %q", method, rpcErr.Message)
			}
			return
		}

		// A successful result must be encodable, or the server cannot answer.
		if _, err := json.Marshal(result); err != nil {
			t.Fatalf("%s produced an unencodable result: %v", method, err)
		}

		// The store must still load. A decoder that admitted a value the store
		// cannot round-trip would corrupt coordination state for every worker.
		if _, err := store.LoadCurrent(filepath.Join(root, ".terminal-todo", "tasks.bin")); err != nil {
			t.Fatalf("%s left an unreadable store: %v", method, err)
		}
	})
}

func FuzzMCPToolCall(f *testing.F) {
	for _, call := range []string{
		`{"name":"terminal_todo_add","arguments":{"title":"Work"}}`,
		`{"name":"terminal_todo_acquire","arguments":{"actor":"w1","requestId":"r1"}}`,
		`{"name":"terminal_todo_complete","arguments":{"ids":[1]}}`,
		`{"name":"","arguments":{}}`,
		`{"name":"nonexistent_tool","arguments":{}}`,
		`{"name":"terminal_todo_add"}`,
		`{"name":"terminal_todo_add","arguments":null}`,
		`{"name":"terminal_todo_add","arguments":[]}`,
		`{"name":"terminal_todo_update","arguments":{"id":1,"extra":{"k":"v"}}}`,
		`{}`,
		`null`,
	} {
		f.Add(call)
	}

	f.Fuzz(func(t *testing.T, call string) {
		if !json.Valid([]byte(call)) {
			return
		}

		root := fuzzProject(t)
		previousRoot := projectRoot
		projectRoot = root
		defer func() { projectRoot = previousRoot }()

		srv := &mcpServer{backend: &server{initialized: true}, initializeSeen: true, initialized: true}
		result, rpcErr := srv.dispatch("tools/call", json.RawMessage(call))

		if rpcErr != nil {
			if rpcErr.Code == 0 {
				t.Fatalf("tool call reported an error with no code: %q", rpcErr.Message)
			}
			return
		}

		callResult, ok := result.(mcpCallResult)
		if !ok {
			t.Fatalf("tools/call returned %T rather than an MCP result", result)
		}
		encoded, err := json.Marshal(callResult)
		if err != nil {
			t.Fatalf("unencodable MCP result: %v", err)
		}
		// The visible text block is bounded by the coordination noise budget.
		// A tool that could exceed it would push protocol payloads into the
		// user's conversation.
		for _, content := range callResult.Content {
			if len(content.Text) > maxMCPResultSummaryLength {
				t.Fatalf("MCP text content is %d bytes, over the %d-byte budget: %q",
					len(content.Text), maxMCPResultSummaryLength, content.Text)
			}
		}
		_ = encoded

		if _, err := store.LoadCurrent(filepath.Join(root, ".terminal-todo", "tasks.bin")); err != nil {
			t.Fatalf("tool call left an unreadable store: %v", err)
		}
	})
}
