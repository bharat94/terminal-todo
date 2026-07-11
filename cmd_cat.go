package main

import (
	"encoding/json"
	"fmt"
	"os"
	"terminal-todo/store"
)

func cmdCat(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	s := loadStore()
	task, ok := s.GetTask(ids[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", ids[0])
		os.Exit(1)
	}

	if hasFlag(args, "--json") {
		output, err := json.MarshalIndent(taskEnvelope{SchemaVersion: protocolVersion, Task: newProtocolTask(task)}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
		return
	}

	fmt.Printf("ID:         %d\n", task.ID)
	fmt.Printf("Title:      %s\n", task.Title)
	statusStr := "Pending"
	switch task.Status {
	case store.StatusInProgress:
		statusStr = "In-Progress"
	case store.StatusCompleted:
		statusStr = "Completed"
	case store.StatusBlocked:
		statusStr = "Blocked"
	}
	fmt.Printf("Status:     %s\n", statusStr)
	fmt.Printf("Owner:      %s\n", task.Owner)
	fmt.Printf("Depends:    %v\n", task.Depends)
	fmt.Printf("Caps:       %v\n", task.Capabilities)
	fmt.Printf("Tags:       %v\n", task.Tags)
	fmt.Printf("Priority:   %.2f\n", task.Priority)
	fmt.Printf("Lineage:    %s\n", task.Lineage)
	fmt.Printf("Retries:    %d\n", task.RetryCount)
	if task.LastError != "" {
		fmt.Printf("Last Error: %s\n", task.LastError)
	}
	if len(task.Log) > 0 {
		fmt.Println("Log:")
		start := 0
		if len(task.Log) > 5 {
			start = len(task.Log) - 5
		}
		for _, entry := range task.Log[start:] {
			fmt.Printf("  [%s] %s: %s\n", formatTimestamp(entry.Timestamp), entry.Agent, entry.Message)
		}
	}
}
