package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidPriorityRejectsNonFiniteAndOutOfRangeValues(t *testing.T) {
	for _, value := range []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		-0.01,
		1.01,
	} {
		assert.False(t, validPriority(value), "%v", value)
	}
	for _, value := range []float64{0, 0.5, 1} {
		assert.True(t, validPriority(value), "%v", value)
	}
}

func TestLoadConfigRejectsInvalidPersistedValues(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()
	assert.NoError(t, os.Mkdir(filepath.Join(projectRoot, ".terminal-todo"), 0700))

	for _, contents := range []string{
		`{"default_ttl":"0s","default_priority":0.5}`,
		`{"default_ttl":"15m","default_priority":1e40}`,
	} {
		assert.NoError(t, os.WriteFile(configPath(), []byte(contents), 0600))
		_, err := loadConfig()
		assert.ErrorContains(t, err, "invalid config")
	}
}
