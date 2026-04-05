package dag

import (
	"fmt"
	"strconv"
	"strings"
	"terminal-todo/store"
)

type DAG struct {
	adjacency map[uint64][]uint64
}

func NewDAG() *DAG {
	return &DAG{
		adjacency: make(map[uint64][]uint64),
	}
}

func (d *DAG) AddEdge(from, to uint64) {
	d.adjacency[from] = append(d.adjacency[from], to)
}

func (d *DAG) BuildFromTasks(tasks map[uint64]*store.Task) {
	d.adjacency = make(map[uint64][]uint64)
	for _, task := range tasks {
		for _, depURI := range task.Depends {
			id, local := ParseLocalID(depURI)
			if local {
				d.adjacency[id] = append(d.adjacency[id], task.ID)
			}
		}
	}
}

// ParseLocalID returns the ID and true if it's a local URI (todo://local/1)
func ParseLocalID(uri string) (uint64, bool) {
	if strings.HasPrefix(uri, "todo://local/") {
		idStr := strings.TrimPrefix(uri, "todo://local/")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err == nil {
			return id, true
		}
	}
	// Also support plain integer strings as local for backward compatibility/simplicity
	id, err := strconv.ParseUint(uri, 10, 64)
	if err == nil {
		return id, true
	}
	return 0, false
}

func (d *DAG) DetectCycle(newDeps []string, newTaskID uint64) error {
	adj := make(map[uint64][]uint64)
	for from, tos := range d.adjacency {
		adj[from] = append([]uint64{}, tos...)
	}

	for _, depURI := range newDeps {
		depID, local := ParseLocalID(depURI)
		if local {
			adj[depID] = append(adj[depID], newTaskID)
		}
	}

	visited := make(map[uint64]bool)
	onStack := make(map[uint64]bool)
	var path []uint64

	var dfs func(node uint64) error
	dfs = func(node uint64) error {
		visited[node] = true
		onStack[node] = true
		path = append(path, node)

		for _, next := range adj[node] {
			if !visited[next] {
				if err := dfs(next); err != nil {
					return err
				}
			} else if onStack[next] {
				path = append(path, next)
				return fmt.Errorf("cycle detected: %v", path)
			}
		}

		onStack[node] = false
		path = path[:len(path)-1]
		return nil
	}

	for node := range adj {
		if !visited[node] {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *DAG) GetReadyTasks(tasks map[uint64]*store.Task) []*store.Task {
	var ready []*store.Task

	for _, task := range tasks {
		if task.Status == store.StatusCompleted {
			continue
		}

		allDepsDone := true
		for _, depURI := range task.Depends {
			depID, local := ParseLocalID(depURI)
			if local {
				depTask, ok := tasks[depID]
				if !ok || depTask.Status != store.StatusCompleted {
					allDepsDone = false
					break
				}
			} else {
				// Cross-repo dependency: For now, assume not done if we can't resolve it
				allDepsDone = false
				break
			}
		}

		if allDepsDone {
			ready = append(ready, task)
		}
	}

	return ready
}

func (d *DAG) GetBlockedTasks(tasks map[uint64]*store.Task) map[uint64][]string {
	blocked := make(map[uint64][]string)

	for _, task := range tasks {
		if task.Status == store.StatusCompleted {
			continue
		}

		for _, depURI := range task.Depends {
			depID, local := ParseLocalID(depURI)
			if local {
				depTask, ok := tasks[depID]
				if !ok || depTask.Status != store.StatusCompleted {
					blocked[task.ID] = append(blocked[task.ID], depURI)
				}
			} else {
				// Cross-repo dependency is always considered blocking for now
				blocked[task.ID] = append(blocked[task.ID], depURI)
			}
		}
	}

	return blocked
}
