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
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Annotations mcpToolAnnotations     `json:"annotations"`
}

type mcpToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
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
		"instructions": "Treat routine coordination as background bookkeeping. Use terminal_todo_acquire for atomic work allocation, heartbeat active leases, record findings with update/log, and complete or release every acquired task. Tell the user about meaningful outcomes and blockers, not every coordination call.",
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
	recordConformanceToolCall(method, arguments, result, callErr)
	if callErr != nil {
		detail := map[string]interface{}{
			"code":    callErr.Code,
			"message": callErr.Message,
		}
		if callErr.Data != nil {
			detail["data"] = callErr.Data
		}
		return newMCPCallResult(p.Name, detail, true), nil
	}
	return newMCPCallResult(p.Name, result, false), nil
}

func newMCPCallResult(toolName string, value interface{}, isError bool) mcpCallResult {
	if _, err := json.Marshal(value); err != nil {
		isError = true
		value = map[string]interface{}{"code": rpcInternal, "message": "could not encode tool result"}
	}
	return mcpCallResult{
		Content:           []mcpContent{{Type: "text", Text: mcpResultSummary(toolName, value, isError)}},
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
		"terminal_todo_bootstrap": "todo.bootstrap",
		"terminal_todo_status":    "todo.status",
		"terminal_todo_cat":       "todo.cat",
		"terminal_todo_add":       "todo.add",
		"terminal_todo_acquire":   "todo.acquire",
		"terminal_todo_heartbeat": "todo.heartbeat",
		"terminal_todo_handoff":   "todo.handoff",
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
	boundedStringProp := func(description string, maxLength int) map[string]interface{} {
		return map[string]interface{}{"type": "string", "maxLength": maxLength, "description": description}
	}
	idProp := map[string]interface{}{"type": "integer", "minimum": 1, "description": "Positive task ID."}
	stringList := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}, "description": description}
	}
	boundedStringList := func(description string, maxItems, maxLength int) map[string]interface{} {
		return map[string]interface{}{
			"type":        "array",
			"maxItems":    maxItems,
			"items":       map[string]interface{}{"type": "string", "maxLength": maxLength},
			"description": description,
		}
	}
	receiptProp := map[string]interface{}{
		"type":        "boolean",
		"description": "Return a bounded versioned mutation receipt. Follow detail_follow_up when full detail is needed.",
	}

	return []mcpTool{
		{
			Name:        "terminal_todo_ping",
			Title:       "Check terminal-todo",
			Description: "Discover the terminal-todo protocol version, project root, initialization state, and coordination capabilities.",
			InputSchema: object(map[string]interface{}{}),
			Annotations: mcpToolAnnotations{ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_init",
			Title:       "Initialize terminal-todo",
			Description: "Initialize terminal-todo's user-owned state in the current project. Safe to call when already initialized.",
			InputSchema: object(map[string]interface{}{}),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_bootstrap",
			Title:       "Get worker session brief",
			Description: "Summarize a caller-supplied worker identity with one bounded brief: objective progress, owned work, compatible ready work, blockers, capability demand, and recent events. Prefer this over dumping status and event history.",
			InputSchema: object(map[string]interface{}{
				"actor":        stringProp("Stable identity for this worker or session."),
				"capabilities": stringList("Capabilities available to this worker. Omit to use its registered agent card."),
				"objectiveId":  map[string]interface{}{"type": "integer", "minimum": 1, "description": "Optional objective task whose local dependency closure defines progress."},
				"limit":        map[string]interface{}{"type": "integer", "minimum": 1, "maximum": maxBootstrapLimit, "description": "Maximum entries per work, blocker, and capability section; defaults to 5."},
				"eventLimit":   map[string]interface{}{"type": "integer", "minimum": 1, "maximum": maxBootstrapLimit, "description": "Maximum recent events; defaults to 5."},
			}, "actor"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_status",
			Title:       "Inspect task graph",
			Description: "Inspect the shared execution graph. Use before planning or resuming work to understand task state and ownership.",
			InputSchema: object(map[string]interface{}{
				"tag":   stringProp("Return only tasks with this tag."),
				"actor": stringProp("Return only tasks owned by this actor."),
				"all":   map[string]interface{}{"type": "boolean", "description": "Include linked repositories."},
			}),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_cat",
			Title:       "Read task details",
			Description: "Read one task's full state, dependencies, lease metadata, findings, and audit fields.",
			InputSchema: object(map[string]interface{}{"id": idProp}, "id"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_add",
			Title:       "Add task",
			Description: "Add durable work to the shared DAG, optionally with dependencies, priority, required capabilities, and tags.",
			InputSchema: object(map[string]interface{}{
				"title":        boundedStringProp("Clear outcome-oriented task title, at most 1024 UTF-8 bytes.", maxTaskTitleBytes),
				"after":        boundedStringList("Task IDs or todo:// dependency URIs that must complete first.", maxTaskDependencies, maxDependencyBytes),
				"priority":     map[string]interface{}{"type": "number", "description": "Higher values are allocated first."},
				"capabilities": boundedStringList("Capabilities an actor must advertise to acquire this task.", maxTaskCapabilities, maxCapabilityBytes),
				"tags":         boundedStringList("User-defined task tags.", maxTaskTags, maxTagBytes),
				"receipt":      receiptProp,
			}, "title"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_acquire",
			Title:       "Acquire ready work",
			Description: "Atomically select and lease one ready task. Always use this instead of separately listing and claiming work. Reuse requestId when retrying the same allocation.",
			InputSchema: object(map[string]interface{}{
				"actor":        boundedStringProp("Stable identity for this worker or session.", maxActorBytes),
				"requestId":    boundedStringProp("Unique idempotency key for this allocation attempt.", 128),
				"ttl":          stringProp("Lease duration such as 30m or 2h."),
				"capabilities": boundedStringList("Capabilities available to this worker.", maxTaskCapabilities, maxCapabilityBytes),
				"receipt":      receiptProp,
			}, "actor", "requestId"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_heartbeat",
			Title:       "Renew task lease",
			Description: "Renew an active lease before it expires. Use periodically during long-running work.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"actor":   boundedStringProp("Current lease owner.", maxActorBytes),
				"ttl":     stringProp("New lease duration such as 30m or 2h."),
				"receipt": receiptProp,
			}, "id", "actor"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_update",
			Title:       "Update task",
			Description: "Update owned task metadata, dependencies, or structured successor context. Use extra for durable handoffs; prefer canonical keys finding, decision, tests, commit, files, and blocker.",
			InputSchema: object(map[string]interface{}{
				"id":           idProp,
				"title":        boundedStringProp("Replacement task title.", maxTaskTitleBytes),
				"priority":     map[string]interface{}{"type": "number"},
				"capabilities": boundedStringList("Replacement required capabilities.", maxTaskCapabilities, maxCapabilityBytes),
				"actor":        boundedStringProp("Actor making the update; required when another actor owns the task.", maxActorBytes),
				"extra":        map[string]interface{}{"type": "object", "maxProperties": maxTaskExtraEntries, "additionalProperties": map[string]interface{}{"type": "string", "maxLength": maxMetadataValueBytes}, "description": "Structured durable handoff fields. Prefer canonical keys finding, decision, tests, commit, files, and blocker. Keys are at most 128 UTF-8 bytes and values at most 16384."},
				"addDeps":      boundedStringList("Dependencies to add.", maxTaskDependencies, maxDependencyBytes),
				"removeDeps":   boundedStringList("Dependencies to remove.", maxTaskDependencies, maxDependencyBytes),
				"receipt":      receiptProp,
			}, "id"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_handoff",
			Title:       "Hand off task",
			Description: "Atomically persist structured successor context and yield a valid owned lease. Prefer this over separate update and release calls when another worker will continue the task.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"actor":   boundedStringProp("Current active lease owner.", maxActorBytes),
				"extra":   map[string]interface{}{"type": "object", "minProperties": 1, "maxProperties": maxTaskExtraEntries, "additionalProperties": map[string]interface{}{"type": "string", "maxLength": maxMetadataValueBytes}, "description": "Structured durable handoff fields. Prefer canonical keys finding, decision, tests, commit, files, and blocker."},
				"receipt": receiptProp,
			}, "id", "actor", "extra"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_log",
			Title:       "Record task note",
			Description: "Append an immutable chronological audit note. Use terminal_todo_update extra, not a log alone, for context a successor must recover by structured lookup.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"message": boundedStringProp("Concise chronological progress, decision, or risk note; not a substitute for structured handoff fields.", maxLogMessageBytes),
				"actor":   boundedStringProp("Actor recording the note.", maxActorBytes),
				"receipt": receiptProp,
			}, "id", "message"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_decompose",
			Title:       "Decompose task",
			Description: "Split a broad task into child tasks. The parent becomes pending on the children and any active parent lease is safely released.",
			InputSchema: object(map[string]interface{}{
				"id":    idProp,
				"actor": boundedStringProp("Current parent lease owner when claimed.", maxActorBytes),
				"subtasks": map[string]interface{}{
					"type":        "array",
					"minItems":    1,
					"maxItems":    maxMutationReceiptIDs,
					"description": "Child work items.",
					"items": object(map[string]interface{}{
						"title":        boundedStringProp("Outcome-oriented child title.", maxTaskTitleBytes),
						"capabilities": boundedStringList("Capabilities required for this child.", maxTaskCapabilities, maxCapabilityBytes),
					}, "title"),
				},
				"receipt": receiptProp,
			}, "id", "subtasks"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_block",
			Title:       "Block task",
			Description: "Mark work explicitly blocked and preserve the reason for coordinators and future sessions.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"reason":  boundedStringProp("Concrete blocking condition and what would unblock it.", maxReasonBytes),
				"actor":   boundedStringProp("Actor reporting the blocker.", maxActorBytes),
				"receipt": receiptProp,
			}, "id", "reason"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_release",
			Title:       "Release task lease",
			Description: "Yield an owned lease back to the ready pool, optionally recording a failed-attempt error for retries and recovery.",
			InputSchema: object(map[string]interface{}{
				"id":      idProp,
				"actor":   boundedStringProp("Current lease owner.", maxActorBytes),
				"error":   boundedStringProp("Failure summary when releasing after an unsuccessful attempt.", maxErrorBytes),
				"receipt": receiptProp,
			}, "id", "actor"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_complete",
			Title:       "Complete tasks",
			Description: "Complete one or more tasks after verifying their outcome and dependencies. Claimed tasks require the owning actor.",
			InputSchema: object(map[string]interface{}{
				"ids":     map[string]interface{}{"type": "array", "minItems": 1, "items": map[string]interface{}{"type": "integer", "minimum": 1}},
				"actor":   boundedStringProp("Current lease owner for claimed tasks.", maxActorBytes),
				"receipt": receiptProp,
			}, "ids"),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: false, OpenWorldHint: false},
		},
		{
			Name:        "terminal_todo_events",
			Title:       "Read coordination events",
			Description: "Read coordination events. Set page=true for a bounded, versioned page, continue from cursor.next_since, and resynchronize current status when cursor.history_gap is true.",
			InputSchema: object(map[string]interface{}{
				"since": map[string]interface{}{"type": "integer", "minimum": 0, "description": "Return events after this event sequence."},
				"limit": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": maxEventPageLimit, "description": "Maximum events to return; defaults to 100."},
				"page":  map[string]interface{}{"type": "boolean", "description": "Return the bounded, versioned page envelope. Recommended for agent callers."},
			}),
			Annotations: mcpToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
		},
	}
}
