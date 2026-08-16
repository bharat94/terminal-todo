package conformance

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCatalogReturnsAllExecutableScenarioDataInManifestOrder(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	assert.Equal(t, "1", catalog.SchemaVersion)
	assert.Equal(t, "terminal-todo-real-agent-v1", catalog.SuiteID)
	require.Len(t, catalog.Scenarios, 9)
	assert.Equal(t, []string{
		"discovery", "bootstrap", "atomic_acquire", "heartbeat", "handoff",
		"no_work", "lease_recovery", "quiet_narration", "cleanup",
	}, catalogScenarioIDs(catalog.Scenarios))
	assert.Equal(t, "terminal-todo-agent-conformance-v1", catalog.ScoringModel.ModelID)

	for _, scenario := range catalog.Scenarios {
		assert.False(t, scenario.InitialTime.IsZero(), scenario.ID)
		assert.LessOrEqual(t, time.Duration(scenario.TimeoutSeconds)*time.Second, 10*time.Minute, scenario.ID)
		assert.NotEmpty(t, scenario.Actors, scenario.ID)
		assert.NotEmpty(t, scenario.Turns, scenario.ID)
		assert.NotEmpty(t, scenario.Assertions, scenario.ID)
	}
}

func TestLoadCatalogVersionKeepsV1StableAndLoadsOutcomeBasedV2(t *testing.T) {
	v1, err := LoadCatalogVersion("v1")
	require.NoError(t, err)
	v2, err := LoadCatalogVersion("v2")
	require.NoError(t, err)

	assert.Equal(t, "terminal-todo-real-agent-v1", v1.SuiteID)
	assert.Equal(t, "terminal-todo-agent-conformance-v1", v1.ScoringModel.ModelID)
	assert.Equal(t, "terminal-todo-real-agent-v2", v2.SuiteID)
	assert.Equal(t, "terminal-todo-agent-conformance-v2", v2.ScoringModel.ModelID)
	assert.Equal(t, catalogScenarioIDs(v1.Scenarios), catalogScenarioIDs(v2.Scenarios))
	assert.Contains(t, scenarioByID(t, v1, "heartbeat").Assertions[0].Expect, "ordered_operations")
	assert.Contains(t, scenarioByID(t, v1, "handoff").Assertions[0].Expect, "update_extra_contains")
	assert.Contains(t, scenarioByID(t, v2, "heartbeat").Assertions[0].Expect, "ordered_operation_alternatives")
	assert.Contains(t, scenarioByID(t, v2, "handoff").Assertions[1].Expect, "extra_value_contains")

	_, err = LoadCatalogVersion("latest")
	assert.ErrorContains(t, err, "unsupported conformance suite")
}

func scenarioByID(t *testing.T, catalog Catalog, id string) CatalogScenario {
	t.Helper()
	for _, scenario := range catalog.Scenarios {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("missing scenario %q", id)
	return CatalogScenario{}
}

func TestCatalogResolvesEverySymbolicTurnAndProjectReference(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			actors := make(map[string]bool)
			for _, actor := range scenario.Actors {
				actors[actor.Ref] = true
			}
			tasks := make(map[string]bool)
			for _, task := range scenario.Project.Tasks {
				tasks[task.Ref] = true
			}
			for _, task := range scenario.Project.Tasks {
				for _, dependency := range task.DependsOn {
					assert.True(t, tasks[strings.TrimPrefix(dependency, "task:")], dependency)
				}
			}
			for _, turn := range scenario.Turns {
				if turn.By != "harness" {
					assert.True(t, actors[strings.TrimPrefix(turn.By, "actor:")], turn.By)
				}
				for _, actor := range turn.Actors {
					assert.True(t, actors[strings.TrimPrefix(actor, "actor:")], actor)
				}
			}
		})
	}
}

func catalogScenarioIDs(scenarios []CatalogScenario) []string {
	ids := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		ids = append(ids, scenario.ID)
	}
	return ids
}
