package conformance

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

//go:embed scenarios/*.json
var catalogAssets embed.FS

// Catalog is the executable, versioned real-agent scenario catalog.
type Catalog struct {
	SchemaVersion  string
	SuiteID        string
	ScenarioSchema string
	ScoringModel   ScoringModel
	Scenarios      []CatalogScenario
}

type catalogManifest struct {
	SchemaVersion  string `json:"schema_version"`
	SuiteID        string `json:"suite_id"`
	ScenarioSchema string `json:"scenario_schema"`
	ScoringModel   string `json:"scoring_model"`
	Scenarios      []struct {
		ID   string `json:"id"`
		File string `json:"file"`
	} `json:"scenarios"`
}

type CatalogScenario struct {
	SchemaVersion  string             `json:"schema_version"`
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Summary        string             `json:"summary"`
	InitialTime    time.Time          `json:"-"`
	InitialTimeRaw string             `json:"initial_time"`
	SkillPolicy    string             `json:"skill_policy"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	Actors         []CatalogActor     `json:"actors"`
	Project        CatalogProject     `json:"project"`
	Turns          []CatalogTurn      `json:"turns"`
	Assertions     []CatalogAssertion `json:"assertions"`
}

type CatalogActor struct {
	Ref          string   `json:"ref"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	MaxLoad      int      `json:"max_load,omitempty"`
}

type CatalogProject struct {
	Initialized bool          `json:"initialized"`
	Tasks       []CatalogTask `json:"tasks"`
}

type CatalogTask struct {
	Ref          string            `json:"ref"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Capabilities []string          `json:"capabilities"`
	DependsOn    []string          `json:"depends_on"`
	Owner        string            `json:"owner,omitempty"`
	LeaseExpires string            `json:"lease_expires,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type CatalogTurn struct {
	ID          string   `json:"id"`
	By          string   `json:"by"`
	Action      string   `json:"action"`
	Prompt      string   `json:"prompt,omitempty"`
	Seconds     int      `json:"seconds,omitempty"`
	Actors      []string `json:"actors,omitempty"`
	Instruction string   `json:"instruction,omitempty"`
}

type CatalogAssertion struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Description string         `json:"description"`
	Criteria    []string       `json:"criteria"`
	HardGate    string         `json:"hard_gate,omitempty"`
	Expect      map[string]any `json:"expect"`
}

