package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const mcpProtocolVersion = "2025-06-18"

type mcpServer struct {
	backend        *server
	encoder        *json.Encoder
	initializeSeen bool
	initialized    bool
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallResult struct {
	Content           []mcpContent `json:"content"`
	StructuredContent interface{}  `json:"structuredContent,omitempty"`
	IsError           bool         `json:"isError,omitempty"`
}

type mcpImplementation struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Title      string            `json:"title,omitempty"`
	WebsiteURL string            `json:"websiteUrl,omitempty"`
	Icons      []json.RawMessage `json:"icons,omitempty"`
}

func cmdMCP(args []string) {
	if !hasFlag(args, "--stdio") {
		fmt.Fprintln(os.Stderr, "Error: --stdio flag is required for mcp command")
		os.Exit(1)
	}

	projectInitialized := true
	if _, err := os.Stat(tasksBinPath()); err != nil {
		projectInitialized = false
	}

	encoder := json.NewEncoder(os.Stdout)
	srv := &mcpServer{
		backend: &server{
			initialized: projectInitialized,
			encoder:     encoder,
		},
		encoder: encoder,
	}
	if err := srv.readRequests(os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
}

func (srv *mcpServer) readRequests(input io.Reader) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if !json.Valid(line) {
			srv.writeError(nil, rpcParse, "Parse error", nil)
			continue
		}

		var req rpcRequest
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			srv.writeError(nil, rpcInvalidRequest, "Invalid Request", nil)
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			srv.writeError(req.ID, rpcInvalidRequest, "Invalid Request", nil)
			continue
		}

		result, rpcErr := srv.dispatch(req.Method, req.Params)
		if len(req.ID) == 0 {
			continue
		}
		if rpcErr != nil {
			srv.writeError(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
			continue
		}
		srv.writeResult(req.ID, result)
	}
	return scanner.Err()
}

func (srv *mcpServer) dispatch(method string, params json.RawMessage) (interface{}, *rpcError) {
	switch method {
	case "initialize":
		return srv.initialize(params)
	case "notifications/initialized":
		if !srv.initializeSeen {
			return nil, rpcErrorf(rpcNotInitialized, "initialize must be called first")
		}
		srv.initialized = true
		return map[string]interface{}{}, nil
	case "ping":
		return map[string]interface{}{}, nil
	case "tools/list":
		if !srv.initialized {
			return nil, rpcErrorf(rpcNotInitialized, "MCP client has not completed initialization")
		}
		return map[string]interface{}{"tools": terminalTodoMCPTools()}, nil
	case "tools/call":
		if !srv.initialized {
			return nil, rpcErrorf(rpcNotInitialized, "MCP client has not completed initialization")
		}
		return srv.callTool(params)
	default:
		return nil, rpcErrorf(rpcMethodNotFound, "Method not found: %s", method)
	}
}

func (srv *mcpServer) initialize(params json.RawMessage) (interface{}, *rpcError) {
	var p struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ClientInfo      mcpImplementation          `json:"clientInfo"`
		Meta            json.RawMessage            `json:"_meta,omitempty"`
	}
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ProtocolVersion == "" || p.ClientInfo.Name == "" || p.ClientInfo.Version == "" {
		return nil, rpcErrorf(rpcInvalidParams, "protocolVersion and clientInfo name/version are required")
	}
	srv.initializeSeen = true

	return map[string]interface{}{
		"protocolVersion": mcpProtocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": false},
		},
		"serverInfo": map[string]string{
			"name":    "terminal-todo",
			"title":   "terminal-todo",
			"version": version,
		},
		"instructions": "Use terminal_todo_acquire for atomic work allocation, heartbeat active leases, record findings with update/log, and complete or release every acquired task.",
	}, nil
}

