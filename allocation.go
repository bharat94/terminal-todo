package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bharat94/terminal-todo/dag"

	"github.com/bharat94/terminal-todo/store"
)

func validateAcquireRequestID(requestID string) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("request ID is required")
	}
	if len(requestID) > 128 {
		return errors.New("request ID must be at most 128 bytes")
	}
	return nil
}

var (
	errNoReadyTasks           = errors.New("no compatible tasks ready")
	errAgentAtCapacity        = errors.New("agent is at capacity")
	errAcquireRequestConflict = errors.New("acquire request ID was already used with different parameters")
)

type allocationReason string

const (
	allocationReady                   allocationReason = "ready"
	allocationNoPendingWork           allocationReason = "no_pending_work"
	allocationDependenciesIncomplete  allocationReason = "dependencies_incomplete"
	allocationCapabilitiesUnavailable allocationReason = "capabilities_unavailable"
	allocationWorkOwnedByOthers       allocationReason = "work_owned_by_others"
	allocationWorkInProgress          allocationReason = "work_in_progress"
	allocationWorkerAtCapacity        allocationReason = "worker_at_capacity"

	maxAllocationDiagnosticItems      = 20
	maxAllocationDiagnosticValueBytes = 96
)

type allocationQueueDiagnostics struct {
	Pending                      int      `json:"pending"`
	Ready                        int      `json:"ready"`
	CompatibleReady              int      `json:"compatible_ready"`
	DependencyBlocked            int      `json:"dependency_blocked"`
	DependencyBlockers           []string `json:"dependency_blockers"`
	DependencyBlockersTotal      int      `json:"dependency_blockers_total"`
	DependencyBlockersTruncated  bool     `json:"dependency_blockers_truncated"`
	CapabilityMismatch           int      `json:"capability_mismatch"`
	MissingCapabilities          []string `json:"missing_capabilities"`
	MissingCapabilitiesTotal     int      `json:"missing_capabilities_total"`
	MissingCapabilitiesTruncated bool     `json:"missing_capabilities_truncated"`
	InProgress                   int      `json:"in_progress"`
	Blocked                      int      `json:"blocked"`
	Completed                    int      `json:"completed"`
	RetriedPending               int      `json:"retried_pending"`
}

type allocationWorkerDiagnostics struct {
	CurrentLoad   int `json:"current_load"`
	MaxLoad       int `json:"max_load"`
	Owned         int `json:"owned"`
	OwnedByOthers int `json:"owned_by_others"`
}

type allocationDiagnostics struct {
	Reason                         allocationReason             `json:"reason"`
	RequestedCapabilities          []string                     `json:"requested_capabilities"`
	RequestedCapabilitiesTotal     int                          `json:"requested_capabilities_total"`
	RequestedCapabilitiesTruncated bool                         `json:"requested_capabilities_truncated"`
	Queue                          allocationQueueDiagnostics   `json:"queue"`
	Worker                         *allocationWorkerDiagnostics `json:"worker,omitempty"`
}

type allocationError struct {
	cause       error
	message     string
	diagnostics allocationDiagnostics
}

func (e *allocationError) Error() string {
	return e.message
}

func (e *allocationError) Unwrap() error {
	return e.cause
}

func allocationDiagnosticsFromError(err error) (allocationDiagnostics, bool) {
	var allocationErr *allocationError
	if !errors.As(err, &allocationErr) {
		return allocationDiagnostics{}, false
	}
	return allocationErr.diagnostics, true
}

