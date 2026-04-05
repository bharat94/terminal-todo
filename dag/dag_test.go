package dag

import (
	"testing"
	"terminal-todo/store"

	"github.com/stretchr/testify/assert"
)

func TestDAG_BuildFromTasks(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*store.Task{
		1: {ID: 1, Depends: nil, Status: store.StatusPending},
		2: {ID: 2, Depends: []string{"todo://local/1"}, Status: store.StatusPending},
		3: {ID: 3, Depends: []string{"todo://local/2"}, Status: store.StatusPending},
	}

	d.BuildFromTasks(tasks)

	adj := d.adjacency
	assert.Contains(t, adj, uint64(1))
	assert.Contains(t, adj, uint64(2))
	assert.Equal(t, []uint64{2}, adj[1])
	assert.Equal(t, []uint64{3}, adj[2])
}

func TestDAG_GetReadyTasks(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*store.Task{
		1: {ID: 1, Depends: nil, Status: store.StatusCompleted},
		2: {ID: 2, Depends: []string{"todo://local/1"}, Status: store.StatusPending},
		3: {ID: 3, Depends: []string{"todo://local/1", "todo://local/2"}, Status: store.StatusPending},
	}

	ready := d.GetReadyTasks(tasks)
	assert.Equal(t, 1, len(ready))
	assert.Equal(t, uint64(2), ready[0].ID)
}

func TestDAG_DetectCycle_NoCycle(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*store.Task{
		1: {ID: 1, Depends: nil, Status: store.StatusPending},
		2: {ID: 2, Depends: []string{"todo://local/1"}, Status: store.StatusPending},
	}
	d.BuildFromTasks(tasks)

	err := d.DetectCycle([]string{"todo://local/2"}, 3)
	assert.NoError(t, err)
}

func TestDAG_DetectCycle_Cycle(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*store.Task{
		1: {ID: 1, Depends: []string{"todo://local/2"}, Status: store.StatusPending},
		2: {ID: 2, Depends: nil, Status: store.StatusPending},
	}
	d.BuildFromTasks(tasks)

	err := d.DetectCycle([]string{"todo://local/1"}, 2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDAG_ParseLocalID(t *testing.T) {
	id, local := ParseLocalID("todo://local/42")
	assert.True(t, local)
	assert.Equal(t, uint64(42), id)

	id, local = ParseLocalID("42")
	assert.True(t, local)
	assert.Equal(t, uint64(42), id)

	_, local = ParseLocalID("todo://other-repo/42")
	assert.False(t, local)
}
