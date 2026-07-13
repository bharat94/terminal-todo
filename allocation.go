package main

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"terminal-todo/dag"
	"terminal-todo/store"
)

var (
	errNoReadyTasks    = errors.New("no compatible tasks ready")
	errAgentAtCapacity = errors.New("agent is at capacity")
)

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

func acquireFromStore(s *store.TaskStore, actor string, ttl time.Duration, capabilities []string, maxLoad int, resolver dag.DependencyResolver) (*store.Task, error) {
	if maxLoad > 0 && computeAgentLoad(s, actor) >= maxLoad {
		return nil, fmt.Errorf("%w: %s has reached max load %d", errAgentAtCapacity, actor, maxLoad)
	}
	ready := rankedReadyTasks(s, resolver, capabilities, true)
	if len(ready) == 0 {
		return nil, errNoReadyTasks
	}

	task := ready[0]
	now := uint64(time.Now().UnixMilli())
	task.Owner = actor
	task.Status = store.StatusInProgress
	task.LeaseExpires = now + uint64(ttl.Milliseconds())
	s.AddLog(task.ID, actor, "acquired")
	s.AddEvent(store.EventTaskClaimed, task.ID, actor, map[string]string{"ttl": ttl.String(), "mode": "acquire"})
	return task, nil
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
