package main

import (
	"testing"

	"github.com/bharat94/terminal-todo/store"

	"github.com/stretchr/testify/assert"
)

func TestNewProtocolTaskUsesStableAgentTypes(t *testing.T) {
	task := &store.Task{
		ID: 7, Title: "Coordinate agents", Status: store.StatusInProgress,
		Created: 0, LeaseExpires: 1_000, Priority: 0.75, BlockReason: "approval",
	}

	got := newProtocolTask(task)

	assert.Equal(t, "in_progress", got.Status)
	assert.Equal(t, "1970-01-01T00:00:00Z", got.Created)
	assert.NotNil(t, got.Metadata.LeaseExpires)
	assert.Equal(t, "1970-01-01T00:00:01Z", *got.Metadata.LeaseExpires)
	assert.Empty(t, got.Depends)
	assert.Empty(t, got.Metadata.Capabilities)
	assert.NotNil(t, got.Metadata.Extra)
	assert.Equal(t, "approval", got.Metadata.BlockReason)
}

func TestNewBlockedSummaryIsDeterministicAndCountsExplicitBlocks(t *testing.T) {
	tasks := map[uint64]*store.Task{
		1: {ID: 1, Status: store.StatusPending},
		2: {ID: 2, Status: store.StatusPending, Depends: []string{"todo://local/1"}},
		3: {ID: 3, Status: store.StatusBlocked},
		4: {ID: 4, Status: store.StatusPending, Depends: []string{"todo://remote/9", "todo://local/1"}},
	}

	got := newBlockedSummary(tasks)

	assert.Equal(t, 3, got.Count)
	assert.Equal(t, []string{"todo://local/1", "todo://remote/9"}, got.PrimaryBlockers)
}