func (srv *mcpServer) callTool(params json.RawMessage) (interface{}, *rpcError) {
	var p mcpCallParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, rpcErrorf(rpcInvalidParams, "tool name is required")
	}

	method, ok := mcpToolMethods()[p.Name]
	if !ok {
		return nil, rpcErrorf(rpcInvalidParams, "unknown tool: %s", p.Name)
	}
	arguments := p.Arguments
	if len(arguments) == 0 || string(arguments) == "null" {
		arguments = json.RawMessage(`{}`)
	}

	result, callErr := srv.backend.dispatch(method, arguments)
	if callErr != nil {
		detail := map[string]interface{}{
			"code":    callErr.Code,
			"message": callErr.Message,
		}
		if callErr.Data != nil {
			detail["data"] = callErr.Data
		}
		return newMCPCallResult(detail, true), nil
	}
	return newMCPCallResult(result, false), nil
}

func newMCPCallResult(value interface{}, isError bool) mcpCallResult {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(`{"code":-32603,"message":"could not encode tool result"}`)
		isError = true
		value = map[string]interface{}{"code": rpcInternal, "message": "could not encode tool result"}
	}
	return mcpCallResult{
		Content:           []mcpContent{{Type: "text", Text: string(encoded)}},
		StructuredContent: value,
		IsError:           isError,
	}
}

func (srv *mcpServer) writeResult(id json.RawMessage, result interface{}) {
	_ = srv.encoder.Encode(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (srv *mcpServer) writeError(id json.RawMessage, code int, message string, data interface{}) {
	_ = srv.encoder.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message, Data: data},
	})
}

func mcpToolMethods() map[string]string {
	return map[string]string{
		"terminal_todo_ping":      "todo.ping",
		"terminal_todo_init":      "todo.init",
		"terminal_todo_status":    "todo.status",
		"terminal_todo_cat":       "todo.cat",
		"terminal_todo_add":       "todo.add",
		"terminal_todo_acquire":   "todo.acquire",
		"terminal_todo_heartbeat": "todo.heartbeat",
		"terminal_todo_update":    "todo.update",
		"terminal_todo_log":       "todo.log",
		"terminal_todo_decompose": "todo.decompose",
		"terminal_todo_block":     "todo.block",
		"terminal_todo_release":   "todo.release",
		"terminal_todo_complete":  "todo.done",
		"terminal_todo_events":    "todo.events",
	}
}

