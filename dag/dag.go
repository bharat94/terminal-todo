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

type DependencyResolver func(uri string) bool

// ParseTaskURI validates a canonical todo://repository/id reference. Repository
// aliases may contain letters, digits, dots, underscores, and hyphens.
func ParseTaskURI(uri string) (string, uint64, error) {
	if !strings.HasPrefix(uri, "todo://") {
		return "", 0, fmt.Errorf("dependency %q must be a task ID or todo://repository/id URI", uri)
	}
	parts := strings.Split(strings.TrimPrefix(uri, "todo://"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", 0, fmt.Errorf("invalid task URI %q", uri)
	}
	for _, r := range parts[0] {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return "", 0, fmt.Errorf("invalid repository alias in task URI %q", uri)
		}
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || id == 0 {
		return "", 0, fmt.Errorf("invalid task ID in URI %q", uri)
	}
	return parts[0], id, nil
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
		repository, id, err := ParseTaskURI(uri)
		if err == nil && repository == "local" {
			return id, true
		}
	}
	// Also support plain integer strings as local for backward compatibility/simplicity
	id, err := strconv.ParseUint(uri, 10, 64)
	if err == nil && id > 0 {
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
	return d.GetReadyTasksWithResolver(tasks, nil)
}

func (d *DAG) GetReadyTasksWithResolver(tasks map[uint64]*store.Task, resolver DependencyResolver) []*store.Task {
	var ready []*store.Task

	for _, task := range tasks {
		if task.Status != store.StatusPending {
			continue
		}

		if DependenciesCompleteWithResolver(task, tasks, resolver) {
			ready = append(ready, task)
		}
	}

	return ready
}

// DependenciesComplete reports whether every dependency is a completed local
// task. Cross-repository dependencies remain blocked until a resolver exists.
func DependenciesComplete(task *store.Task, tasks map[uint64]*store.Task) bool {
	return DependenciesCompleteWithResolver(task, tasks, nil)
}

func DependenciesCompleteWithResolver(task *store.Task, tasks map[uint64]*store.Task, resolver DependencyResolver) bool {
	for _, depURI := range task.Depends {
		depID, local := ParseLocalID(depURI)
		if !local {
			if resolver == nil || !resolver(depURI) {
				return false
			}
			continue
		}
		depTask, ok := tasks[depID]
		if !ok || depTask.Status != store.StatusCompleted {
			return false
		}
	}
	return true
}

func (d *DAG) GetBlockedTasks(tasks map[uint64]*store.Task) map[uint64][]string {
	return d.GetBlockedTasksWithResolver(tasks, nil)
}

func (d *DAG) GetBlockedTasksWithResolver(tasks map[uint64]*store.Task, resolver DependencyResolver) map[uint64][]string {
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
				if resolver == nil || !resolver(depURI) {
					blocked[task.ID] = append(blocked[task.ID], depURI)
				}
			}
		}
	}

	return blocked
}
