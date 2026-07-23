package conformance

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed scenarios/*.json
var scenarioFiles embed.FS

type scenarioManifest struct {
	SchemaVersion string `json:"schema_version"`
	SuiteID       string `json:"suite_id"`
	Scenarios     []struct {
		ID   string `json:"id"`
		File string `json:"file"`
	} `json:"scenarios"`
}

type scenarioFixture struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	InitialTime   string `json:"initial_time"`
	Actors        []struct {
		Ref string `json:"ref"`
	} `json:"actors"`
	Project struct {
		Tasks []struct {
			Ref string `json:"ref"`
		} `json:"tasks"`
	} `json:"project"`
	Turns []struct {
		ID string `json:"id"`
	} `json:"turns"`
	Assertions []struct {
		ID       string   `json:"id"`
		Criteria []string `json:"criteria"`
		HardGate string   `json:"hard_gate"`
		Expect   any      `json:"expect"`
	} `json:"assertions"`
}

func TestScenarioCatalogIsCompleteAndInternallyConsistent(t *testing.T) {
	var manifest scenarioManifest
	readScenarioJSON(t, "manifest.json", &manifest)
	assert.Equal(t, "1", manifest.SchemaVersion)
	assert.Equal(t, "terminal-todo-real-agent-v1", manifest.SuiteID)
	require.Len(t, manifest.Scenarios, 9)

	var model ScoringModel
	readScenarioJSON(t, "scoring-model.json", &model)
	knownCriteria := criterionIDs(model)
	knownGates := hardGateIDs(model)
	seenScenarios := make(map[string]struct{}, len(manifest.Scenarios))
	referencedCriteria := make(map[string]struct{})

	for _, entry := range manifest.Scenarios {
		t.Run(entry.ID, func(t *testing.T) {
			assert.Equal(t, filepath.Base(entry.File), entry.File)
			if _, duplicate := seenScenarios[entry.ID]; duplicate {
				t.Fatalf("duplicate scenario %q", entry.ID)
			}
			seenScenarios[entry.ID] = struct{}{}

			var fixture scenarioFixture
			readScenarioJSON(t, entry.File, &fixture)
			assert.Equal(t, "1", fixture.SchemaVersion)
			assert.Equal(t, entry.ID, fixture.ID)
			assert.NotEmpty(t, fixture.Title)
			assert.NotEmpty(t, fixture.Summary)
			_, err := time.Parse(time.RFC3339, fixture.InitialTime)
			assert.NoError(t, err)
			assert.NotEmpty(t, fixture.Actors)
			assert.NotEmpty(t, fixture.Turns)
			assert.NotEmpty(t, fixture.Assertions)

			assertUniqueRefs(t, "actor", actorRefs(fixture))
			assertUniqueRefs(t, "task", taskRefs(fixture))
			assertUniqueRefs(t, "turn", turnRefs(fixture))
			assertionIDs := make([]string, 0, len(fixture.Assertions))
			for _, assertion := range fixture.Assertions {
				assertionIDs = append(assertionIDs, assertion.ID)
				assert.NotEmpty(t, assertion.Expect)
				require.NotEmpty(t, assertion.Criteria)
				for _, criterion := range assertion.Criteria {
					if _, ok := knownCriteria[criterion]; !ok {
						t.Errorf("assertion %q references unknown criterion %q", assertion.ID, criterion)
					}
					referencedCriteria[criterion] = struct{}{}
				}
				if assertion.HardGate != "" {
					if _, ok := knownGates[assertion.HardGate]; !ok {
						t.Errorf("assertion %q references unknown hard gate %q", assertion.ID, assertion.HardGate)
					}
				}
			}
			assertUniqueRefs(t, "assertion", assertionIDs)
		})
	}

	for criterion := range knownCriteria {
		assert.Contains(t, referencedCriteria, criterion, "criterion must be exercised by the catalog")
	}
}

func readScenarioJSON(t *testing.T, name string, target any) {
	t.Helper()
	data, err := scenarioFiles.ReadFile("scenarios/" + name)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, target), fmt.Sprintf("parse %s", name))
}

func actorRefs(fixture scenarioFixture) []string {
	result := make([]string, 0, len(fixture.Actors))
	for _, actor := range fixture.Actors {
		result = append(result, actor.Ref)
	}
	return result
}

func taskRefs(fixture scenarioFixture) []string {
	result := make([]string, 0, len(fixture.Project.Tasks))
	for _, task := range fixture.Project.Tasks {
		result = append(result, task.Ref)
	}
	return result
}

func turnRefs(fixture scenarioFixture) []string {
	result := make([]string, 0, len(fixture.Turns))
	for _, turn := range fixture.Turns {
		result = append(result, turn.ID)
	}
	return result
}

func assertUniqueRefs(t *testing.T, kind string, values []string) {
	t.Helper()
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		assert.NotEmpty(t, value)
		if _, duplicate := seen[value]; duplicate {
			t.Errorf("duplicate %s reference %q", kind, value)
		}
		seen[value] = struct{}{}
	}
}
