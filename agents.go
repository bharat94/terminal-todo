package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"terminal-todo/lock"
	"terminal-todo/store"
)

type AgentCard struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description,omitempty"`
	MaxLoad      int      `json:"max_load,omitempty"`
	CreatedAt    string   `json:"created_at"`
	LastSeen     string   `json:"last_seen,omitempty"`
}

type AgentRegistry struct {
	SchemaVersion string                `json:"schema_version"`
	Agents        map[string]AgentCard  `json:"agents"`
}

func agentsPath() string {
	return filepath.Join(projectRoot, ".terminal-todo", "agents.json")
}

func loadAgentRegistry() (*AgentRegistry, error) {
	data, err := os.ReadFile(agentsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentRegistry{
				SchemaVersion: "1",
				Agents:        make(map[string]AgentCard),
			}, nil
		}
		return nil, err
	}
	var registry AgentRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("invalid agent registry: %w", err)
	}
	if registry.Agents == nil {
		registry.Agents = make(map[string]AgentCard)
	}
	return &registry, nil
}

func saveAgentRegistry(registry *AgentRegistry) error {
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = "1"
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(agentsPath()), ".agents-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, agentsPath()); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(agentsPath()))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func updateAgentRegistry(mutate func(*AgentRegistry) error) error {
	lk, err := lock.Open(agentsPath())
	if err != nil {
		return err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Write); err != nil {
		return err
	}
	defer lk.Release()

	registry, err := loadAgentRegistry()
	if err != nil {
		return err
	}
	if err := mutate(registry); err != nil {
		return err
	}
	return saveAgentRegistry(registry)
}

func computeAgentLoad(s *store.TaskStore, agentName string) int {
	count := 0
	for _, task := range s.GetAllTasks() {
		if task.Owner == agentName && task.Status == store.StatusInProgress {
			count++
		}
	}
	return count
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
