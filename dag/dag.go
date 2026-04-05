package dag

import "fmt"

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

func (d *DAG) BuildFromTasks(tasks map[uint64]*Task) {
	d.adjacency = make(map[uint64][]uint64)
	for _, task := range tasks {
		for _, dep := range task.Depends {
			d.adjacency[dep] = append(d.adjacency[dep], task.ID)
		}
	}
}

func (d *DAG) DetectCycle(newDeps []uint64, newTaskID uint64) error {
	adj := make(map[uint64][]uint64)
	for from, tos := range d.adjacency {
		adj[from] = append([]uint64{}, tos...)
	}

	for _, dep := range newDeps {
		adj[dep] = append(adj[dep], newTaskID)
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

type Task struct {
	ID      uint64
	Title   string
	Depends []uint64
	Status  uint8
}

func (d *DAG) GetReadyTasks(tasks map[uint64]*Task) []*Task {
	var ready []*Task

	for _, task := range tasks {
		if task.Status == 2 {
			continue
		}

		allDepsDone := true
		for _, dep := range task.Depends {
			depTask, ok := tasks[dep]
			if !ok || depTask.Status != 2 {
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

func (d *DAG) GetBlockedTasks(tasks map[uint64]*Task) map[uint64][]uint64 {
	blocked := make(map[uint64][]uint64)

	for _, task := range tasks {
		if task.Status == 2 {
			continue
		}

		for _, dep := range task.Depends {
			depTask, ok := tasks[dep]
			if !ok || depTask.Status != 2 {
				blocked[task.ID] = append(blocked[task.ID], dep)
				break
			}
		}
	}

	return blocked
}
