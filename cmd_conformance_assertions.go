package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/store"
)

type catalogTaskState struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Owner        string            `json:"owner,omitempty"`
	LeaseExpires uint64            `json:"lease_expires,omitempty"`
	Extra        map[string]string `json:"extra"`
}

func catalogNormalizer(hostName string, runtime catalogFixtureRuntime) conformance.SequenceNormalizer {
	return conformance.SequenceNormalizerFunc(func(_ context.Context, workspace string, turns []conformance.TurnResult) (conformance.Evidence, error) {
		aggregated := conformance.Capture{Stdout: []conformance.Event{}, Stderr: []conformance.Event{}}
		for _, turn := range turns {
			aggregated.BytesRead += turn.Execution.Capture.BytesRead
			aggregated.Stdout = append(aggregated.Stdout, turn.Execution.Capture.Stdout...)
			aggregated.Stderr = append(aggregated.Stderr, turn.Execution.Capture.Stderr...)
		}
		evidence := conformance.EmptyEvidence(aggregated)
		operations, domainErrors, err := conformance.ReadTrace(workspace)
		if err != nil {
			return evidence, err
		}
		evidence.Operations = operations
		evidence.Errors = domainErrors

		taskStore, err := store.Load(filepath.Join(workspace, ".terminal-todo", "tasks.bin"))
		if err != nil {
			return evidence, fmt.Errorf("load catalog task store: %w", err)
		}
		if len(operations) == 0 && len(domainErrors) == 0 && len(taskStore.Events) > len(runtime.TaskIDs) {
			return evidence, fmt.Errorf("conformance operation trace is missing after persisted task mutations")
		}
		refsByID := make(map[string]string, len(runtime.TaskIDs))
		for ref, id := range runtime.TaskIDs {
			refsByID[id] = ref
		}
		for id, task := range taskStore.Tasks {
			idString := strconv.FormatUint(id, 10)
			ref := refsByID[idString]
			if ref == "" {
				ref = "task:" + idString
			}
			evidence.Tasks[ref] = catalogTaskState{
				ID: idString, Status: statusName(task.Status), Owner: task.Owner,
				LeaseExpires: task.LeaseExpires, Extra: cloneStringMap(task.Extra),
			}
		}
		for _, event := range taskStore.Events {
			encoded, err := json.Marshal(event)
			if err != nil {
				return evidence, fmt.Errorf("encode catalog event: %w", err)
			}
			evidence.Events = append(evidence.Events, encoded)
		}
		for _, turn := range turns {
			for _, message := range extractHostAssistantMessages(hostName, turn.Execution.Capture) {
				evidence.AssistantMessages = append(evidence.AssistantMessages, message)
				evidence.AssistantTurns = append(evidence.AssistantTurns, conformance.AssistantMessage{Actor: turn.Actor, Text: message})
			}
		}
		return evidence, nil
	})
}

func compileCatalogAssertions(scenario conformance.CatalogScenario, runtime catalogFixtureRuntime) ([]conformance.Assertion, error) {
	assertions := make([]conformance.Assertion, 0, len(scenario.Assertions))
	for _, fixtureAssertion := range scenario.Assertions {
		fixtureAssertion := fixtureAssertion
		if err := validateCatalogExpectation(fixtureAssertion); err != nil {
			return nil, fmt.Errorf("scenario %q assertion %q: %w", scenario.ID, fixtureAssertion.ID, err)
		}
		assertion := conformance.EvidenceCheck(
			fixtureAssertion.ID, fixtureAssertion.Description, 1, true,
			func(evidence conformance.Evidence) (bool, string) {
				return evaluateCatalogAssertion(fixtureAssertion, scenario, runtime, evidence)
			},
		).WithCriteria(fixtureAssertion.Criteria...)
		if fixtureAssertion.HardGate != "" {
			assertion = assertion.WithHardGate(fixtureAssertion.HardGate)
		}
		assertions = append(assertions, assertion)
	}
	return assertions, nil
}