type acquireFingerprintInput struct {
	Operation        string   `json:"operation"`
	Actor            string   `json:"actor"`
	TTLMode          string   `json:"ttl_mode"`
	CapabilitiesMode string   `json:"capabilities_mode"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

func acquireFingerprint(actor, ttlMode, capabilitiesMode string, capabilities []string) string {
	canonicalCapabilities := append([]string(nil), capabilities...)
	sort.Strings(canonicalCapabilities)
	payload, _ := json.Marshal(acquireFingerprintInput{
		Operation:        "acquire.v1",
		Actor:            actor,
		TTLMode:          ttlMode,
		CapabilitiesMode: capabilitiesMode,
		Capabilities:     canonicalCapabilities,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func rankedReadyTasks(s *store.TaskStore, resolver dag.DependencyResolver, capabilities []string, filterCapabilities bool) []*store.Task {
	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)
	ready := d.GetReadyTasksWithResolver(s.Tasks, resolver)
	if filterCapabilities {
		filtered := ready[:0]
		for _, task := range ready {
			if matchesCapabilities(task.Capabilities, capabilities) {
				filtered = append(filtered, task)
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
	return ready
}

func diagnoseAllocation(s *store.TaskStore, resolver dag.DependencyResolver, actor string, capabilities []string, filterCapabilities bool, maxLoad int) allocationDiagnostics {
	requestedCapabilities := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		requestedCapabilities[capability] = struct{}{}
	}
	diagnostics := allocationDiagnostics{}
	diagnostics.RequestedCapabilities,
		diagnostics.RequestedCapabilitiesTotal,
		diagnostics.RequestedCapabilitiesTruncated = boundedAllocationDiagnosticValues(requestedCapabilities)
	ready := rankedReadyTasks(s, resolver, capabilities, false)
	compatibleReady := ready
	if filterCapabilities {
		compatibleReady = rankedReadyTasks(s, resolver, capabilities, true)
	}

	diagnostics.Queue.Ready = len(ready)
	diagnostics.Queue.CompatibleReady = len(compatibleReady)
	diagnostics.Queue.CapabilityMismatch = len(ready) - len(compatibleReady)

	capabilitySet := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capabilitySet[capability] = struct{}{}
	}
	missingCapabilities := make(map[string]struct{})
	if filterCapabilities {
		for _, task := range ready {
			if matchesCapabilities(task.Capabilities, capabilities) {
				continue
			}
			for _, required := range task.Capabilities {
				if _, provided := capabilitySet[required]; !provided {
					missingCapabilities[required] = struct{}{}
				}
			}
		}
	}

	dependencyBlockers := make(map[string]struct{})
	ownedByActor := 0
	ownedByOthers := 0
	for _, task := range s.Tasks {
		switch task.Status {
		case store.StatusPending:
			diagnostics.Queue.Pending++
			if !dag.DependenciesCompleteWithResolver(task, s.Tasks, resolver) {
				diagnostics.Queue.DependencyBlocked++
				for _, dependency := range task.Depends {
					dependencyComplete := false
					if dependencyID, local := dag.ParseLocalID(dependency); local {
						dependencyTask, exists := s.Tasks[dependencyID]
						dependencyComplete = exists && dependencyTask.Status == store.StatusCompleted
					} else if resolver != nil {
						dependencyComplete = resolver(dependency)
					}
					if !dependencyComplete {
						dependencyBlockers[dependency] = struct{}{}
					}
				}
			}
			if task.RetryCount > 0 {
				diagnostics.Queue.RetriedPending++
			}
		case store.StatusInProgress:
			diagnostics.Queue.InProgress++
			if actor != "" && task.Owner == actor {
				ownedByActor++
			} else {
				ownedByOthers++
			}
		case store.StatusBlocked:
			diagnostics.Queue.Blocked++
		case store.StatusCompleted:
			diagnostics.Queue.Completed++
		}
	}
	diagnostics.Queue.DependencyBlockers,
		diagnostics.Queue.DependencyBlockersTotal,
		diagnostics.Queue.DependencyBlockersTruncated = boundedAllocationDiagnosticValues(dependencyBlockers)
	diagnostics.Queue.MissingCapabilities,
		diagnostics.Queue.MissingCapabilitiesTotal,
		diagnostics.Queue.MissingCapabilitiesTruncated = boundedAllocationDiagnosticValues(missingCapabilities)

	if actor != "" {
		diagnostics.Worker = &allocationWorkerDiagnostics{
			CurrentLoad:   ownedByActor,
			MaxLoad:       maxLoad,
			Owned:         ownedByActor,
			OwnedByOthers: ownedByOthers,
		}
	}

	switch {
	case actor != "" && maxLoad > 0 && ownedByActor >= maxLoad:
		diagnostics.Reason = allocationWorkerAtCapacity
	case diagnostics.Queue.CompatibleReady > 0:
		diagnostics.Reason = allocationReady
	case diagnostics.Queue.Pending > 0 && diagnostics.Queue.Ready == 0:
		diagnostics.Reason = allocationDependenciesIncomplete
	case diagnostics.Queue.Pending > 0:
		diagnostics.Reason = allocationCapabilitiesUnavailable
	case actor != "" && ownedByOthers > 0:
		diagnostics.Reason = allocationWorkOwnedByOthers
	case actor == "" && diagnostics.Queue.InProgress > 0:
		diagnostics.Reason = allocationWorkInProgress
	default:
		diagnostics.Reason = allocationNoPendingWork
	}
	return diagnostics
}

func boundedAllocationDiagnosticValues(values map[string]struct{}) ([]string, int, bool) {
	sorted := make([]string, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	sort.Strings(sorted)

	total := len(sorted)
	truncated := total > maxAllocationDiagnosticItems
	if truncated {
		sorted = sorted[:maxAllocationDiagnosticItems]
	}
	for i, value := range sorted {
		compact := compactAllocationDiagnosticValue(value)
		if compact != value {
			truncated = true
		}
		sorted[i] = compact
	}
	return sorted, total, truncated
}

func compactAllocationDiagnosticValue(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= maxAllocationDiagnosticValueBytes {
		return value
	}
	end := maxAllocationDiagnosticValueBytes - 3
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "..."
}

func allocationReasonMessage(reason allocationReason) string {
	switch reason {
	case allocationNoPendingWork:
		return "no pending work"
	case allocationDependenciesIncomplete:
		return "pending work is waiting on dependencies"
	case allocationCapabilitiesUnavailable:
		return "ready work requires capabilities this worker does not provide"
	case allocationWorkOwnedByOthers:
		return "work is currently owned by other workers"
	case allocationWorkInProgress:
		return "work is currently in progress"
	case allocationWorkerAtCapacity:
		return "worker is at capacity"
	default:
		return "work is ready"
	}
}

func acquireFromStore(s *store.TaskStore, actor, requestID, fingerprint string, ttl time.Duration, capabilities []string, maxLoad int, resolver dag.DependencyResolver) (*store.Task, bool, error) {
	if receipt, ok := s.Acquisitions[requestID]; ok {
		if receipt.Operation != "acquire.v1" || receipt.Fingerprint != fingerprint {
			return nil, false, fmt.Errorf("%w: %q", errAcquireRequestConflict, requestID)
		}
		task := cloneTask(receipt.Task)
		return task, true, nil
	}
	diagnostics := diagnoseAllocation(s, resolver, actor, capabilities, true, maxLoad)
	if diagnostics.Reason == allocationWorkerAtCapacity {
		return nil, false, &allocationError{
			cause:       errAgentAtCapacity,
			message:     fmt.Sprintf("%s (%d/%d active)", allocationReasonMessage(diagnostics.Reason), diagnostics.Worker.CurrentLoad, maxLoad),
			diagnostics: diagnostics,
		}
	}
	ready := rankedReadyTasks(s, resolver, capabilities, true)
	if len(ready) == 0 {
		return nil, false, &allocationError{
			cause:       errNoReadyTasks,
			message:     allocationReasonMessage(diagnostics.Reason),
			diagnostics: diagnostics,
		}
	}

	task := ready[0]
	now := uint64(time.Now().UnixMilli())
	task.Owner = actor
	task.Status = store.StatusInProgress
	task.LeaseExpires = now + uint64(ttl.Milliseconds())
	s.AddLog(task.ID, actor, "acquired")
	s.AddEvent(store.EventTaskClaimed, task.ID, actor, map[string]string{"ttl": ttl.String(), "mode": "acquire", "request_id": requestID})
	s.Acquisitions[requestID] = store.AcquisitionReceipt{
		RequestID:   requestID,
		Operation:   "acquire.v1",
		Fingerprint: fingerprint,
		Actor:       actor,
		Created:     now,
		Task:        *cloneTask(*task),
	}
	return task, false, nil
}

func cloneTask(task store.Task) *store.Task {
	clone := task
	clone.Depends = append([]string(nil), task.Depends...)
	clone.Capabilities = append([]string(nil), task.Capabilities...)
	clone.Tags = append([]string(nil), task.Tags...)
	clone.Log = append([]store.LogEntry(nil), task.Log...)
	clone.Extra = make(map[string]string, len(task.Extra))
	for key, value := range task.Extra {
		clone.Extra[key] = value
	}
	return &clone
}

func agentAllocationProfile(actor string, explicitCapabilities []string) ([]string, int, error) {
	registry, err := loadAgentRegistry()
	if err != nil {
		return nil, 0, err
	}
	card := registry.Agents[actor]
	capabilities := explicitCapabilities
	if capabilities == nil {
		capabilities = append([]string(nil), card.Capabilities...)
	}
	return capabilities, card.MaxLoad, nil
}
