package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"terminal-todo/dag"
	"terminal-todo/store"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	rpcParse               = -32700
	rpcInvalidRequest      = -32600
	rpcMethodNotFound      = -32601
	rpcInvalidParams       = -32602
	rpcInternal            = -32603
	rpcTaskNotFound        = -32001
	rpcNotInitialized      = -32002
	rpcCycleDetected       = -32003
	rpcAlreadyClaimed      = -32004
	rpcNotOwner            = -32005
	rpcDependency          = -32006
	rpcStoreCorrupted      = -32007
	rpcLockContention      = -32008
	rpcSchemaVersion       = -32009
	rpcNoWork              = -32010
	rpcAgentCapacity       = -32011
	rpcIdempotencyConflict = -32012
)

type server struct {
	initialized bool
	encoder     *json.Encoder
}

type addParams struct {
	Title        string   `json:"title"`
	After        []string `json:"after,omitempty"`
	Priority     *float32 `json:"priority,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type doneParams struct {
	IDs   []uint64 `json:"ids"`
	Actor string   `json:"actor,omitempty"`
}

type statusParams struct {
	Tag   string `json:"tag,omitempty"`
	Actor string `json:"actor,omitempty"`
	All   bool   `json:"all,omitempty"`
}

type catParams struct {
	ID uint64 `json:"id"`
}

type updateParams struct {
	ID           uint64            `json:"id"`
	Title        *string           `json:"title,omitempty"`
	Priority     *float32          `json:"priority,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Actor        string            `json:"actor,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
	AddDeps      []string          `json:"addDeps,omitempty"`
	RemoveDeps   []string          `json:"removeDeps,omitempty"`
}

type claimParams struct {
	ID    uint64 `json:"id"`
	Actor string `json:"actor"`
	TTL   string `json:"ttl,omitempty"`
}

type acquireParams struct {
	Actor        string   `json:"actor"`
	RequestID    string   `json:"requestId"`
	TTL          string   `json:"ttl,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type releaseParams struct {
	ID    uint64 `json:"id"`
	Actor string `json:"actor"`
	Error string `json:"error,omitempty"`
}

type blockParams struct {
	ID     uint64 `json:"id"`
	Reason string `json:"reason"`
	Actor  string `json:"actor,omitempty"`
}

type unblockParams struct {
	ID    uint64 `json:"id"`
	Actor string `json:"actor,omitempty"`
}

type nextParams struct {
	Capabilities []string `json:"capabilities,omitempty"`
}

type logParams struct {
	ID      uint64 `json:"id"`
	Message string `json:"message"`
	Actor   string `json:"actor,omitempty"`
}

type myParams struct {
	Actor string `json:"actor"`
}

type searchParams struct {
	Query string `json:"query"`
}

type dependsParams struct {
	ID uint64 `json:"id"`
}

type dependentsParams struct {
	ID uint64 `json:"id"`
}

type decomposeParams struct {
	ID       uint64             `json:"id"`
	Subtasks []decomposeSubtask `json:"subtasks"`
	Actor    string             `json:"actor,omitempty"`
}

type decomposeSubtask struct {
	Title        string   `json:"title"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type lineageParams struct {
	ID uint64 `json:"id"`
}

type eventsParams struct {
	Since uint64 `json:"since,omitempty"`
}

type whatIfParams struct {
	ID       uint64 `json:"id"`
	Scenario string `json:"scenario,omitempty"`
}

type graphParams struct {
	Format string `json:"format,omitempty"`
}

type configGetParams struct {
	Key string `json:"key,omitempty"`
}

type configSetParams struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type exportParams struct {
	Format string `json:"format,omitempty"`
}

type linkParams struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}

type unlinkParams struct {
	Alias string `json:"alias"`
}

type backupParams struct {
	Output string `json:"output,omitempty"`
}

type restoreParams struct {
	Path string `json:"path"`
}

type doctorResult map[string]interface{}

type doctorParams struct {
	Fix bool `json:"fix,omitempty"`
}

type agentCardParams struct {
	Actor   string   `json:"actor,omitempty"`
	Caps    []string `json:"caps,omitempty"`
	Desc    string   `json:"desc,omitempty"`
	MaxLoad int      `json:"maxLoad,omitempty"`
}

type capsParams struct {
	Actor string `json:"actor,omitempty"`
	All   bool   `json:"all,omitempty"`
}

type dependsEntry struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
	URI   string `json:"uri"`
}

type dependsResult struct {
	TaskID    uint64         `json:"task_id"`
	TaskTitle string         `json:"task_title"`
	Depends   []dependsEntry `json:"depends"`
}

type dependentsEntry struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

type dependentsResult struct {
	TaskID     uint64            `json:"task_id"`
	TaskTitle  string            `json:"task_title"`
	Dependents []dependentsEntry `json:"dependents"`
}

type decomposeResult struct {
	Parent   protocolTask   `json:"parent"`
	Subtasks []protocolTask `json:"subtasks"`
}

type claimResult struct {
	ID         uint64 `json:"id"`
	Owner      string `json:"owner"`
	Expires    string `json:"expires"`
	RetryCount uint32 `json:"retryCount"`
	LastError  string `json:"lastError"`
}

type releaseResult struct {
	ID     uint64 `json:"id"`
	Status string `json:"status"`
}

type blockResult struct {
	ID     uint64 `json:"id"`
	Status string `json:"status"`
}

type unblockResult struct {
	ID     uint64 `json:"id"`
	Status string `json:"status"`
}

type logResult struct {
	ID uint64 `json:"id"`
}

type whatIfResult struct {
	TaskID    uint64      `json:"task_id"`
	Title     string      `json:"title"`
	IfDone    interface{} `json:"if_done,omitempty"`
	IfBlocked interface{} `json:"if_blocked,omitempty"`
}

