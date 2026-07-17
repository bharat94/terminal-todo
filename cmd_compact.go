package main

import (
	"fmt"
	"strconv"
	"time"

	"terminal-todo/store"
)

type compactOptions struct {
	KeepEvents     *int
	ReceiptsBefore time.Duration
	DryRun         bool
}

type compactResult struct {
	EventsRemoved   int  `json:"events_removed"`
	ReceiptsRemoved int  `json:"receipts_removed"`
	DryRun          bool `json:"dry_run"`
}

func cmdCompact(args []string) {
	options, err := parseCompactOptions(args)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	var result compactResult
	if options.DryRun {
		s, err := store.Load(tasksBinPath())
		if err != nil {
			fail(ErrStoreCorrupted, "loading store: %v", err)
		}
		result = compactTaskStore(s, options, false, time.Now())
	} else {
		updateStore(func(s *store.TaskStore) error {
			result = compactTaskStore(s, options, true, time.Now())
			return nil
		})
	}

	mode := "Removed"
	if options.DryRun {
		mode = "Would remove"
	}
	fmt.Printf("%s %d event(s) and %d completed-task acquisition receipt(s)\n",
		mode, result.EventsRemoved, result.ReceiptsRemoved)
}

func parseCompactOptions(args []string) (compactOptions, error) {
	options := compactOptions{DryRun: hasFlag(args, "--dry-run")}
	if raw := optionValue(args, "--keep-events"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return options, fmt.Errorf("--keep-events must be a non-negative integer")
		}
		options.KeepEvents = &value
	}
	if raw := optionValue(args, "--receipts-before"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return options, fmt.Errorf("--receipts-before must be a positive duration such as 2160h")
		}
		options.ReceiptsBefore = value
	}
	if options.KeepEvents == nil && options.ReceiptsBefore == 0 {
		return options, fmt.Errorf("set --keep-events, --receipts-before, or both")
	}
	return options, nil
}

func compactTaskStore(s *store.TaskStore, options compactOptions, apply bool, now time.Time) compactResult {
	result := compactResult{DryRun: options.DryRun}
	if options.KeepEvents != nil && len(s.Events) > *options.KeepEvents {
		result.EventsRemoved = len(s.Events) - *options.KeepEvents
		if apply {
			s.Events = append([]store.Event(nil), s.Events[result.EventsRemoved:]...)
		}
	}

	if options.ReceiptsBefore > 0 {
		cutoff := uint64(now.Add(-options.ReceiptsBefore).UnixMilli())
		for requestID, receipt := range s.Acquisitions {
			if receipt.Created >= cutoff {
				continue
			}
			task, exists := s.Tasks[receipt.Task.ID]
			if exists && task.Status != store.StatusCompleted {
				continue
			}
			result.ReceiptsRemoved++
			if apply {
				delete(s.Acquisitions, requestID)
			}
		}
	}
	return result
}
