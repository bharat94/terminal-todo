package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/bharat94/terminal-todo/store"
)

const (
	defaultEventPageLimit = 100
	maxEventPageLimit     = 1000
)

// eventsParams keeps the native todo.events response backward compatible.
// Callers opt into the versioned, bounded protocol envelope with page=true.
type eventsParams struct {
	Since uint64 `json:"since,omitempty"`
	Limit *int   `json:"limit,omitempty"`
	Page  bool   `json:"page,omitempty"`
}

type eventRetentionGap struct {
	From    uint64 `json:"from"`
	Through uint64 `json:"through"`
}

type eventCursor struct {
	RequestedSince  uint64             `json:"requested_since"`
	NextSince       uint64             `json:"next_since"`
	OldestAvailable uint64             `json:"oldest_available"`
	LatestEventID   uint64             `json:"latest_event_id"`
	HasMore         bool               `json:"has_more"`
	HistoryGap      bool               `json:"history_gap"`
	RetentionGap    *eventRetentionGap `json:"retention_gap,omitempty"`
}

type eventsEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Events        []protocolEvent `json:"events"`
	Limit         int             `json:"limit"`
	Returned      int             `json:"returned"`
	Cursor        eventCursor     `json:"cursor"`
}

func eventPageLimit(value *int) (int, error) {
	if value == nil {
		return defaultEventPageLimit, nil
	}
	if *value < 1 || *value > maxEventPageLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxEventPageLimit)
	}
	return *value, nil
}

func buildEventPage(s *store.TaskStore, after uint64, limit int) eventsEnvelope {
	events := append([]store.Event(nil), s.Events...)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
	})

	cursor := eventCursor{
		RequestedSince: after,
		NextSince:      after,
	}
	if len(events) > 0 {
		cursor.OldestAvailable = events[0].ID
	}
	if s.NextEventID > 0 {
		cursor.LatestEventID = s.NextEventID - 1
	}

	start := sort.Search(len(events), func(i int) bool {
		return events[i].ID > after
	})
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	selected := events[start:end]
	cursor.HasMore = end < len(events)
	if len(selected) > 0 {
		cursor.NextSince = selected[len(selected)-1].ID
	}

	if after < math.MaxUint64 {
		missingFrom := after + 1
		switch {
		case start < len(events) && events[start].ID > missingFrom:
			cursor.RetentionGap = &eventRetentionGap{
				From:    missingFrom,
				Through: events[start].ID - 1,
			}
		case start == len(events) && cursor.LatestEventID >= missingFrom:
			cursor.RetentionGap = &eventRetentionGap{
				From:    missingFrom,
				Through: cursor.LatestEventID,
			}
		}
	}
	cursor.HistoryGap = cursor.RetentionGap != nil

	protocolEvents := make([]protocolEvent, len(selected))
	for i, event := range selected {
		protocolEvents[i] = newProtocolEvent(event)
	}

	return eventsEnvelope{
		SchemaVersion: protocolVersion,
		Events:        protocolEvents,
		Limit:         limit,
		Returned:      len(protocolEvents),
		Cursor:        cursor,
	}
}
