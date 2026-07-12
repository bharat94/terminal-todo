package main

import (
	"fmt"
	"strconv"
	"time"
)

func cmdEvents(args []string) {
	s := loadStore()

	var since uint64
	if len(args) > 0 {
		if id, err := strconv.ParseUint(args[0], 10, 64); err == nil && id > 0 {
			since = id
		}
	}

	events := s.EventsSince(since)

	if hasFlag(args, "--json") {
		protocolEvents := make([]protocolEvent, len(events))
		for i, e := range events {
			protocolEvents[i] = newProtocolEvent(e)
		}
		writeJSON(map[string]interface{}{
			"schema_version": protocolVersion,
			"events":         protocolEvents,
		})
		return
	}

	if len(events) == 0 {
		if since > 0 {
			fmt.Println("No events since ID", since)
		} else {
			fmt.Println("No events recorded")
		}
		return
	}

	fmt.Printf("%-4s %-25s %-14s %-6s %-16s %s\n", "ID", "TIMESTAMP", "TYPE", "TASK", "ACTOR", "DATA")
	for _, e := range events {
		ts := time.UnixMilli(int64(e.Timestamp)).Format(time.RFC3339)
		dataStr := ""
		first := true
		for k, v := range e.Data {
			if !first {
				dataStr += ", "
			}
			dataStr += fmt.Sprintf("%s=%s", k, v)
			first = false
		}
		fmt.Printf("%-4d %-25s %-14s %-6d %-16s %s\n", e.ID, ts, string(e.Type), e.TaskID, e.Actor, dataStr)
	}
}
