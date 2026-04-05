package main

import (
	"fmt"
	"os"
)

func cmdRm(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: task ID required\n")
		os.Exit(1)
	}

	var id uint64
	if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid task ID: %s\n", args[0])
		os.Exit(1)
	}

	s := loadStore()
	if !s.RemoveTask(id) {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
		os.Exit(1)
	}
	saveStore(s)

	fmt.Printf("Removed task %d\n", id)
}
