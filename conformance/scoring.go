package conformance

import (
	"errors"
	"fmt"
	"math"
)

type Score struct {
	Earned   float64 `json:"earned"`
	Possible float64 `json:"possible"`
	Percent  float64 `json:"percent"`
	Scored   bool    `json:"scored"`
}

func evaluateAssertions(assertions []Assertion, observation Observation) ([]CheckResult, Score, bool) {
	results := make([]CheckResult, 0, len(assertions))
	score := Score{Scored: true}
	requiredPassed := true

	for _, assertion := range assertions {
		passed, detail := assertion.Evaluate(observation)
		result := CheckResult{
			ID:          assertion.ID,
			Description: assertion.Description,
			Passed:      passed,
			Required:    assertion.Required,
			Weight:      assertion.Weight,
			Criteria:    append([]string(nil), assertion.Criteria...),
			HardGate:    assertion.HardGate,
			Detail:      detail,
		}
		results = append(results, result)
		score.Possible += assertion.Weight
		if passed {
			score.Earned += assertion.Weight
		}
		if assertion.Required && !passed {
			requiredPassed = false
		}
	}

	if score.Possible == 0 {
		score.Percent = 100
	} else {
		score.Percent = math.Round((score.Earned/score.Possible)*1000) / 10
	}
	return results, score, requiredPassed
}

// ScoringModel mirrors the vendor-neutral scoring-model.json contract. Grade
// can combine checks from multiple scenario reports, so criterion points are
// awarded only after every assertion referencing that criterion passes.
type ScoringModel struct {
	SchemaVersion string      `json:"schema_version"`
	ModelID       string      `json:"model_id"`
	MaximumScore  float64     `json:"maximum_score"`
	CriterionRule string      `json:"criterion_rule"`
	Criteria      []Criterion `json:"criteria"`
	HardGates     []HardGate  `json:"hard_gates"`
	Levels        []Level     `json:"levels"`
}

type Criterion struct {
	ID          string  `json:"id"`
	Points      float64 `json:"points"`
	Dimension   string  `json:"dimension,omitempty"`
	Description string  `json:"description,omitempty"`
}

type HardGate struct {
	ID          string  `json:"id"`
	ScoreCap    float64 `json:"score_cap"`
	Description string  `json:"description,omitempty"`
}

type Level struct {
	ID                         string  `json:"id"`
	MinimumScore               float64 `json:"minimum_score"`
	MaximumScore               float64 `json:"maximum_score"`
	RequiresNoHardGateFailures bool    `json:"requires_no_hard_gate_failures"`
}

type CriterionResult struct {
	ID     string  `json:"id"`
	Points float64 `json:"points"`
	Passed bool    `json:"passed"`
}

type ModelScore struct {
	ModelID          string            `json:"model_id"`
	RawScore         float64           `json:"raw_score"`
	CappedScore      float64           `json:"capped_score"`
	MaximumScore     float64           `json:"maximum_score"`
	Level            string            `json:"level"`
	Criteria         []CriterionResult `json:"criteria"`
	HardGateFailures []string          `json:"hard_gate_failures"`
}

