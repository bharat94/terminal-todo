package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func cmdEvents(args []string) {
	since, limit, err := parseEventCLIOptions(args)
	if err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	s := loadStore()
	if limit == nil {
		events := s.EventsSince(since)
		protocolEvents := make([]protocolEvent, len(events))
		for i, event := range events {
			protocolEvents[i] = newProtocolEvent(event)
		}
		if hasFlag(args, "--json") {
			writeJSON(map[string]interface{}{
				"schema_version": protocolVersion,
				"events":         protocolEvents,
			})
			return
		}
		renderEventTable(protocolEvents, since)
		return
	}

	page := buildEventPage(s, since, *limit)
	if hasFlag(args, "--json") {
		writeJSON(page)
		return
	}
	if page.Cursor.HistoryGap {
		fmt.Printf(
			"Warning: event IDs %d-%d are no longer retained; inspect current status before resuming.\n",
			page.Cursor.RetentionGap.From,
			page.Cursor.RetentionGap.Through,
		)
	}
	renderEventTable(page.Events, since)
	if page.Cursor.HasMore {
		fmt.Printf(
			"More events are available; continue with `todo events %d --limit %d`.\n",
			page.Cursor.NextSince,
			page.Limit,
		)
	}
}

func renderEventTable(events []protocolEvent, since uint64) {
	if len(events) == 0 {
		if since > 0 {
			fmt.Println("No events since ID", since)
		} else {
			fmt.Println("No events recorded")
		}
		return
	}

	fmt.Printf("%-4s %-25s %-14s %-6s %-16s %s\n", "ID", "TIMESTAMP", "TYPE", "TASK", "ACTOR", "DATA")
	for _, event := range events {
		data := make([]string, 0, len(event.Data))
		for key, value := range event.Data {
			data = append(data, fmt.Sprintf("%s=%s", key, value))
		}
		sort.Strings(data)
		timestamp, parseErr := time.Parse(time.RFC3339Nano, event.Timestamp)
		if parseErr != nil {
			timestamp = time.Time{}
		}
		fmt.Printf(
			"%-4d %-25s %-14s %-6d %-16s %s\n",
			event.ID,
			timestamp.Local().Format(time.RFC3339),
			string(event.Type),
			event.TaskID,
			event.Actor,
			strings.Join(data, ", "),
		)
	}
}

func parseEventCLIOptions(args []string) (uint64, *int, error) {
	var limit *int
	if raw := optionValue(args, "--limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return 0, nil, fmt.Errorf("--limit must be an integer")
		}
		validated, err := eventPageLimit(&parsed)
		if err != nil {
			return 0, nil, fmt.Errorf("--%v", err)
		}
		limit = &validated
	}

	var (
		since    uint64
		sinceSet bool
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			i++
			continue
		case "--json":
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if sinceSet {
			return 0, nil, fmt.Errorf("events accepts at most one cursor")
		}
		parsed, parseErr := strconv.ParseUint(args[i], 10, 64)
		if parseErr != nil {
			return 0, nil, fmt.Errorf("event cursor must be a non-negative integer")
		}
		since = parsed
		sinceSet = true
	}
	return since, limit, nil
}