// LoadCatalog parses and validates the embedded manifest, scoring model, and
// every scenario in manifest order.
func LoadCatalog() (Catalog, error) {
	var manifest catalogManifest
	if err := readCatalogAsset("manifest.json", &manifest); err != nil {
		return Catalog{}, err
	}
	var model ScoringModel
	if err := readCatalogAsset(manifest.ScoringModel, &model); err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{
		SchemaVersion:  manifest.SchemaVersion,
		SuiteID:        manifest.SuiteID,
		ScenarioSchema: manifest.ScenarioSchema,
		ScoringModel:   model,
		Scenarios:      make([]CatalogScenario, 0, len(manifest.Scenarios)),
	}
	for _, entry := range manifest.Scenarios {
		if filepath.Base(entry.File) != entry.File {
			return Catalog{}, fmt.Errorf("scenario %q uses unsafe file path %q", entry.ID, entry.File)
		}
		var scenario CatalogScenario
		if err := readCatalogAsset(entry.File, &scenario); err != nil {
			return Catalog{}, err
		}
		if scenario.ID != entry.ID {
			return Catalog{}, fmt.Errorf("manifest scenario %q loaded fixture %q", entry.ID, scenario.ID)
		}
		initialTime, err := time.Parse(time.RFC3339, scenario.InitialTimeRaw)
		if err != nil {
			return Catalog{}, fmt.Errorf("scenario %q initial time: %w", scenario.ID, err)
		}
		scenario.InitialTime = initialTime
		catalog.Scenarios = append(catalog.Scenarios, scenario)
	}
	if err := validateCatalog(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func readCatalogAsset(name string, target any) error {
	if strings.TrimSpace(name) == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid conformance catalog asset %q", name)
	}
	data, err := catalogAssets.ReadFile("scenarios/" + name)
	if err != nil {
		return fmt.Errorf("read conformance catalog asset %q: %w", name, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse conformance catalog asset %q: %w", name, err)
	}
	return nil
}

func validateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != "1" || strings.TrimSpace(catalog.SuiteID) == "" {
		return errors.New("invalid conformance catalog identity")
	}
	if err := validateScoringModel(catalog.ScoringModel); err != nil {
		return err
	}
	if len(catalog.Scenarios) == 0 {
		return errors.New("conformance catalog has no scenarios")
	}
	criteria := criterionIDs(catalog.ScoringModel)
	gates := hardGateIDs(catalog.ScoringModel)
	referencedCriteria := make(map[string]bool, len(criteria))
	seenScenarios := make(map[string]bool, len(catalog.Scenarios))
	for _, scenario := range catalog.Scenarios {
		if seenScenarios[scenario.ID] {
			return fmt.Errorf("duplicate conformance scenario %q", scenario.ID)
		}
		seenScenarios[scenario.ID] = true
		if err := validateCatalogScenario(scenario, criteria, gates, referencedCriteria); err != nil {
			return err
		}
	}
	for criterion := range criteria {
		if !referencedCriteria[criterion] {
			return fmt.Errorf("conformance criterion %q is not exercised", criterion)
		}
	}
	return nil
}

func validateCatalogScenario(scenario CatalogScenario, criteria, gates map[string]struct{}, referenced map[string]bool) error {
	if scenario.SchemaVersion != "1" || strings.TrimSpace(scenario.ID) == "" || strings.TrimSpace(scenario.Title) == "" || strings.TrimSpace(scenario.Summary) == "" {
		return fmt.Errorf("scenario %q has invalid identity", scenario.ID)
	}
	if scenario.SkillPolicy != "project_integration" && scenario.SkillPolicy != "not_preloaded" {
		return fmt.Errorf("scenario %q has invalid skill policy %q", scenario.ID, scenario.SkillPolicy)
	}
	if scenario.TimeoutSeconds < 1 || scenario.TimeoutSeconds > 600 {
		return fmt.Errorf("scenario %q timeout must be between 1 and 600 seconds", scenario.ID)
	}
	actors := make(map[string]CatalogActor, len(scenario.Actors))
	for _, actor := range scenario.Actors {
		if strings.TrimSpace(actor.Ref) == "" || strings.TrimSpace(actor.Name) == "" {
			return fmt.Errorf("scenario %q has an invalid actor", scenario.ID)
		}
		if _, duplicate := actors[actor.Ref]; duplicate {
			return fmt.Errorf("scenario %q repeats actor %q", scenario.ID, actor.Ref)
		}
		actors[actor.Ref] = actor
	}
	if len(actors) == 0 {
		return fmt.Errorf("scenario %q has no actors", scenario.ID)
	}
	tasks := make(map[string]CatalogTask, len(scenario.Project.Tasks))
	for _, task := range scenario.Project.Tasks {
		if strings.TrimSpace(task.Ref) == "" || strings.TrimSpace(task.Title) == "" {
			return fmt.Errorf("scenario %q has an invalid task", scenario.ID)
		}
		if _, duplicate := tasks[task.Ref]; duplicate {
			return fmt.Errorf("scenario %q repeats task %q", scenario.ID, task.Ref)
		}
		if task.Status != "pending" && task.Status != "in_progress" && task.Status != "completed" && task.Status != "blocked" {
			return fmt.Errorf("scenario %q task %q has invalid status %q", scenario.ID, task.Ref, task.Status)
		}
		if task.Owner != "" {
			if _, ok := actors[strings.TrimPrefix(task.Owner, "actor:")]; !strings.HasPrefix(task.Owner, "actor:") || !ok {
				return fmt.Errorf("scenario %q task %q references unknown owner %q", scenario.ID, task.Ref, task.Owner)
			}
		}
		if task.LeaseExpires != "" {
			if _, err := time.Parse(time.RFC3339, task.LeaseExpires); err != nil {
				return fmt.Errorf("scenario %q task %q lease expiry: %w", scenario.ID, task.Ref, err)
			}
		}
		tasks[task.Ref] = task
	}
	for _, task := range scenario.Project.Tasks {
		for _, dependency := range task.DependsOn {
			ref := strings.TrimPrefix(dependency, "task:")
			if !strings.HasPrefix(dependency, "task:") || ref == task.Ref {
				return fmt.Errorf("scenario %q task %q has invalid dependency %q", scenario.ID, task.Ref, dependency)
			}
			if _, ok := tasks[ref]; !ok {
				return fmt.Errorf("scenario %q task %q references unknown dependency %q", scenario.ID, task.Ref, dependency)
			}
		}
	}
	seenTurns := make(map[string]bool, len(scenario.Turns))
	for _, turn := range scenario.Turns {
		if strings.TrimSpace(turn.ID) == "" || seenTurns[turn.ID] {
			return fmt.Errorf("scenario %q has invalid or duplicate turn %q", scenario.ID, turn.ID)
		}
		seenTurns[turn.ID] = true
		if err := validateCatalogTurn(scenario.ID, turn, actors); err != nil {
			return err
		}
	}
	if len(seenTurns) == 0 {
		return fmt.Errorf("scenario %q has no turns", scenario.ID)
	}
	seenAssertions := make(map[string]bool, len(scenario.Assertions))
	for _, assertion := range scenario.Assertions {
		if strings.TrimSpace(assertion.ID) == "" || seenAssertions[assertion.ID] {
			return fmt.Errorf("scenario %q has invalid or duplicate assertion %q", scenario.ID, assertion.ID)
		}
		seenAssertions[assertion.ID] = true
		if strings.TrimSpace(assertion.Description) == "" || len(assertion.Expect) == 0 {
			return fmt.Errorf("scenario %q assertion %q is incomplete", scenario.ID, assertion.ID)
		}
		switch assertion.Kind {
		case "operation_trace", "task_state", "event_log", "domain_error", "assistant_output":
		default:
			return fmt.Errorf("scenario %q assertion %q has invalid kind %q", scenario.ID, assertion.ID, assertion.Kind)
		}
		if len(assertion.Criteria) == 0 {
			return fmt.Errorf("scenario %q assertion %q has no criteria", scenario.ID, assertion.ID)
		}
		for _, criterion := range assertion.Criteria {
			if _, ok := criteria[criterion]; !ok {
				return fmt.Errorf("scenario %q assertion %q references unknown criterion %q", scenario.ID, assertion.ID, criterion)
			}
			referenced[criterion] = true
		}
		if assertion.HardGate != "" {
			if _, ok := gates[assertion.HardGate]; !ok {
				return fmt.Errorf("scenario %q assertion %q references unknown hard gate %q", scenario.ID, assertion.ID, assertion.HardGate)
			}
		}
	}
	if len(seenAssertions) == 0 {
		return fmt.Errorf("scenario %q has no assertions", scenario.ID)
	}
	return nil
}

func validateCatalogTurn(scenarioID string, turn CatalogTurn, actors map[string]CatalogActor) error {
	actorRef := func(value string) bool {
		if !strings.HasPrefix(value, "actor:") {
			return false
		}
		_, ok := actors[strings.TrimPrefix(value, "actor:")]
		return ok
	}
	switch turn.Action {
	case "prompt":
		if !actorRef(turn.By) || strings.TrimSpace(turn.Prompt) == "" {
			return fmt.Errorf("scenario %q prompt turn %q is invalid", scenarioID, turn.ID)
		}
	case "resume":
		if !actorRef(turn.By) || strings.TrimSpace(turn.Instruction) == "" {
			return fmt.Errorf("scenario %q resume turn %q is invalid", scenarioID, turn.ID)
		}
	case "advance_clock":
		if turn.By != "harness" || turn.Seconds < 1 {
			return fmt.Errorf("scenario %q clock turn %q is invalid", scenarioID, turn.ID)
		}
	case "checkpoint":
		if turn.By != "harness" || strings.TrimSpace(turn.Instruction) == "" {
			return fmt.Errorf("scenario %q checkpoint turn %q is invalid", scenarioID, turn.ID)
		}
	case "concurrent_turns":
		if turn.By != "harness" || len(turn.Actors) < 2 || strings.TrimSpace(turn.Prompt) == "" {
			return fmt.Errorf("scenario %q concurrent turn %q is invalid", scenarioID, turn.ID)
		}
		seen := make(map[string]bool, len(turn.Actors))
		for _, actor := range turn.Actors {
			if !actorRef(actor) || seen[actor] {
				return fmt.Errorf("scenario %q concurrent turn %q has invalid actor %q", scenarioID, turn.ID, actor)
			}
			seen[actor] = true
		}
	default:
		return fmt.Errorf("scenario %q turn %q has invalid action %q", scenarioID, turn.ID, turn.Action)
	}
	return nil
}
