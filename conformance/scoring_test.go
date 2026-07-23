package conformance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGradeRequiresAllAssertionsAndAppliesHardGateCap(t *testing.T) {
	model := ScoringModel{
		ModelID:       "test-v1",
		MaximumScore:  100,
		CriterionRule: "all_assertions",
		Criteria: []Criterion{
			{ID: "discovery", Points: 40},
			{ID: "safety", Points: 60},
		},
		HardGates: []HardGate{
			{ID: "unsafe", ScoreCap: 49},
		},
		Levels: []Level{
			{ID: "conformant", MinimumScore: 90, MaximumScore: 100, RequiresNoHardGateFailures: true},
			{ID: "non_conformant", MinimumScore: 0, MaximumScore: 89},
		},
	}
	checks := []CheckResult{
		{ID: "discover-a", Passed: true, Criteria: []string{"discovery"}},
		{ID: "discover-b", Passed: true, Criteria: []string{"discovery"}},
		{ID: "safe-a", Passed: true, Criteria: []string{"safety"}},
		{ID: "safe-b", Passed: false, Criteria: []string{"safety"}, HardGate: "unsafe"},
	}

	score, err := Grade(model, checks)
	require.NoError(t, err)
	assert.Equal(t, 40.0, score.RawScore)
	assert.Equal(t, 40.0, score.CappedScore)
	assert.Equal(t, "non_conformant", score.Level)
	assert.Equal(t, []string{"unsafe"}, score.HardGateFailures)
	assert.True(t, score.Criteria[0].Passed)
	assert.False(t, score.Criteria[1].Passed)

	checks[3].Passed = true
	score, err = Grade(model, checks)
	require.NoError(t, err)
	assert.Equal(t, 100.0, score.RawScore)
	assert.Equal(t, 100.0, score.CappedScore)
	assert.Equal(t, "conformant", score.Level)
	assert.Empty(t, score.HardGateFailures)
}

func TestGradeRejectsUnknownReferences(t *testing.T) {
	model := ScoringModel{
		ModelID:      "test-v1",
		MaximumScore: 1,
		Criteria:     []Criterion{{ID: "known", Points: 1}},
	}
	_, err := Grade(model, []CheckResult{{
		ID: "bad", Passed: true, Criteria: []string{"missing"},
	}})
	assert.ErrorContains(t, err, "unknown criterion")
}