func Grade(model ScoringModel, checks []CheckResult) (ModelScore, error) {
	if model.MaximumScore == 0 {
		for _, criterion := range model.Criteria {
			model.MaximumScore += criterion.Points
		}
	}
	if err := validateScoringModel(model); err != nil {
		return ModelScore{}, err
	}
	result := ModelScore{
		ModelID:          model.ModelID,
		MaximumScore:     model.MaximumScore,
		Criteria:         make([]CriterionResult, 0, len(model.Criteria)),
		HardGateFailures: []string{},
	}

	criterionChecks := make(map[string][]CheckResult)
	failedGates := make(map[string]bool)
	knownCriteria := criterionIDs(model)
	knownGates := hardGateIDs(model)
	for _, check := range checks {
		for _, criterion := range check.Criteria {
			if _, exists := knownCriteria[criterion]; !exists {
				return ModelScore{}, fmt.Errorf("check %q references unknown criterion %q", check.ID, criterion)
			}
			criterionChecks[criterion] = append(criterionChecks[criterion], check)
		}
		if check.HardGate != "" {
			if _, exists := knownGates[check.HardGate]; !exists {
				return ModelScore{}, fmt.Errorf("check %q references unknown hard gate %q", check.ID, check.HardGate)
			}
			if !check.Passed {
				failedGates[check.HardGate] = true
			}
		}
	}

	for _, criterion := range model.Criteria {
		references := criterionChecks[criterion.ID]
		passed := len(references) > 0
		for _, check := range references {
			if !check.Passed {
				passed = false
				break
			}
		}
		result.Criteria = append(result.Criteria, CriterionResult{
			ID: criterion.ID, Points: criterion.Points, Passed: passed,
		})
		if passed {
			result.RawScore += criterion.Points
		}
	}
	result.CappedScore = result.RawScore
	for _, gate := range model.HardGates {
		if !failedGates[gate.ID] {
			continue
		}
		result.HardGateFailures = append(result.HardGateFailures, gate.ID)
		if result.CappedScore > gate.ScoreCap {
			result.CappedScore = gate.ScoreCap
		}
	}
	for _, level := range model.Levels {
		if result.CappedScore < level.MinimumScore || result.CappedScore > level.MaximumScore {
			continue
		}
		if level.RequiresNoHardGateFailures && len(result.HardGateFailures) > 0 {
			continue
		}
		result.Level = level.ID
		break
	}
	return result, nil
}

func validateScoringModel(model ScoringModel) error {
	if model.ModelID == "" {
		return errors.New("conformance scoring model ID is required")
	}
	if model.CriterionRule != "" && model.CriterionRule != "all_assertions" {
		return fmt.Errorf("unsupported conformance criterion rule %q", model.CriterionRule)
	}
	criterionIDs := make(map[string]struct{}, len(model.Criteria))
	var points float64
	for _, criterion := range model.Criteria {
		if criterion.ID == "" || criterion.Points < 0 {
			return errors.New("conformance scoring criteria require an ID and non-negative points")
		}
		if _, exists := criterionIDs[criterion.ID]; exists {
			return fmt.Errorf("duplicate conformance criterion %q", criterion.ID)
		}
		criterionIDs[criterion.ID] = struct{}{}
		points += criterion.Points
	}
	if model.MaximumScore < 0 || math.Abs(points-model.MaximumScore) > 0.000001 {
		return fmt.Errorf("conformance scoring criteria total %.1f does not equal maximum %.1f", points, model.MaximumScore)
	}
	gateIDs := make(map[string]struct{}, len(model.HardGates))
	for _, gate := range model.HardGates {
		if gate.ID == "" || gate.ScoreCap < 0 || gate.ScoreCap > model.MaximumScore {
			return fmt.Errorf("invalid conformance hard gate %q", gate.ID)
		}
		if _, exists := gateIDs[gate.ID]; exists {
			return fmt.Errorf("duplicate conformance hard gate %q", gate.ID)
		}
		gateIDs[gate.ID] = struct{}{}
	}
	levelIDs := make(map[string]struct{}, len(model.Levels))
	for _, level := range model.Levels {
		if level.ID == "" || level.MinimumScore < 0 || level.MaximumScore < level.MinimumScore || level.MaximumScore > model.MaximumScore {
			return fmt.Errorf("invalid conformance level %q", level.ID)
		}
		if _, exists := levelIDs[level.ID]; exists {
			return fmt.Errorf("duplicate conformance level %q", level.ID)
		}
		levelIDs[level.ID] = struct{}{}
	}
	return nil
}

func criterionIDs(model ScoringModel) map[string]struct{} {
	result := make(map[string]struct{}, len(model.Criteria))
	for _, criterion := range model.Criteria {
		result[criterion.ID] = struct{}{}
	}
	return result
}

func hardGateIDs(model ScoringModel) map[string]struct{} {
	result := make(map[string]struct{}, len(model.HardGates))
	for _, gate := range model.HardGates {
		result[gate.ID] = struct{}{}
	}
	return result
}
