package main

import (
	"fmt"
	"sort"

	"terminal-todo/dag"
	"terminal-todo/store"
)

func cmdGraph(args []string) {
	s := loadStore()

	if hasFlag(args, "--dot") {
		renderDOT(s.Tasks)
		return
	}

	if hasFlag(args, "--json") {
		type graphEdge struct {
			From uint64 `json:"from"`
			To   uint64 `json:"to"`
		}
		type graphNode struct {
			ID     uint64 `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		var nodes []graphNode
		var edges []graphEdge
		for _, t := range s.Tasks {
			nodes = append(nodes, graphNode{
				ID:     t.ID,
				Title:  t.Title,
				Status: statusName(t.Status),
			})
			for _, dep := range t.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local {
					edges = append(edges, graphEdge{From: depID, To: t.ID})
				}
			}
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
		writeJSON(map[string]interface{}{
			"schema_version": protocolVersion,
			"nodes":          nodes,
			"edges":          edges,
		})
		return
	}

	// Default: text-based topological overview
	d := dag.NewDAG()
	d.BuildFromTasks(s.Tasks)

	fmt.Println("Task Dependency Graph:")
	fmt.Println()

	// Print each task with its dependents
	for _, t := range sortedTasks(s.Tasks) {
		deps := ""
		if len(t.Depends) > 0 {
			var depIDs []string
			for _, dep := range t.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local {
					depIDs = append(depIDs, fmt.Sprintf("%d", depID))
				} else {
					depIDs = append(depIDs, dep)
				}
			}
			deps = " ← " + joinStrings(depIDs, ", ")
		}

		dependents := ""
		var depOf []uint64
		for _, other := range s.Tasks {
			for _, dep := range other.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local && depID == t.ID {
					depOf = append(depOf, other.ID)
				}
			}
		}
		if len(depOf) > 0 {
			depOfStrs := make([]string, len(depOf))
			for i, id := range depOf {
				depOfStrs[i] = fmt.Sprintf("%d", id)
			}
			dependents = " → " + joinStrings(depOfStrs, ", ")
		}

		fmt.Printf("  %d %s%s%s\n", t.ID, t.Title, deps, dependents)
	}
}

func renderDOT(tasks map[uint64]*store.Task) {
	fmt.Println("digraph tasks {")
	fmt.Println("  rankdir=LR;")
	fmt.Println("  node [shape=box, style=rounded];")

	for _, t := range sortedTasks(tasks) {
		color := "white"
		switch t.Status {
		case store.StatusCompleted:
			color = "palegreen"
		case store.StatusInProgress:
			color = "lightyellow"
		case store.StatusBlocked:
			color = "mistyrose"
		}
		label := fmt.Sprintf("%d\\n%s", t.ID, escapeDOT(t.Title))
		fmt.Printf("  %d [label=\"%s\", fillcolor=%s, style=filled];\n", t.ID, label, color)
	}

	for _, t := range sortedTasks(tasks) {
		for _, dep := range t.Depends {
			depID, local := dag.ParseLocalID(dep)
			if local {
				fmt.Printf("  %d -> %d;\n", depID, t.ID)
			} else {
				fmt.Printf("  \"%s\" -> %d [style=dashed, color=gray];\n", dep, t.ID)
			}
		}
	}

	fmt.Println("}")
}

func escapeDOT(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		case '\n':
			result += "\\n"
		default:
			result += string(c)
		}
	}
	return result
}

func sortedTasks(tasks map[uint64]*store.Task) []*store.Task {
	result := make([]*store.Task, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