type graphNode struct {
	ID     uint64 `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type graphEdge struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}

type graphResult struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

type configGetResult struct {
	Config interface{} `json:"config"`
}

type configSetResult struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type pruneResult struct {
	RemovedCount int `json:"removedCount"`
}

type linkResult struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}

type unlinkResult struct {
	Alias string `json:"alias"`
}

type backupResult struct {
	Path      string `json:"path"`
	TaskCount int    `json:"taskCount"`
}

type restoreResult struct {
	TaskCount int `json:"taskCount"`
}

type agentCardResult struct {
	Actor      string   `json:"actor"`
	Caps       []string `json:"caps"`
	Desc       string   `json:"desc"`
	MaxLoad    int      `json:"maxLoad"`
	Registered bool     `json:"registered"`
}

type capsResult struct {
	Capabilities []string `json:"capabilities"`
	TaskCount    int      `json:"task_count"`
}

func rpcErrorf(code int, format string, args ...interface{}) *rpcError {
	return &rpcError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func unmarshalParams(params json.RawMessage, target interface{}) *rpcError {
	if len(params) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return rpcErrorf(rpcInvalidParams, "Invalid params: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return rpcErrorf(rpcInvalidParams, "Invalid params: trailing JSON data")
	}
	return nil
}

func loadStoreSafe() (*store.TaskStore, error) {
	return store.LoadCurrent(tasksBinPath())
}

func updateStoreSafe(mutate func(*store.TaskStore) error) (*store.TaskStore, error) {
	return store.Update(tasksBinPath(), mutate)
}

func (srv *server) ensureInitialized() *rpcError {
	if !srv.initialized {
		return &rpcError{Code: rpcNotInitialized, Message: "Project not initialized"}
	}
	return nil
}

func (srv *server) writeResult(id json.RawMessage, result interface{}) {
	srv.encoder.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (srv *server) writeError(id json.RawMessage, code int, message string, data interface{}) {
	srv.encoder.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}

func cmdServe(args []string) {
	if !hasFlag(args, "--stdio") {
		fmt.Fprintf(os.Stderr, "Error: --stdio flag is required for serve command\n")
		os.Exit(1)
	}

	initialized := true
	if _, err := os.Stat(tasksBinPath()); err != nil {
		initialized = false
	}

	srv := &server{
		initialized: initialized,
		encoder:     json.NewEncoder(os.Stdout),
	}

	srv.loop()
}

func (srv *server) loop() {
	if err := srv.readRequests(os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
}

func (srv *server) readRequests(input io.Reader) error {
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
		} else {
			srv.writeResult(req.ID, result)
		}
	}

	return scanner.Err()
}

func (srv *server) dispatch(method string, params json.RawMessage) (interface{}, *rpcError) {
	switch method {
	case "todo.ping":
		return srv.handlePing(params)
	case "todo.init":
		return srv.handleInit(params)
	case "todo.add":
		return srv.handleAdd(params)
	case "todo.done":
		return srv.handleDone(params)
	case "todo.status":
		return srv.handleStatus(params)
	case "todo.cat":
		return srv.handleCat(params)
	case "todo.update":
		return srv.handleUpdate(params)
	case "todo.claim":
		return srv.handleClaim(params)
	case "todo.acquire":
		return srv.handleAcquire(params)
	case "todo.release":
		return srv.handleRelease(params)
	case "todo.block":
		return srv.handleBlock(params)
	case "todo.unblock":
		return srv.handleUnblock(params)
	case "todo.next":
		return srv.handleNext(params)
	case "todo.log":
		return srv.handleLog(params)
	case "todo.my":
		return srv.handleMy(params)
	case "todo.search":
		return srv.handleSearch(params)
	case "todo.depends":
		return srv.handleDepends(params)
	case "todo.dependents":
		return srv.handleDependents(params)
	case "todo.decompose":
		return srv.handleDecompose(params)
	case "todo.lineage":
		return srv.handleLineage(params)
	case "todo.events":
		return srv.handleEvents(params)
	case "todo.whatIf":
		return srv.handleWhatIf(params)
	case "todo.graph":
		return srv.handleGraph(params)
	case "todo.config.get":
		return srv.handleConfigGet(params)
	case "todo.config.set":
		return srv.handleConfigSet(params)
	case "todo.prune":
		return srv.handlePrune(params)
	case "todo.export":
		return srv.handleExport(params)
	case "todo.link":
		return srv.handleLink(params)
	case "todo.unlink":
		return srv.handleUnlink(params)
	case "todo.backup":
		return srv.handleBackup(params)
	case "todo.restore":
		return srv.handleRestore(params)
	case "todo.doctor":
		return srv.handleDoctor(params)
	case "todo.agentCard":
		return srv.handleAgentCard(params)
	case "todo.caps":
		return srv.handleCaps(params)
	default:
		return nil, rpcErrorf(rpcMethodNotFound, "Method not found: %s", method)
	}
}

func (srv *server) handlePing(params json.RawMessage) (interface{}, *rpcError) {
	var p struct{}
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"version":     version,
		"initialized": srv.initialized,
		"project":     projectRoot,
	}, nil
}

func (srv *server) handleInit(params json.RawMessage) (interface{}, *rpcError) {
	var p struct{}
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	ttDir := filepath.Join(projectRoot, ".terminal-todo")
	storePath := filepath.Join(ttDir, "tasks.bin")

	if _, err := os.Stat(storePath); err == nil {
		srv.initialized = true
		return map[string]string{"path": projectRoot}, nil
	}

	if err := os.MkdirAll(ttDir, 0755); err != nil {
		return nil, rpcErrorf(rpcInternal, "creating directory: %v", err)
	}

	s := store.NewTaskStore()
	if err := s.Save(storePath); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "creating store: %v", err)
	}

	srv.initialized = true
	return map[string]string{"path": projectRoot}, nil
}

func (srv *server) handleAdd(params json.RawMessage) (interface{}, *rpcError) {
	var p addParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Title == "" {
		return nil, rpcErrorf(rpcInvalidParams, "title is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	cfg, _ := loadConfig()

	var priority float64
	if p.Priority != nil {
		priority = float64(*p.Priority)
	} else if cfg != nil {
		priority = float64(cfg.DefaultPriority)
	}

	capabilities := p.Capabilities
	if capabilities == nil && cfg != nil && cfg.DefaultCapCaps != "" {
		capabilities = normalizeCapabilities(cfg.DefaultCapCaps)
	}

	var taskID uint64
	var taskTitle string
	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)
		var finalDeps []string
		for _, dep := range p.After {
			depID, local := dag.ParseLocalID(dep)
			if local {
				if _, ok := s.Tasks[depID]; !ok {
					return fmt.Errorf("dependency task %d not found", depID)
				}
				finalDeps = append(finalDeps, fmt.Sprintf("todo://local/%d", depID))
			} else {
				if _, _, err := dag.ParseTaskURI(dep); err != nil {
					return err
				}
				finalDeps = append(finalDeps, dep)
			}
		}
		if err := d.DetectCycle(finalDeps, s.NextID); err != nil {
			return err
		}
		task := s.AddTask(p.Title, finalDeps)
		task.Priority = float32(priority)
		task.Capabilities = capabilities
		task.Tags = p.Tags
		if task.Tags == nil {
			task.Tags = []string{}
		}
		taskID = task.ID
		taskTitle = task.Title
		s.AddEvent(store.EventTaskCreated, task.ID, "", map[string]string{"title": p.Title})
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "cycle") {
			return nil, rpcErrorf(rpcCycleDetected, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return map[string]interface{}{"id": taskID, "title": taskTitle}, nil
}

func (srv *server) handleDone(params json.RawMessage) (interface{}, *rpcError) {
	var p doneParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if len(p.IDs) == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "ids is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	preflight, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}
	remoteTasks := make([]*store.Task, 0, len(p.IDs))
	for _, id := range p.IDs {
		if task, ok := preflight.GetTask(id); ok {
			remoteTasks = append(remoteTasks, task)
		}
	}
	resolver := snapshotDependencyResolver(remoteTasks)

	var completed []uint64
	_, err = updateStoreSafe(func(s *store.TaskStore) error {
		for _, id := range p.IDs {
			task, ok := s.GetTask(id)
			if !ok {
				return fmt.Errorf("task %d not found", id)
			}
			if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
				return fmt.Errorf("task %d has incomplete dependencies", id)
			}
			if task.Owner != "" && task.Owner != p.Actor {
				return fmt.Errorf("task %d is claimed by %s", id, task.Owner)
			}
			task.Status = store.StatusCompleted
			task.Completed = uint64(time.Now().UnixMilli())
			task.Owner = ""
			task.LeaseExpires = 0
			task.BlockReason = ""
			s.AddEvent(store.EventTaskCompleted, id, p.Actor, nil)
			completed = append(completed, id)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		if strings.Contains(err.Error(), "incomplete dependencies") {
			return nil, rpcErrorf(rpcDependency, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return map[string]interface{}{
		"completed": completed,
		"unblocked": []uint64{},
	}, nil
}

func (srv *server) handleStatus(params json.RawMessage) (interface{}, *rpcError) {
	var p statusParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	if p.All {
		return srv.handleStatusAll()
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	var protocolTasks []protocolTask
	for _, task := range tasks {
		if p.Tag != "" && !hasTag(task.Tags, p.Tag) {
			continue
		}
		if p.Actor != "" && task.Owner != p.Actor {
			continue
		}
		protocolTasks = append(protocolTasks, newProtocolTask(task))
	}
	if protocolTasks == nil {
		protocolTasks = []protocolTask{}
	}

	return tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, nil
}

func (srv *server) handleStatusAll() (interface{}, *rpcError) {
	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	projects := []projectStatus{projectStatusFromStore("local", ".", s)}
	registry, err := loadRepositoryRegistry()
	if err == nil {
		aliases := make([]string, 0, len(registry.Repositories))
		for alias := range registry.Repositories {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			path := registry.Repositories[alias]
			resolvedPath := path
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(projectRoot, resolvedPath)
			}
			linkedStore, err := store.LoadCurrent(filepath.Join(filepath.Clean(resolvedPath), ".terminal-todo", "tasks.bin"))
			if err != nil {
				projects = append(projects, projectStatus{Alias: alias, Path: path, Available: false, Error: err.Error(), Tasks: []protocolTask{}})
				continue
			}
			projects = append(projects, projectStatusFromStore(alias, path, linkedStore))
		}
	}

	return projectsEnvelope{SchemaVersion: protocolVersion, Projects: projects}, nil
}

func (srv *server) handleCat(params json.RawMessage) (interface{}, *rpcError) {
	var p catParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	task, ok := s.GetTask(p.ID)
	if !ok {
		return nil, rpcErrorf(rpcTaskNotFound, "task %d not found", p.ID)
	}

	return newProtocolTask(task), nil
}

func (srv *server) handleUpdate(params json.RawMessage) (interface{}, *rpcError) {
	var p updateParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if p.Title == nil && p.Priority == nil && p.Capabilities == nil && len(p.Extra) == 0 && len(p.AddDeps) == 0 && len(p.RemoveDeps) == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "nothing to update")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	var updated *store.Task
	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Owner != "" && task.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", task.ID, task.Owner)
		}

		if len(p.AddDeps) > 0 || len(p.RemoveDeps) > 0 {
			depSet := make(map[string]bool)
			for _, dep := range task.Depends {
				depSet[dep] = true
			}
			for _, dep := range p.RemoveDeps {
				if !depSet[dep] {
					return fmt.Errorf("dependency %q not found on task %d", dep, task.ID)
				}
				delete(depSet, dep)
			}
			for _, dep := range p.AddDeps {
				if depSet[dep] {
					continue
				}
				depID, local := dag.ParseLocalID(dep)
				if local {
					if _, ok := s.Tasks[depID]; !ok {
						return fmt.Errorf("dependency task %d not found", depID)
					}
				} else {
					if _, _, err := dag.ParseTaskURI(dep); err != nil {
						return err
					}
				}
				depSet[dep] = true
			}

			newDeps := make([]string, 0, len(depSet))
			for dep := range depSet {
				newDeps = append(newDeps, dep)
			}

			d := dag.NewDAG()
			oldDeps := task.Depends
			task.Depends = newDeps
			d.BuildFromTasks(s.Tasks)
			task.Depends = oldDeps

			if err := d.DetectCycle(nil, task.ID); err != nil {
				return fmt.Errorf("cannot update dependencies: %w", err)
			}

			oldSet := make(map[string]bool)
			for _, dep := range task.Depends {
				oldSet[dep] = true
			}
			for _, dep := range newDeps {
				if !oldSet[dep] {
					s.AddEvent(store.EventDependencyAdded, task.ID, p.Actor, map[string]string{"dep": dep})
				}
			}
			for _, dep := range task.Depends {
				if !depSet[dep] {
					s.AddEvent(store.EventDependencyRemoved, task.ID, p.Actor, map[string]string{"dep": dep})
				}
			}

			task.Depends = newDeps
		}

		if p.Title != nil {
			title := strings.TrimSpace(*p.Title)
			if title == "" {
				return fmt.Errorf("title cannot be empty")
			}
			task.Title = title
		}
		if p.Priority != nil {
			if *p.Priority < 0 || *p.Priority > 1 {
				return fmt.Errorf("priority must be between 0 and 1")
			}
			task.Priority = *p.Priority
		}
		if p.Capabilities != nil {
			task.Capabilities = p.Capabilities
		}
		if task.Extra == nil {
			task.Extra = make(map[string]string)
		}
		for key, value := range p.Extra {
			task.Extra[key] = value
		}
		if p.Title != nil || p.Priority != nil || p.Capabilities != nil || len(p.Extra) > 0 {
			s.AddEvent(store.EventTaskUpdated, task.ID, p.Actor, nil)
		}

		updated = task
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		if strings.Contains(err.Error(), "cycle") {
			return nil, rpcErrorf(rpcCycleDetected, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return newProtocolTask(updated), nil
}

func (srv *server) handleClaim(params json.RawMessage) (interface{}, *rpcError) {
	var p claimParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if p.Actor == "" {
		return nil, rpcErrorf(rpcInvalidParams, "actor is required")
	}
	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	cfg, cfgErr := loadConfig()
	if cfgErr != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading config: %v", cfgErr)
	}
	ttl := parseDefaultTTL(cfg)
	if p.TTL != "" {
		t, err := time.ParseDuration(p.TTL)
		if err != nil || t <= 0 {
			return nil, rpcErrorf(rpcInvalidParams, "ttl must be a positive duration")
		}
		ttl = t
	}
	if err := touchAgent(p.Actor); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "registering agent %s: %v", p.Actor, err)
	}

	preflight, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}
	var resolver dag.DependencyResolver
	if task, ok := preflight.GetTask(p.ID); ok {
		resolver = snapshotDependencyResolver([]*store.Task{task})
	}

	var result claimResult
	_, err = updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Status == store.StatusCompleted {
			return fmt.Errorf("task %d is already completed", p.ID)
		}
		if task.Status == store.StatusBlocked {
			return fmt.Errorf("task %d is blocked", p.ID)
		}
		if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
			return fmt.Errorf("task %d has incomplete dependencies", p.ID)
		}
		now := uint64(time.Now().UnixMilli())
		if task.Owner != "" && task.Owner != p.Actor && task.LeaseExpires > now {
			return fmt.Errorf("task %d already claimed by %s", p.ID, task.Owner)
		}

		task.Owner = p.Actor
		task.Status = store.StatusInProgress
		task.LeaseExpires = now + uint64(ttl.Milliseconds())
		s.AddLog(p.ID, p.Actor, "claimed")
		s.AddEvent(store.EventTaskClaimed, p.ID, p.Actor, map[string]string{"ttl": ttl.String()})

		result = claimResult{
			ID:         p.ID,
			Owner:      p.Actor,
			Expires:    formatTimestamp(task.LeaseExpires),
			RetryCount: task.RetryCount,
			LastError:  task.LastError,
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "already claimed") {
			return nil, rpcErrorf(rpcAlreadyClaimed, "%v", err)
		}
		if strings.Contains(err.Error(), "incomplete dependencies") {
			return nil, rpcErrorf(rpcDependency, "%v", err)
		}
		if strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "unblock") {
			return nil, rpcErrorf(rpcDependency, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return result, nil
}

func (srv *server) handleAcquire(params json.RawMessage) (interface{}, *rpcError) {
	var p acquireParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Actor == "" {
		return nil, rpcErrorf(rpcInvalidParams, "actor is required")
	}
	if err := validateAcquireRequestID(p.RequestID); err != nil {
		return nil, rpcErrorf(rpcInvalidParams, "requestId: %v", err)
	}
	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading config: %v", err)
	}
	ttl := parseDefaultTTL(cfg)
	ttlMode := "default"
	if p.TTL != "" {
		ttl, err = time.ParseDuration(p.TTL)
		if err != nil || ttl <= 0 {
			return nil, rpcErrorf(rpcInvalidParams, "ttl must be a positive duration")
		}
		ttlMode = "explicit:" + ttl.String()
	}
	if err := touchAgent(p.Actor); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "registering agent %s: %v", p.Actor, err)
	}
	var explicitCapabilities []string
	capabilitiesMode := "registered"
	if p.Capabilities != nil {
		explicitCapabilities = normalizeCapabilities(strings.Join(p.Capabilities, ","))
		if explicitCapabilities == nil {
			explicitCapabilities = []string{}
		}
		capabilitiesMode = "explicit"
	}
	capabilities, maxLoad, err := agentAllocationProfile(p.Actor, explicitCapabilities)
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading agent profile: %v", err)
	}
	preflight, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}
	resolver := snapshotDependencyResolver(preflight.GetAllTasks())
	fingerprint := acquireFingerprint(p.Actor, ttlMode, capabilitiesMode, explicitCapabilities)

	var acquired *store.Task
	var replayed bool
	_, err = updateStoreSafe(func(s *store.TaskStore) error {
		var acquireErr error
		acquired, replayed, acquireErr = acquireFromStore(s, p.Actor, p.RequestID, fingerprint, ttl, capabilities, maxLoad, resolver)
		return acquireErr
	})
	if err != nil {
		switch {
		case errors.Is(err, errNoReadyTasks):
			return nil, rpcErrorf(rpcNoWork, "%v", err)
		case errors.Is(err, errAgentAtCapacity):
			return nil, rpcErrorf(rpcAgentCapacity, "%v", err)
		case errors.Is(err, errAcquireRequestConflict):
			return nil, rpcErrorf(rpcIdempotencyConflict, "%v", err)
		default:
			return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
		}
	}
	return acquireEnvelope{SchemaVersion: protocolVersion, RequestID: p.RequestID, Replayed: replayed, Task: newProtocolTask(acquired)}, nil
}

func (srv *server) handleRelease(params json.RawMessage) (interface{}, *rpcError) {
	var p releaseParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if p.Actor == "" {
		return nil, rpcErrorf(rpcInvalidParams, "actor is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Status != store.StatusInProgress {
			return fmt.Errorf("task %d is not in progress", p.ID)
		}
		if task.Owner != "" && task.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", p.ID, task.Owner)
		}

		task.RetryCount++
		data := map[string]string{}
		if p.Error != "" {
			task.LastError = p.Error
			data["error"] = p.Error
			s.AddLog(p.ID, p.Actor, fmt.Sprintf("released with error: %s", p.Error))
		} else {
			s.AddLog(p.ID, p.Actor, "released")
		}
		s.AddEvent(store.EventTaskReleased, p.ID, p.Actor, data)
		task.Owner = ""
		task.LeaseExpires = 0
		task.Status = store.StatusPending
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return releaseResult{ID: p.ID, Status: "pending"}, nil
}

func (srv *server) handleBlock(params json.RawMessage) (interface{}, *rpcError) {
	var p blockParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if p.Reason == "" {
		return nil, rpcErrorf(rpcInvalidParams, "reason is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Status == store.StatusCompleted {
			return fmt.Errorf("task %d is already completed", p.ID)
		}
		if task.Owner != "" && task.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", p.ID, task.Owner)
		}

		task.Status = store.StatusBlocked
		task.BlockReason = p.Reason
		s.AddLog(p.ID, p.Actor, fmt.Sprintf("blocked: %s", p.Reason))
		s.AddEvent(store.EventTaskBlocked, p.ID, p.Actor, map[string]string{"reason": p.Reason})
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return blockResult{ID: p.ID, Status: "blocked"}, nil
}

func (srv *server) handleUnblock(params json.RawMessage) (interface{}, *rpcError) {
	var p unblockParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Status != store.StatusBlocked {
			return fmt.Errorf("task %d is not blocked", p.ID)
		}
		if task.Owner != "" && task.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", p.ID, task.Owner)
		}

		task.Status = store.StatusPending
		task.BlockReason = ""
		s.AddLog(p.ID, p.Actor, "unblocked")
		s.AddEvent(store.EventTaskUnblocked, p.ID, p.Actor, nil)
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return unblockResult{ID: p.ID, Status: "pending"}, nil
}

func (srv *server) handleNext(params json.RawMessage) (interface{}, *rpcError) {
	var p nextParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)
	resolver := dependencyResolver()
	ready := d.GetReadyTasksWithResolver(s.Tasks, resolver)

	if len(p.Capabilities) > 0 {
		var filtered []*store.Task
		for _, t := range ready {
			if matchesCapabilities(t.Capabilities, p.Capabilities) {
				filtered = append(filtered, t)
			}
		}
		ready = filtered
	}

	sort.Slice(ready, func(i, j int) bool {
		if ready[i].Priority == ready[j].Priority {
			return ready[i].ID < ready[j].ID
		}
		return ready[i].Priority > ready[j].Priority
	})

	available := make([]availableTask, 0, len(ready))
	for _, task := range ready {
		caps := append([]string(nil), task.Capabilities...)
		if caps == nil {
			caps = []string{}
		}
		available = append(available, availableTask{
			ID: task.ID, Title: task.Title, Priority: task.Priority,
			Capabilities: caps, Reason: "ready: all dependencies completed",
		})
	}

	return nextEnvelope{
		SchemaVersion:  protocolVersion,
		AvailableTasks: available,
		BlockedSummary: newBlockedSummaryWithResolver(s.Tasks, resolver),
	}, nil
}

func (srv *server) handleLog(params json.RawMessage) (interface{}, *rpcError) {
	var p logParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if p.Message == "" {
		return nil, rpcErrorf(rpcInvalidParams, "message is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		task, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("task %d not found", p.ID)
		}
		if task.Owner != "" && task.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", p.ID, task.Owner)
		}
		s.AddLog(p.ID, p.Actor, p.Message)
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return logResult{ID: p.ID}, nil
}

func (srv *server) handleMy(params json.RawMessage) (interface{}, *rpcError) {
	var p myParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Actor == "" {
		return nil, rpcErrorf(rpcInvalidParams, "actor is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	tasks := s.GetAllTasks()
	var mine []*store.Task
	for _, t := range tasks {
		if t.Owner == p.Actor {
			mine = append(mine, t)
		}
	}

	sort.Slice(mine, func(i, j int) bool {
		if mine[i].Status != mine[j].Status {
			return mine[i].Status < mine[j].Status
		}
		return mine[i].ID < mine[j].ID
	})

	protocolTasks := make([]protocolTask, 0, len(mine))
	for _, t := range mine {
		protocolTasks = append(protocolTasks, newProtocolTask(t))
	}

	return tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, nil
}

func (srv *server) handleSearch(params json.RawMessage) (interface{}, *rpcError) {
	var p searchParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Query == "" {
		return nil, rpcErrorf(rpcInvalidParams, "query is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	queryLower := strings.ToLower(p.Query)
	var results []*store.Task
	for _, task := range s.GetAllTasks() {
		if strings.Contains(strings.ToLower(task.Title), queryLower) {
			results = append(results, task)
			continue
		}
		for _, tag := range task.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				results = append(results, task)
				break
			}
		}
	}

	protocolTasks := make([]protocolTask, 0, len(results))
	for _, t := range results {
		protocolTasks = append(protocolTasks, newProtocolTask(t))
	}

	return tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, nil
}

func (srv *server) handleDepends(params json.RawMessage) (interface{}, *rpcError) {
	var p dependsParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	task, ok := s.GetTask(p.ID)
	if !ok {
		return nil, rpcErrorf(rpcTaskNotFound, "task %d not found", p.ID)
	}

	depends := make([]dependsEntry, 0, len(task.Depends))
	for _, dep := range task.Depends {
		depID, local := dag.ParseLocalID(dep)
		if local {
			if dt, ok := s.GetTask(depID); ok {
				depends = append(depends, dependsEntry{ID: depID, Title: dt.Title, URI: dep})
			} else {
				depends = append(depends, dependsEntry{ID: depID, Title: "[not found locally]", URI: dep})
			}
		} else {
			depends = append(depends, dependsEntry{URI: dep})
		}
	}

	return dependsResult{
		TaskID:    p.ID,
		TaskTitle: task.Title,
		Depends:   depends,
	}, nil
}

func (srv *server) handleDependents(params json.RawMessage) (interface{}, *rpcError) {
	var p dependentsParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	task, ok := s.GetTask(p.ID)
	if !ok {
		return nil, rpcErrorf(rpcTaskNotFound, "task %d not found", p.ID)
	}

	var dependents []dependentsEntry
	for _, t := range s.Tasks {
		for _, dep := range t.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local && depID == p.ID {
				dependents = append(dependents, dependentsEntry{ID: t.ID, Title: t.Title})
				break
			}
		}
	}

	return dependentsResult{
		TaskID:     p.ID,
		TaskTitle:  task.Title,
		Dependents: dependents,
	}, nil
}

func (srv *server) handleDecompose(params json.RawMessage) (interface{}, *rpcError) {
	var p decomposeParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}
	if len(p.Subtasks) == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "at least one subtask is required")
	}
	for _, sub := range p.Subtasks {
		if strings.TrimSpace(sub.Title) == "" {
			return nil, rpcErrorf(rpcInvalidParams, "subtask title is required")
		}
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	var parentProtocol protocolTask
	var subtaskProtocols []protocolTask
	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		parentTask, ok := s.GetTask(p.ID)
		if !ok {
			return fmt.Errorf("parent task %d not found", p.ID)
		}
		if parentTask.Status == store.StatusCompleted {
			return fmt.Errorf("parent task %d is already completed", p.ID)
		}
		if parentTask.Owner != "" && parentTask.Owner != p.Actor {
			return fmt.Errorf("task %d is claimed by %s", p.ID, parentTask.Owner)
		}

		var added []*store.Task
		for _, sub := range p.Subtasks {
			subTask := s.AddTask(strings.TrimSpace(sub.Title), nil)
			subTask.Capabilities = sub.Capabilities
			subTask.Lineage = fmt.Sprintf("todo://local/%d", p.ID)
			parentTask.Depends = append(parentTask.Depends, fmt.Sprintf("todo://local/%d", subTask.ID))
			added = append(added, subTask)
		}

		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)
		if err := d.DetectCycle(parentTask.Depends, parentTask.ID); err != nil {
			return fmt.Errorf("decompose would create a cycle: %w", err)
		}

		parentTask.Status = store.StatusPending
		if p.Actor != "" {
			parentTask.Owner = p.Actor
		} else {
			parentTask.Owner = ""
		}
		parentTask.LeaseExpires = 0
		s.AddEvent(store.EventTaskDecomposed, p.ID, "", map[string]string{"count": fmt.Sprintf("%d", len(p.Subtasks))})

		parentProtocol = newProtocolTask(parentTask)
		subtaskProtocols = make([]protocolTask, 0, len(added))
		for _, sub := range added {
			subtaskProtocols = append(subtaskProtocols, newProtocolTask(sub))
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, rpcErrorf(rpcTaskNotFound, "%v", err)
		}
		if strings.Contains(err.Error(), "claimed by") {
			return nil, rpcErrorf(rpcNotOwner, "%v", err)
		}
		if strings.Contains(err.Error(), "cycle") {
			return nil, rpcErrorf(rpcCycleDetected, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return decomposeResult{Parent: parentProtocol, Subtasks: subtaskProtocols}, nil
}

func (srv *server) handleLineage(params json.RawMessage) (interface{}, *rpcError) {
	var p lineageParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	root, ok := s.GetTask(p.ID)
	if !ok {
		return nil, rpcErrorf(rpcTaskNotFound, "task %d not found", p.ID)
	}

	descendants := lineageDescendants(root.ID, s.Tasks)
	progress := calculateLineageProgress(append([]*store.Task{root}, descendants...))

	protocolDescendants := make([]protocolTask, 0, len(descendants))
	for _, task := range descendants {
		protocolDescendants = append(protocolDescendants, newProtocolTask(task))
	}

	return lineageEnvelope{
		SchemaVersion: protocolVersion,
		Root:          newProtocolTask(root),
		Descendants:   protocolDescendants,
		Progress:      progress,
	}, nil
}

func (srv *server) handleEvents(params json.RawMessage) (interface{}, *rpcError) {
	var p eventsParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	events := s.EventsSince(p.Since)

	return map[string]interface{}{
		"events": events,
	}, nil
}

func (srv *server) handleWhatIf(params json.RawMessage) (interface{}, *rpcError) {
	var p whatIfParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 {
		return nil, rpcErrorf(rpcInvalidParams, "id is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	task, ok := s.GetTask(p.ID)
	if !ok {
		return nil, rpcErrorf(rpcTaskNotFound, "task %d not found", p.ID)
	}

	result := whatIfResult{
		TaskID: p.ID,
		Title:  task.Title,
	}

	if p.Scenario == "" || p.Scenario == "done" {
		d := dag.NewDAG()
		d.BuildFromTasks(s.Tasks)

		simTasks := make(map[uint64]*store.Task)
		for k, v := range s.Tasks {
			t := *v
			t.Depends = append([]string(nil), v.Depends...)
			t.Capabilities = append([]string(nil), v.Capabilities...)
			t.Tags = append([]string(nil), v.Tags...)
			t.Log = append([]store.LogEntry(nil), v.Log...)
			simTasks[k] = &t
		}
		if simTask, ok := simTasks[p.ID]; ok {
			simTask.Status = store.StatusCompleted
		}

		resolver := dependencyResolver()
		ready := d.GetReadyTasksWithResolver(simTasks, resolver)
		beforeBlocked := d.GetBlockedTasksWithResolver(s.Tasks, resolver)

		var newlyReady []*store.Task
		for _, t := range ready {
			if _, wasBlocked := beforeBlocked[t.ID]; wasBlocked {
				newlyReady = append(newlyReady, t)
			}
		}

		type unblockedEntry struct {
			ID    uint64 `json:"id"`
			Title string `json:"title"`
		}
		unblocked := make([]unblockedEntry, 0, len(newlyReady))
		for _, t := range newlyReady {
			unblocked = append(unblocked, unblockedEntry{ID: t.ID, Title: t.Title})
		}

		stillBlocked := len(d.GetBlockedTasksWithResolver(simTasks, resolver))
		result.IfDone = map[string]interface{}{
			"unblocked":           unblocked,
			"still_blocked_count": stillBlocked,
		}
	}

	if p.Scenario == "" || p.Scenario == "block" {
		count := 0
		for _, t := range s.Tasks {
			for _, dep := range t.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local && depID == p.ID {
					count++
					break
				}
			}
		}
		result.IfBlocked = map[string]interface{}{
			"dependents_count": count,
		}
	}

	return result, nil
}

func (srv *server) handleGraph(params json.RawMessage) (interface{}, *rpcError) {
	var p graphParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	var nodes []graphNode
	var edges []graphEdge
	for _, t := range s.Tasks {
		nodes = append(nodes, graphNode{
			ID:     t.ID,
			Title:  t.Title,
			Status: statusName(t.Status),
		})
		for _, dep := range t.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local {
				edges = append(edges, graphEdge{From: depID, To: t.ID})
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	return graphResult{Nodes: nodes, Edges: edges}, nil
}

func (srv *server) handleConfigGet(params json.RawMessage) (interface{}, *rpcError) {
	var p configGetParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading config: %v", err)
	}

	if p.Key != "" {
		var value string
		switch p.Key {
		case "default_ttl":
			value = cfg.DefaultTTL
		case "default_priority":
			value = fmt.Sprintf("%.2f", cfg.DefaultPriority)
		case "default_caps":
			value = cfg.DefaultCapCaps
		default:
			return nil, rpcErrorf(rpcInvalidParams, "unknown config key: %s", p.Key)
		}
		return configGetResult{
			Config: map[string]string{p.Key: value},
		}, nil
	}

	return configGetResult{Config: cfg}, nil
}

func (srv *server) handleConfigSet(params json.RawMessage) (interface{}, *rpcError) {
	var p configSetParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Key == "" {
		return nil, rpcErrorf(rpcInvalidParams, "key is required")
	}
	if p.Value == "" {
		return nil, rpcErrorf(rpcInvalidParams, "value is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	if err := updateConfig(func(cfg *ProjectConfig) error {
		switch p.Key {
		case "default_ttl":
			d, err := time.ParseDuration(p.Value)
			if err != nil || d <= 0 {
				return fmt.Errorf("default_ttl must be a positive duration")
			}
			cfg.DefaultTTL = p.Value
		case "default_priority":
			val, err := strconv.ParseFloat(p.Value, 32)
			if err != nil || val < 0 || val > 1 {
				return fmt.Errorf("default_priority must be between 0 and 1")
			}
			cfg.DefaultPriority = float32(val)
		case "default_caps":
			cfg.DefaultCapCaps = p.Value
		default:
			return fmt.Errorf("unknown config key %q", p.Key)
		}
		return nil
	}); err != nil {
		if strings.Contains(err.Error(), "unknown config key") {
			return nil, rpcErrorf(rpcInvalidParams, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return configSetResult{Key: p.Key, Value: p.Value}, nil
}

func (srv *server) handlePrune(params json.RawMessage) (interface{}, *rpcError) {
	var p struct{}
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	var removedCount int
	_, err := updateStoreSafe(func(s *store.TaskStore) error {
		completed := make(map[uint64]struct{})
		for _, t := range s.GetAllTasks() {
			if t.Status == store.StatusCompleted {
				completed[t.ID] = struct{}{}
			}
		}
		for _, task := range s.Tasks {
			if _, willRemove := completed[task.ID]; willRemove {
				continue
			}
			kept := task.Depends[:0]
			for _, dependency := range task.Depends {
				dependencyID, local := dag.ParseLocalID(dependency)
				if _, pruned := completed[dependencyID]; local && pruned {
					continue
				}
				kept = append(kept, dependency)
			}
			task.Depends = kept
		}
		for id := range completed {
			s.RemoveTask(id)
			removedCount++
		}
		return nil
	})
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return pruneResult{RemovedCount: removedCount}, nil
}

func (srv *server) handleExport(params json.RawMessage) (interface{}, *rpcError) {
	var p exportParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	tasks := s.GetAllTasks()
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	protocolTasks := make([]protocolTask, 0, len(tasks))
	for _, t := range tasks {
		protocolTasks = append(protocolTasks, newProtocolTask(t))
	}

	return tasksEnvelope{SchemaVersion: protocolVersion, Tasks: protocolTasks}, nil
}

func (srv *server) handleLink(params json.RawMessage) (interface{}, *rpcError) {
	var p linkParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Alias == "" || p.Path == "" {
		return nil, rpcErrorf(rpcInvalidParams, "alias and path are required")
	}
	if p.Alias == "local" {
		return nil, rpcErrorf(rpcInvalidParams, "alias 'local' is reserved")
	}
	if _, _, err := dag.ParseTaskURI(fmt.Sprintf("todo://%s/1", p.Alias)); err != nil {
		return nil, rpcErrorf(rpcInvalidParams, "%v", err)
	}

	target, err := filepath.Abs(p.Path)
	if err != nil {
		return nil, rpcErrorf(rpcInvalidParams, "resolving path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".terminal-todo", "tasks.bin")); err != nil {
		return nil, rpcErrorf(rpcNotInitialized, "%s is not an initialized todo project", target)
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	currentInfo, currentErr := os.Stat(projectRoot)
	targetInfo, targetErr := os.Stat(target)
	if currentErr == nil && targetErr == nil && os.SameFile(currentInfo, targetInfo) {
		return nil, rpcErrorf(rpcInvalidParams, "cannot link a project to itself")
	}

	storedPath := target
	if relative, err := filepath.Rel(projectRoot, target); err == nil {
		storedPath = relative
	}

	if err := updateRepositoryRegistry(func(registry *repositoryRegistry) error {
		registry.Repositories[p.Alias] = storedPath
		return nil
	}); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "saving registry: %v", err)
	}

	return linkResult{Alias: p.Alias, Path: storedPath}, nil
}

func (srv *server) handleUnlink(params json.RawMessage) (interface{}, *rpcError) {
	var p unlinkParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Alias == "" {
		return nil, rpcErrorf(rpcInvalidParams, "alias is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	var found bool
	if err := updateRepositoryRegistry(func(registry *repositoryRegistry) error {
		if _, ok := registry.Repositories[p.Alias]; !ok {
			return fmt.Errorf("alias %s not found", p.Alias)
		}
		delete(registry.Repositories, p.Alias)
		found = true
		return nil
	}); err != nil {
		if !found {
			return nil, rpcErrorf(rpcInvalidParams, "%v", err)
		}
		return nil, rpcErrorf(rpcStoreCorrupted, "%v", err)
	}

	return unlinkResult{Alias: p.Alias}, nil
}

func (srv *server) handleBackup(params json.RawMessage) (interface{}, *rpcError) {
	var p backupParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	output := p.Output
	if output == "" {
		output = filepath.Join(projectRoot, ".terminal-todo", fmt.Sprintf("backup-%d.bin", time.Now().UnixMilli()))
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "creating backup directory: %v", err)
	}

	if err := s.Save(output); err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "saving backup: %v", err)
	}

	return backupResult{Path: output, TaskCount: len(s.Tasks)}, nil
}

func (srv *server) handleRestore(params json.RawMessage) (interface{}, *rpcError) {
	var p restoreParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, rpcErrorf(rpcInvalidParams, "path is required")
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	backup, err := store.Load(p.Path)
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading backup: %v", err)
	}

	var taskCount int
	_, err = updateStoreSafe(func(existing *store.TaskStore) error {
		existing.Tasks = backup.Tasks
		existing.NextID = backup.NextID
		taskCount = len(backup.Tasks)
		return nil
	})
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "restoring from backup: %v", err)
	}

	return restoreResult{TaskCount: taskCount}, nil
}

func (srv *server) handleDoctor(params json.RawMessage) (interface{}, *rpcError) {
	var p doctorParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	ttDir := filepath.Join(projectRoot, ".terminal-todo")
	result := make(doctorResult)

	tasksBin := filepath.Join(ttDir, "tasks.bin")
	if info, err := os.Stat(tasksBin); err == nil {
		result["tasks_bin"] = fmt.Sprintf("ok (%d B)", info.Size())
	} else {
		result["tasks_bin"] = "MISSING"
	}

	reposJSON := filepath.Join(ttDir, "repositories.json")
	if info, err := os.Stat(reposJSON); err == nil {
		result["repositories_json"] = fmt.Sprintf("ok (%d B)", info.Size())
	} else {
		result["repositories_json"] = "absent"
	}

	configJSON := filepath.Join(ttDir, "config.json")
	if info, err := os.Stat(configJSON); err == nil {
		result["config_json"] = fmt.Sprintf("ok (%d B)", info.Size())
	} else {
		result["config_json"] = "absent"
	}

	result["stale_locks"] = false

	s, err := store.Load(tasksBin)
	if err != nil {
		result["store_load"] = fmt.Sprintf("error: %v", err)
	} else {
		result["task_count"] = len(s.Tasks)
		orphaned := 0
		for _, t := range s.Tasks {
			for _, dep := range t.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local {
					if _, ok := s.Tasks[depID]; !ok {
						orphaned++
					}
				}
			}
		}
		result["orphaned_deps"] = orphaned
	}

	return result, nil
}

func (srv *server) handleAgentCard(params json.RawMessage) (interface{}, *rpcError) {
	var p agentCardParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	return agentCardResult{
		Actor:      p.Actor,
		Caps:       p.Caps,
		Desc:       p.Desc,
		MaxLoad:    p.MaxLoad,
		Registered: true,
	}, nil
}

func (srv *server) handleCaps(params json.RawMessage) (interface{}, *rpcError) {
	var p capsParams
	if err := unmarshalParams(params, &p); err != nil {
		return nil, err
	}

	if err := srv.ensureInitialized(); err != nil {
		return nil, err
	}

	s, err := loadStoreSafe()
	if err != nil {
		return nil, rpcErrorf(rpcStoreCorrupted, "loading store: %v", err)
	}

	capSet := make(map[string]bool)
	taskCount := 0
	for _, t := range s.Tasks {
		if p.Actor != "" && t.Owner != p.Actor {
			continue
		}
		if !p.All && t.Status == store.StatusCompleted {
			continue
		}
		taskCount++
		for _, c := range t.Capabilities {
			capSet[c] = true
		}
	}

	caps := make([]string, 0, len(capSet))
	for c := range capSet {
		caps = append(caps, c)
	}
	sort.Strings(caps)

	return capsResult{Capabilities: caps, TaskCount: taskCount}, nil
}
