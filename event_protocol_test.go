package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bharat94/terminal-todo/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventPagesAdvanceWithoutDuplicatesOrLoss(t *testing.T) {
	s := eventTestStore(5)

	var received []uint64
	since := uint64(0)
	for {
		page := buildEventPage(s, since, 2)
		assert.Equal(t, protocolVersion, page.SchemaVersion)
		assert.LessOrEqual(t, page.Returned, 2)
		assert.Equal(t, uint64(5), page.Cursor.LatestEventID)
		for _, event := range page.Events {
			received = append(received, event.ID)
		}
		if !page.Cursor.HasMore {
			break
		}
		require.Greater(t, page.Cursor.NextSince, since)
		since = page.Cursor.NextSince
	}

	assert.Equal(t, []uint64{1, 2, 3, 4, 5}, received)
}

func TestEventPageReportsCompactedHistoryGap(t *testing.T) {
	s := eventTestStore(5)
	s.Events = append([]store.Event(nil), s.Events[2:]...)

	page := buildEventPage(s, 0, 2)

	assert.True(t, page.Cursor.HistoryGap)
	assert.Equal(t, uint64(3), page.Cursor.OldestAvailable)
	assert.Equal(t, uint64(5), page.Cursor.LatestEventID)
	assert.Equal(t, &eventRetentionGap{From: 1, Through: 2}, page.Cursor.RetentionGap)
	assert.Equal(t, []uint64{3, 4}, eventIDs(page.Events))
	assert.True(t, page.Cursor.HasMore)
	assert.Equal(t, uint64(4), page.Cursor.NextSince)
}

func TestNativeAndMCPEventsKeepLegacyDefaultAndOfferPageOptIn(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	s := eventTestStore(3)
	require.NoError(t, s.Save(filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")))
	backend := &server{initialized: true}

	legacy, rpcErr := backend.dispatch("todo.events", json.RawMessage(`{"since":0}`))
	require.Nil(t, rpcErr)
	legacyMap := legacy.(map[string]interface{})
	assert.Len(t, legacyMap, 1)
	assert.Len(t, legacyMap["events"].([]store.Event), 3)

	pageResult, rpcErr := backend.dispatch(
		"todo.events",
		json.RawMessage(`{"since":0,"page":true,"limit":2}`),
	)
	require.Nil(t, rpcErr)
	nativePage := pageResult.(eventsEnvelope)
	assert.Equal(t, []uint64{1, 2}, eventIDs(nativePage.Events))

	mcp := &mcpServer{
		backend:        backend,
		initializeSeen: true,
		initialized:    true,
	}
	mcpLegacy, rpcErr := mcp.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_events",
		"arguments":{"since":0}
	}`))
	require.Nil(t, rpcErr)
	legacyCall := mcpLegacy.(mcpCallResult)
	require.False(t, legacyCall.IsError)
	assert.Equal(t, legacy, legacyCall.StructuredContent)

	mcpPaged, rpcErr := mcp.dispatch("tools/call", json.RawMessage(`{
		"name":"terminal_todo_events",
		"arguments":{"since":0,"page":true,"limit":2}
	}`))
	require.Nil(t, rpcErr)
	pagedCall := mcpPaged.(mcpCallResult)
	require.False(t, pagedCall.IsError)
	assert.Equal(t, nativePage, pagedCall.StructuredContent)
}

func TestEventPageParametersAreStrictAndBounded(t *testing.T) {
	oldRoot := projectRoot
	projectRoot = t.TempDir()
	defer func() { projectRoot = oldRoot }()

	require.NoError(t, eventTestStore(1).Save(filepath.Join(projectRoot, ".terminal-todo", "tasks.bin")))
	backend := &server{initialized: true}

	for _, params := range []string{
		`{"limit":1}`,
		`{"page":true,"limit":0}`,
		`{"page":true,"limit":1001}`,
		`{"page":true,"unknown":1}`,
	} {
		result, rpcErr := backend.dispatch("todo.events", json.RawMessage(params))
		assert.Nil(t, result, params)
		require.NotNil(t, rpcErr, params)
		assert.Equal(t, rpcInvalidParams, rpcErr.Code, params)
	}
}

func TestMCPEventsAdvertisesExplicitPageOptIn(t *testing.T) {
	var eventsTool mcpTool
	for _, tool := range terminalTodoMCPTools() {
		if tool.Name == "terminal_todo_events" {
			eventsTool = tool
			break
		}
	}
	require.Equal(t, "terminal_todo_events", eventsTool.Name)
	properties := eventsTool.InputSchema["properties"].(map[string]interface{})
	assert.Equal(t, "boolean", properties["page"].(map[string]interface{})["type"])
	assert.Equal(t, maxEventPageLimit, properties["limit"].(map[string]interface{})["maximum"])
}

func TestCLIEventsLegacyJSONIsUnchangedUntilLimitIsRequested(t *testing.T) {
	root := t.TempDir()
	todo := buildTodo(t)
	runLifecycleCommand(t, todo, root, "init")
	runLifecycleCommand(t, todo, root, "add", "First")
	runLifecycleCommand(t, todo, root, "add", "Second")

	var legacy map[string]json.RawMessage
	runReceiptJSONCommand(t, todo, root, &legacy, "events", "--json")
	assert.ElementsMatch(t, []string{"events", "schema_version"}, mapKeys(legacy))

	var page eventsEnvelope
	runReceiptJSONCommand(t, todo, root, &page, "events", "0", "--limit", "1", "--json")
	assert.Equal(t, 1, page.Limit)
	assert.Equal(t, 1, page.Returned)
	assert.True(t, page.Cursor.HasMore)
	assert.False(t, page.Cursor.HistoryGap)
}

func eventTestStore(count int) *store.TaskStore {
	s := store.NewTaskStore()
	for i := 0; i < count; i++ {
		s.AddEvent(store.EventTaskUpdated, uint64(i+1), "event-test", map[string]string{"n": "x"})
	}
	return s
}

func eventIDs(events []protocolEvent) []uint64 {
	ids := make([]uint64, len(events))
	for i, event := range events {
		ids[i] = event.ID
	}
	return ids
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
