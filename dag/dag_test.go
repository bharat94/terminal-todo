package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDAG_BuildFromTasks(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 0},
		2: {ID: 2, Depends: []uint64{1}, Status: 0},
		3: {ID: 3, Depends: []uint64{2}, Status: 0},
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
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 2},
		2: {ID: 2, Depends: []uint64{1}, Status: 0},
		3: {ID: 3, Depends: []uint64{1, 2}, Status: 0},
	}

	ready := d.GetReadyTasks(tasks)
	assert.Equal(t, 1, len(ready))
	assert.Equal(t, uint64(2), ready[0].ID)
}

func TestDAG_DetectCycle_NoCycle(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 0},
		2: {ID: 2, Depends: []uint64{1}, Status: 0},
	}
	d.BuildFromTasks(tasks)

	err := d.DetectCycle([]uint64{2}, 3)
	assert.NoError(t, err)
}

func TestDAG_GetReadyTasks_AllReady(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 2},
		2: {ID: 2, Depends: []uint64{1}, Status: 2},
		3: {ID: 3, Depends: []uint64{2}, Status: 2},
	}

	ready := d.GetReadyTasks(tasks)
	assert.Equal(t, 0, len(ready))
}

func TestDAG_GetReadyTasks_NonePending(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 2},
		2: {ID: 2, Depends: nil, Status: 2},
	}

	ready := d.GetReadyTasks(tasks)
	assert.Equal(t, 0, len(ready))
}

func TestDAG_GetReadyTasks_NoDeps(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{
		1: {ID: 1, Depends: nil, Status: 0},
		2: {ID: 2, Depends: nil, Status: 0},
	}

	ready := d.GetReadyTasks(tasks)
	assert.Equal(t, 2, len(ready))
}

func TestDAG_DetectCycle_AddingTaskDependsOnSelf(t *testing.T) {
	d := NewDAG()
	tasks := map[uint64]*Task{}
	d.BuildFromTasks(tasks)

	err := d.DetectCycle([]uint64{1}, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}
