package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type agentDetail struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description,omitempty"`
	MaxLoad      int      `json:"max_load,omitempty"`
	CurrentLoad  int      `json:"current_load,omitempty"`
	CreatedAt    string   `json:"created_at"`
	LastSeen     string   `json:"last_seen,omitempty"`
}

type agentEnvelope struct {
	SchemaVersion string      `json:"schema_version"`
	Agent         agentDetail `json:"agent"`
}

type agentsEnvelope struct {
	SchemaVersion string               `json:"schema_version"`
	Agents        map[string]AgentCard `json:"agents"`
}

func cmdAgentCard(args []string) {
	agentName := optionValue(args, "--as")
	hasJSON := hasFlag(args, "--json")

	if agentName == "" {
		registry, err := loadAgentRegistry()
		if err != nil {
			fail(ErrStoreCorrupted, "loading agent registry: %v", err)
		}
		writeJSON(agentsEnvelope{
			SchemaVersion: "1",
			Agents:        registry.Agents,
		})
		return
	}

	isModification := hasFlag(args, "--caps") || hasFlag(args, "--desc") || hasFlag(args, "--max-load")

	if isModification {
		if err := validateActor(agentName, true); err != nil {
			fail(ErrInvalidArgs, "%v", err)
		}
		var capabilities []string
		if hasFlag(args, "--caps") {
			capabilities = normalizeCapabilities(optionValue(args, "--caps"))
			if capabilities == nil {
				capabilities = []string{}
			}
			if err := validateCapabilities(capabilities); err != nil {
				fail(ErrInvalidArgs, "%v", err)
			}
		}
		description := optionValue(args, "--desc")
		if hasFlag(args, "--desc") {
			if err := validatePersistedString("description", description, maxAgentDescriptionBytes); err != nil {
				fail(ErrInvalidArgs, "%v", err)
			}
		}
		now := nowTimestamp()
		err := updateAgentRegistry(func(r *AgentRegistry) error {
			card, exists := r.Agents[agentName]
			if !exists {
				card = AgentCard{
					Name:      agentName,
					CreatedAt: now,
				}
			}
			if hasFlag(args, "--caps") {
				card.Capabilities = capabilities
			}
			if hasFlag(args, "--desc") {
				card.Description = description
			}
			if hasFlag(args, "--max-load") {
				ml, err := strconv.Atoi(optionValue(args, "--max-load"))
				if err != nil || ml < 0 {
					return fmt.Errorf("--max-load must be a non-negative integer")
				}
				card.MaxLoad = ml
			}
			card.LastSeen = now
			r.Agents[agentName] = card
			return nil
		})
		if err != nil {
			fail(ErrStoreCorrupted, "updating agent registry: %v", err)
		}
		fmt.Printf("Agent %s updated\n", agentName)
		return
	}

	registry, err := loadAgentRegistry()
	if err != nil {
		fail(ErrStoreCorrupted, "loading agent registry: %v", err)
	}
	card, ok := registry.Agents[agentName]
	if !ok {
		fail(ErrTaskNotFound, "agent %s not found in registry", agentName)
	}

	s := loadStore()
	currentLoad := computeAgentLoad(s, agentName)

	detail := agentDetail{
		Name:         card.Name,
		Capabilities: card.Capabilities,
		Description:  card.Description,
		MaxLoad:      card.MaxLoad,
		CurrentLoad:  currentLoad,
		CreatedAt:    card.CreatedAt,
		LastSeen:     card.LastSeen,
	}

	if hasJSON {
		writeJSON(agentEnvelope{
			SchemaVersion: "1",
			Agent:         detail,
		})
		return
	}

	fmt.Printf("Agent: %s\n", detail.Name)
	if len(detail.Capabilities) > 0 {
		sort.Strings(detail.Capabilities)
		fmt.Printf("  Capabilities: %s\n", strings.Join(detail.Capabilities, ", "))
	}
	if detail.Description != "" {
		fmt.Printf("  Description: %s\n", detail.Description)
	}
	if detail.MaxLoad > 0 {
		fmt.Printf("  Max Load: %d\n", detail.MaxLoad)
	}
	fmt.Printf("  Current Load: %d\n", detail.CurrentLoad)
	fmt.Printf("  Created: %s\n", detail.CreatedAt)
	if detail.LastSeen != "" {
		fmt.Printf("  Last Seen: %s\n", detail.LastSeen)
	}
}