func terminalTodoMCPTools() []mcpTool {
	object := func(properties map[string]interface{}, required ...string) map[string]interface{} {
		schema := map[string]interface{}{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": false,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	stringProp := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": description}
	}
	idProp := map[string]interface{}{"type": "integer", "minimum": 1, "description": "Positive task ID."}
	stringList := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}, "description": description}
	}

	return []mcpTool{
		{
			Name:        "terminal_todo_ping",
			Description: "Discover the terminal-todo protocol version, project root, initialization state, and coordination capabilities.",
			InputSchema: object(map[string]interface{}{}),
		},
		{
			Name:        "terminal_todo_init",
			Description: "Initialize terminal-todo's user-owned state in the current project. Safe to call when already initialized.",
			InputSchema: object(map[string]interface{}{}),
		},
		{
			Name:        "terminal_todo_status",
			Description: "Inspect the shared execution graph. Use before planning or resuming work to understand task state and ownership.",
			InputSchema: object(map[string]interface{}{
				"tag":   stringProp("Return only tasks with this tag."),
				"actor": stringProp("Return only tasks owned by this actor."),
				"all":   map[string]interface{}{"type": "boolean", "description": "Include linked repositories."},
			}),
		},
		{
			Name:        "terminal_todo_cat",
			Description: "Read one task's full state, dependencies, lease metadata, findings, and audit fields.",
			InputSchema: object(map[string]interface{}{"id": idProp}, "id"),
		},
		{
			Name:        "terminal_todo_add",
			Description: "Add durable work to the shared DAG, optionally with dependencies, priority, required capabilities, and tags.",
			InputSchema: object(map[string]interface{}{
				"title":        stringProp("Clear outcome-oriented task title."),
				"after":        stringList("Task IDs or todo:// dependency URIs that must complete first."),
				"priority":     map[string]interface{}{"type": "number", "description": "Higher values are allocated first."},
				"capabilities": stringList("Capabilities an actor must advertise to acquire this task."),
				"tags":         stringList("User-defined task tags."),
			}, "title"),
		},
		{
			Name:        "terminal_todo_acquire",
			Description: "Atomically select and lease one ready task. Always use this instead of separately listing and claiming work. Reuse requestId when retrying the same allocation.",
			InputSchema: object(map[string]interface{}{
				"actor":        stringProp("Stable identity for this worker or session."),
				"requestId":    stringProp("Unique idempotency key for this allocation attempt."),
				"ttl":          stringProp("Lease duration such as 30m or 2h."),
				"capabilities": stringList("Capabilities available to this worker."),
			}, "actor", "requestId"),
		},
		{
			Name:        "terminal_todo_heartbeat",
			Description: "Renew an active lease before it expires. Use periodically during long-running work.",
			InputSchema: object(map[string]interface{}{
				"id":    idProp,
				"actor": stringProp("Current lease owner."),
				"ttl":   stringProp("New lease duration such as 30m or 2h."),
			}, "id", "actor"),
		},
		{
			Name:        "terminal_todo_update",
			Description: "Update owned task metadata, dependencies, or structured findings. Use extra for durable handoff facts such as tests, commit, files, or decisions.",
			InputSchema: object(map[string]interface{}{
				"id":           idProp,
				"title":        stringProp("Replacement task title."),
				"priority":     map[string]interface{}{"type": "number"},
				"capabilities": stringList("Replacement required capabilities."),
				"actor":        stringProp("Actor making the update; required when another actor owns the task."),
				"extra":        map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "string"}, "description": "Structured durable handoff fields."},
				"addDeps":      stringList("Dependencies to add."),
				"removeDeps":   stringList("Dependencies to remove."),
			}, "id"),
		},
		{
			Name:        "terminal_todo_log",
			Description: "Append an immutable human-readable progress note or finding to a task's audit trail.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"message": stringProp("Concise progress, decision, risk, or handoff note."),
				"actor":   stringProp("Actor recording the note."),
			}, "id", "message"),
		},
		{
			Name:        "terminal_todo_decompose",
			Description: "Split a broad task into child tasks. The parent becomes pending on the children and any active parent lease is safely released.",
			InputSchema: object(map[string]interface{}{
				"id":    idProp,
				"actor": stringProp("Current parent lease owner when claimed."),
				"subtasks": map[string]interface{}{
					"type":        "array",
					"minItems":    1,
					"description": "Child work items.",
					"items": object(map[string]interface{}{
						"title":        stringProp("Outcome-oriented child title."),
						"capabilities": stringList("Capabilities required for this child."),
					}, "title"),
				},
			}, "id", "subtasks"),
		},
		{
			Name:        "terminal_todo_block",
			Description: "Mark work explicitly blocked and preserve the reason for coordinators and future sessions.",
			InputSchema: object(map[string]interface{}{
				"id":     idProp,
				"reason": stringProp("Concrete blocking condition and what would unblock it."),
				"actor":  stringProp("Actor reporting the blocker."),
			}, "id", "reason"),
		},
		{
			Name:        "terminal_todo_release",
			Description: "Yield an owned lease back to the ready pool, optionally recording a failed-attempt error for retries and recovery.",
			InputSchema: object(map[string]interface{}{
				"id":    idProp,
				"actor": stringProp("Current lease owner."),
				"error": stringProp("Failure summary when releasing after an unsuccessful attempt."),
			}, "id", "actor"),
		},
		{
			Name:        "terminal_todo_complete",
			Description: "Complete one or more tasks after verifying their outcome and dependencies. Claimed tasks require the owning actor.",
			InputSchema: object(map[string]interface{}{
				"ids":   map[string]interface{}{"type": "array", "minItems": 1, "items": map[string]interface{}{"type": "integer", "minimum": 1}},
				"actor": stringProp("Current lease owner for claimed tasks."),
			}, "ids"),
		},
		{
			Name:        "terminal_todo_events",
			Description: "Read the append-only coordination event stream for audit, recovery, monitoring, or incremental synchronization.",
			InputSchema: object(map[string]interface{}{
				"since": map[string]interface{}{"type": "integer", "minimum": 0, "description": "Return events after this event sequence."},
			}),
		},
	}
}