func validateCatalogExpectation(assertion conformance.CatalogAssertion) error {
	allowed := map[string]map[string]bool{
		"operation_trace": {
			"contains_operation": true, "first_operation": true, "operation_count": true,
			"before": true, "after": true, "excludes_operations": true, "excludes_sequences": true,
			"per_actor_operation": true, "unique_argument": true, "ordered_operations": true,
			"actor": true, "task": true, "operation": true, "result_task": true,
			"update_extra_contains": true, "one_of_operations": true,
		},
		"task_state": {
			"task": true, "owner_count": true, "status": true, "owner": true,
			"lease_after": true, "owned_task_count": true,
		},
		"domain_error": {"code": true, "terminal": true},
		"assistant_output": {
			"actor": true, "maximum_messages": true, "maximum_words": true,
			"semantic_contains": true, "semantic_excludes": true, "excludes_patterns": true,
			"excludes_semantic_intent": true,
		},
		"event_log": {"contains_event": true, "actor": true, "task": true},
	}[assertion.Kind]
	if allowed == nil {
		return fmt.Errorf("unsupported assertion kind %q", assertion.Kind)
	}
	for key := range assertion.Expect {
		if !allowed[key] {
			return fmt.Errorf("unsupported %s expectation %q", assertion.Kind, key)
		}
	}
	return nil
}

func evaluateCatalogAssertion(
	assertion conformance.CatalogAssertion,
	scenario conformance.CatalogScenario,
	runtime catalogFixtureRuntime,
	evidence conformance.Evidence,
) (bool, string) {
	switch assertion.Kind {
	case "operation_trace":
		return evaluateOperationExpectation(assertion.Expect, scenario, runtime, evidence.Operations)
	case "task_state":
		return evaluateTaskExpectation(assertion.Expect, runtime, evidence.Tasks)
	case "domain_error":
		return evaluateDomainErrorExpectation(assertion.Expect, evidence)
	case "assistant_output":
		return evaluateAssistantExpectation(assertion.Expect, runtime, evidence)
	case "event_log":
		return evaluateEventExpectation(assertion.Expect, runtime, evidence.Events)
	default:
		return false, "unsupported catalog assertion kind"
	}
}

func evaluateOperationExpectation(expect map[string]any, scenario conformance.CatalogScenario, runtime catalogFixtureRuntime, operations []conformance.Operation) (bool, string) {
	filtered := filterCatalogOperations(operations, expect, runtime)
	if expected, ok := stringListExpectation(expect, "contains_operation"); ok {
		for _, operation := range expected {
			if !operationPresent(filtered, operation) {
				return false, fmt.Sprintf("operation %q was not observed", operation)
			}
		}
	}
	if expected, ok := stringExpectation(expect, "first_operation"); ok {
		if len(filtered) == 0 || filtered[0].Operation != expected {
			return false, fmt.Sprintf("first operation was not %q", expected)
		}
	}
	if counts, ok := objectExpectation(expect, "operation_count"); ok {
		for operation, rawCount := range counts {
			expected, valid := intValue(rawCount)
			if !valid || operationCount(filtered, operation) != expected {
				return false, fmt.Sprintf("operation %q count was %d, expected %v", operation, operationCount(filtered, operation), rawCount)
			}
		}
	}
	if excluded, ok := stringListExpectation(expect, "excludes_operations"); ok {
		window := filtered
		if before, exists := stringExpectation(expect, "before"); exists {
			if index := operationIndex(window, before); index >= 0 {
				window = window[:index]
			}
		}
		for _, operation := range excluded {
			if operationPresent(window, operation) {
				return false, fmt.Sprintf("forbidden operation %q was observed", operation)
			}
		}
	}
	if sequences, ok := expect["excludes_sequences"].([]any); ok {
		for _, rawSequence := range sequences {
			sequence := anyStringList(rawSequence)
			for _, actor := range scenario.Actors {
				if operationSubsequence(filterOperationsByActor(filtered, actor.Name), sequence) {
					return false, fmt.Sprintf("actor %q used forbidden operation sequence %v", actor.Name, sequence)
				}
			}
		}
	}
	if expected, ok := stringExpectation(expect, "per_actor_operation"); ok {
		for _, actor := range scenario.Actors {
			if operationCount(filterOperationsByActor(filtered, actor.Name), expected) != 1 {
				return false, fmt.Sprintf("actor %q did not call %q exactly once", actor.Name, expected)
			}
		}
		if argument, exists := stringExpectation(expect, "unique_argument"); exists {
			seen := make(map[string]bool, len(scenario.Actors))
			for _, operation := range filtered {
				if operation.Operation != expected {
					continue
				}
				value := fmt.Sprint(operation.Arguments[argument])
				if value == "" || value == "<nil>" || seen[value] {
					return false, fmt.Sprintf("argument %q was empty or reused", argument)
				}
				seen[value] = true
			}
		}
	}
	if ordered, ok := stringListExpectation(expect, "ordered_operations"); ok {
		indexes, matched := orderedOperationIndexes(filtered, ordered)
		if !matched {
			return false, fmt.Sprintf("operations %v were not observed in order", ordered)
		}
		if checkpoint, exists := catalogClockCheckpoint(scenario); exists {
			last := filtered[indexes[len(indexes)-1]]
			observed, err := time.Parse(time.RFC3339Nano, last.Timestamp)
			if err != nil || observed.Before(checkpoint) {
				return false, fmt.Sprintf("operation %q was not observed after the harness clock checkpoint", last.Operation)
			}
		}
	}
	if expected, ok := stringExpectation(expect, "operation"); ok {
		matched := false
		for _, operation := range filtered {
			if operation.Operation != expected {
				continue
			}
			if resultTask, exists := stringExpectation(expect, "result_task"); exists && !operationResultTaskMatches(operation, runtime.TaskIDs[resultTask]) {
				continue
			}
			matched = true
			break
		}
		if !matched {
			return false, fmt.Sprintf("matching %q operation was not observed", expected)
		}
	}
	if extra, ok := objectExpectation(expect, "update_extra_contains"); ok {
		matched := false
		for _, operation := range filtered {
			if operation.Operation == "update" && objectContains(operation.Arguments["extra"], extra) {
				matched = true
				break
			}
		}
		if !matched {
			return false, "no update persisted the expected structured handoff"
		}
	}
	if after, ok := stringExpectation(expect, "after"); ok {
		index := operationIndex(filtered, after)
		choices, hasChoices := stringListExpectation(expect, "one_of_operations")
		if index < 0 || (hasChoices && !anyOperationPresent(filtered[index+1:], choices)) {
			return false, fmt.Sprintf("no terminal operation followed %q", after)
		}
	}
	return true, ""
}

