package main

import (
	"fmt"
	"strings"
	"time"

	"terminal-todo/store"
)

func cmdWatch(args []string) {
	ids := parseIDs(args)
	pollStr := optionValue(args, "--poll")
	pollInterval := 2 * time.Second
	if pollStr != "" {
		if d, err := time.ParseDuration(pollStr); err == nil && d > 0 {
			pollInterval = d
		}
	}

	if len(ids) > 0 {
		watchTask(ids[0], pollInterval)
	} else {
		watchDashboard(pollInterval)
	}
}

func watchTask(id uint64, interval time.Duration) {
	lastStatus := ""
	lastLogCount := 0

	for {
		s := loadStore()
		task, ok := s.GetTask(id)
		if !ok {
			fail(ErrTaskNotFound, "Task %d not found", id)
		}

		statusStr := statusName(task.Status)
		changed := statusStr != lastStatus || len(task.Log) != lastLogCount

		if changed || lastStatus == "" {
			clearScreen()
			fmt.Printf("Watching task %d — %s\n", id, task.Title)
			fmt.Println(strings.Repeat("─", 50))
			fmt.Printf("Status: %s\n", statusStr)
			if task.Owner != "" {
				fmt.Printf("Owner:  %s\n", task.Owner)
			}
			if len(task.Log) > 0 {
				fmt.Println("\nRecent log:")
				start := 0
				if len(task.Log) > 5 {
					start = len(task.Log) - 5
				}
				for _, entry := range task.Log[start:] {
					fmt.Printf("  [%s] %s: %s\n", formatTimestamp(entry.Timestamp), entry.Agent, entry.Message)
				}
			}
			lastStatus = statusStr
			lastLogCount = len(task.Log)
		}

		if task.Status == store.StatusCompleted {
			fmt.Println("\n✓ Task completed!")
			return
		}

		time.Sleep(interval)
	}
}

func watchDashboard(interval time.Duration) {
	for {
		clearScreen()
		cmdStatus([]string{})
		time.Sleep(interval)
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
