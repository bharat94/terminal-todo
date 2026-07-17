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

func acquireFromStore(s *store.TaskStore, actor, requestID, fingerprint string, ttl time.Duration, capabilities []string, maxLoad int, resolver dag.DependencyResolver) (*store.Task, bool, error) {
	if receipt, ok := s.Acquisitions[requestID]; ok {
		if receipt.Operation != "acquire.v1" || receipt.Fingerprint != fingerprint {
			return nil, false, fmt.Errorf("%w: %q", errAcquireRequestConflict, requestID)
		}
		task := cloneTask(receipt.Task)
		return task, true, nil
	}
	if maxLoad > 0 && computeAgentLoad(s, actor) >= maxLoad {
		return nil, false, fmt.Errorf("%w: %s has reached max load %d", errAgentAtCapacity, actor, maxLoad)
	}
	ready := rankedReadyTasks(s, resolver, capabilities, true)
	if len(ready) == 0 {
		return nil, false, errNoReadyTasks
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