func filterCatalogOperations(operations []conformance.Operation, expect map[string]any, runtime catalogFixtureRuntime) []conformance.Operation {
	actor, actorFiltered := stringExpectation(expect, "actor")
	if actorFiltered {
		actor = runtime.ActorNames[actor]
	}
	task, taskFiltered := stringExpectation(expect, "task")
	if taskFiltered {
		task = runtime.TaskIDs[task]
	}
	filtered := make([]conformance.Operation, 0, len(operations))
	for _, operation := range operations {
		if actorFiltered && operation.Actor != actor {
			continue
		}
		if taskFiltered && !operationTaskMatches(operation, task) {
			continue
		}
		filtered = append(filtered, operation)
	}
	return filtered
}

func evaluateTaskExpectation(expect map[string]any, runtime catalogFixtureRuntime, tasks map[string]any) (bool, string) {
	if owner, ok := stringExpectation(expect, "owner"); ok {
		owner = runtime.ActorNames[owner]
		expected, valid := intExpectation(expect, "owned_task_count")
		if valid {
			count := 0
			for _, rawTask := range tasks {
				if task, ok := rawTask.(catalogTaskState); ok && task.Owner == owner {
					count++
				}
			}
			if count != expected {
				return false, fmt.Sprintf("actor %q owned %d tasks, expected %d", owner, count, expected)
			}
			return true, ""
		}
	}
	reference, ok := stringExpectation(expect, "task")
	if !ok {
		return false, "task-state expectation did not identify a task or owner count"
	}
	rawTask, exists := tasks[reference]
	task, typed := rawTask.(catalogTaskState)
	if !exists || !typed {
		return false, fmt.Sprintf("task %q was not found", reference)
	}
	if expected, ok := stringExpectation(expect, "status"); ok && task.Status != expected {
		return false, fmt.Sprintf("task %q status was %q", reference, task.Status)
	}
	if expected, ok := intExpectation(expect, "owner_count"); ok {
		actual := 0
		if task.Owner != "" {
			actual = 1
		}
		if actual != expected {
			return false, fmt.Sprintf("task %q owner count was %d", reference, actual)
		}
	}
	if expected, ok := stringExpectation(expect, "owner"); ok {
		if task.Owner != runtime.ActorNames[expected] {
			return false, fmt.Sprintf("task %q owner was %q", reference, task.Owner)
		}
	}
	if rawAfter, ok := stringExpectation(expect, "lease_after"); ok {
		after, err := time.Parse(time.RFC3339, rawAfter)
		if err != nil || task.LeaseExpires <= uint64(after.UnixMilli()) {
			return false, fmt.Sprintf("task %q lease did not extend beyond %s", reference, rawAfter)
		}
	}
	return true, ""
}

