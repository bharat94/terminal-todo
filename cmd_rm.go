package main

import (
	"fmt"
	"os"
)

func cmdRm(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	s := loadStore()
	for _, id := range ids {
		if s.RemoveTask(id) {
			fmt.Printf("Removed task %d\n", id)
		} else {
			fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
		}
	}
	saveStore(s)
}
