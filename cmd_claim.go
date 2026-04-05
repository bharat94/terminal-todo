package main

import (
	"fmt"
	"os"
	"terminal-todo/store"
	"time"
)

func cmdClaim(args []string) {
	ids := parseIDs(args)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task ID required")
		os.Exit(1)
	}

	var owner string
	var ttl time.Duration = 15 * time.Minute

	for i, arg := range args {
		if arg == "--as" && i+1 < len(args) {
			owner = args[i+1]
		}
		if arg == "--ttl" && i+1 < len(args) {
			t, err := time.ParseDuration(args[i+1])
			if err == nil {
				ttl = t
			}
		}
	}

	if owner == "" {
		fmt.Fprintln(os.Stderr, "Error: --as <owner> is required")
		os.Exit(1)
	}

	s := loadStore()
	id := ids[0]
	task, ok := s.GetTask(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: task %d not found\n", id)
		os.Exit(1)
	}

	now := uint64(time.Now().UnixMilli())
	if task.Owner != "" && task.Owner != owner && task.LeaseExpires > now {
		fmt.Fprintf(os.Stderr, "Error: task %d already claimed by %s (expires in %s)\n", 
			id, task.Owner, time.Duration(task.LeaseExpires-now)*time.Millisecond)
		os.Exit(1)
	}

	task.Owner = owner
	task.Status = store.StatusInProgress
	task.LeaseExpires = now + uint64(ttl.Milliseconds())
	
	saveStore(s)
	fmt.Printf("Task %d claimed by %s (expires in %s)\n", id, owner, ttl)
}