func evaluateDomainErrorExpectation(expect map[string]any, evidence conformance.Evidence) (bool, string) {
	code, _ := stringExpectation(expect, "code")
	for _, domainError := range evidence.Errors {
		if domainError.Code != code {
			continue
		}
		if terminal, ok := boolExpectation(expect, "terminal"); ok && terminal {
			if len(evidence.Operations) == 0 || evidence.Operations[len(evidence.Operations)-1].Operation != domainError.Operation {
				return false, fmt.Sprintf("domain error %q was not terminal", code)
			}
		}
		return true, ""
	}
	return false, fmt.Sprintf("domain error %q was not observed", code)
}

func evaluateAssistantExpectation(expect map[string]any, runtime catalogFixtureRuntime, evidence conformance.Evidence) (bool, string) {
	messages := append([]string(nil), evidence.AssistantMessages...)
	if actor, ok := stringExpectation(expect, "actor"); ok {
		messages = []string{}
		resolved := runtime.ActorNames[actor]
		for _, message := range evidence.AssistantTurns {
			if message.Actor == resolved {
				messages = append(messages, message.Text)
			}
		}
	}
	if maximum, ok := intExpectation(expect, "maximum_messages"); ok && (len(messages) == 0 || len(messages) > maximum) {
		return false, fmt.Sprintf("assistant emitted %d messages, expected 1..%d", len(messages), maximum)
	}
	for _, message := range messages {
		if maximum, ok := intExpectation(expect, "maximum_words"); ok && len(strings.Fields(message)) > maximum {
			return false, fmt.Sprintf("assistant message exceeded %d words", maximum)
		}
	}
	joined := strings.ToLower(strings.Join(messages, "\n"))
	if expected, ok := stringListExpectation(expect, "semantic_contains"); ok {
		for _, value := range expected {
			if !strings.Contains(joined, strings.ToLower(value)) {
				return false, fmt.Sprintf("assistant output did not contain %q", value)
			}
		}
	}
	if excluded, ok := stringListExpectation(expect, "excludes_patterns"); ok {
		for _, value := range excluded {
			if strings.Contains(joined, strings.ToLower(value)) {
				return false, fmt.Sprintf("assistant output contained forbidden pattern %q", value)
			}
		}
	}
	if intents, ok := stringListExpectation(expect, "excludes_semantic_intent"); ok {
		for _, intent := range intents {
			if semanticIntentPresent(intent, joined) {
				return false, fmt.Sprintf("assistant output expressed forbidden intent %q", intent)
			}
		}
	}
	if intents, ok := stringListExpectation(expect, "semantic_excludes"); ok {
		for _, intent := range intents {
			if semanticIntentPresent(intent, joined) {
				return false, fmt.Sprintf("assistant output expressed forbidden intent %q", intent)
			}
		}
	}
	return true, ""
}

func evaluateEventExpectation(expect map[string]any, runtime catalogFixtureRuntime, events []json.RawMessage) (bool, string) {
	expected, ok := stringExpectation(expect, "contains_event")
	if !ok {
		return false, "event expectation did not identify an event"
	}
	actor, actorFiltered := stringExpectation(expect, "actor")
	if actorFiltered {
		actor = runtime.ActorNames[actor]
	}
	task, taskFiltered := stringExpectation(expect, "task")
	if taskFiltered {
		task = runtime.TaskIDs[task]
	}
	for _, raw := range events {
		var event store.Event
		if json.Unmarshal(raw, &event) != nil || string(event.Type) != expected {
			continue
		}
		if actorFiltered && event.Actor != actor {
			continue
		}
		if taskFiltered && strconv.FormatUint(event.TaskID, 10) != task {
			continue
		}
		return true, ""
	}
	return false, fmt.Sprintf("event %q was not observed", expected)
}

func semanticIntentPresent(intent, text string) bool {
	switch intent {
	case "ask_user_to_restate_plan":
		return strings.Contains(text, "restate") || strings.Contains(text, "share the plan") || strings.Contains(text, "provide the plan") || strings.Contains(text, "what should i work on")
	case "routine_coordination_narration":
		for _, phrase := range []string{"bootstrap", "acquire", "heartbeat", "lease", "request id", "coordination command", "terminal-todo"} {
			if strings.Contains(text, phrase) {
				return true
			}
		}
	}
	return false
}

func operationPresent(operations []conformance.Operation, expected string) bool {
	return operationIndex(operations, expected) >= 0
}

func operationIndex(operations []conformance.Operation, expected string) int {
	for index, operation := range operations {
		if operation.Operation == expected {
			return index
		}
	}
	return -1
}

func operationCount(operations []conformance.Operation, expected string) int {
	count := 0
	for _, operation := range operations {
		if operation.Operation == expected {
			count++
		}
	}
	return count
}

func filterOperationsByActor(operations []conformance.Operation, actor string) []conformance.Operation {
	filtered := make([]conformance.Operation, 0, len(operations))
	for _, operation := range operations {
		if operation.Actor == actor {
			filtered = append(filtered, operation)
		}
	}
	return filtered
}

func operationSubsequence(operations []conformance.Operation, expected []string) bool {
	_, matched := orderedOperationIndexes(operations, expected)
	return matched
}

func orderedOperationIndexes(operations []conformance.Operation, expected []string) ([]int, bool) {
	if len(expected) == 0 {
		return nil, false
	}
	indexes := make([]int, 0, len(expected))
	index := 0
	for operationIndex, operation := range operations {
		if index < len(expected) && operation.Operation == expected[index] {
			indexes = append(indexes, operationIndex)
			index++
		}
	}
	return indexes, index == len(expected)
}

func catalogClockCheckpoint(scenario conformance.CatalogScenario) (time.Time, bool) {
	current := scenario.InitialTime
	for _, turn := range scenario.Turns {
		if turn.Action == "advance_clock" {
			current = current.Add(time.Duration(turn.Seconds) * time.Second)
			return current, true
		}
	}
	return time.Time{}, false
}

func anyOperationPresent(operations []conformance.Operation, expected []string) bool {
	for _, value := range expected {
		if operationPresent(operations, value) {
			return true
		}
	}
	return false
}

func operationTaskMatches(operation conformance.Operation, taskID string) bool {
	return numericString(operation.Arguments["id"]) == taskID
}

func operationResultTaskMatches(operation conformance.Operation, taskID string) bool {
	task, ok := operation.Result["task"].(map[string]any)
	return ok && numericString(task["id"]) == taskID
}

func objectContains(raw any, expected map[string]any) bool {
	actual, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	for key, value := range expected {
		actualValue, expectedValue := fmt.Sprint(actual[key]), fmt.Sprint(value)
		if !strings.Contains(actualValue, expectedValue) {
			return false
		}
	}
	return true
}

func numericString(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatUint(uint64(typed), 10)
	case json.Number:
		return typed.String()
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func stringExpectation(expect map[string]any, key string) (string, bool) {
	value, ok := expect[key].(string)
	return value, ok
}

func stringListExpectation(expect map[string]any, key string) ([]string, bool) {
	raw, ok := expect[key]
	if !ok {
		return nil, false
	}
	return anyStringList(raw), true
}

func anyStringList(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		if strings, ok := raw.([]string); ok {
			return append([]string(nil), strings...)
		}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func objectExpectation(expect map[string]any, key string) (map[string]any, bool) {
	value, ok := expect[key].(map[string]any)
	return value, ok
}

func intExpectation(expect map[string]any, key string) (int, bool) {
	return intValue(expect[key])
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), typed == float64(int(typed))
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func boolExpectation(expect map[string]any, key string) (bool, bool) {
	value, ok := expect[key].(bool)
	return value, ok
}
