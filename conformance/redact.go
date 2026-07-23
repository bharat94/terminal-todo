package conformance

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

const redactedValue = "<redacted>"

type redactor struct {
	values []string
}

func newRedactor(values []string) redactor {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
		for _, line := range strings.Split(value, "\n") {
			if line != "" {
				unique[line] = struct{}{}
			}
		}
	}
	sorted := make([]string, 0, len(unique))
	for value := range unique {
		sorted = append(sorted, value)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i]) == len(sorted[j]) {
			return sorted[i] < sorted[j]
		}
		return len(sorted[i]) > len(sorted[j])
	})
	return redactor{values: sorted}
}

func (r redactor) text(value string) string {
	for _, secret := range r.values {
		value = strings.ReplaceAll(value, secret, redactedValue)
	}
	return value
}

func (r redactor) event(line []byte, stream Stream, sequence uint64) Event {
	var value any
	if json.Unmarshal(line, &value) == nil {
		value = r.jsonValue(value)
		encoded, err := marshalJSON(value)
		if err == nil {
			return Event{
				Sequence: sequence,
				Stream:   stream,
				Kind:     EventJSON,
				JSON:     encoded,
			}
		}
	}
	return Event{
		Sequence: sequence,
		Stream:   stream,
		Kind:     EventText,
		Text:     r.text(string(line)),
	}
}

func marshalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func (r redactor) jsonValue(value any) any {
	switch typed := value.(type) {
	case string:
		return r.text(typed)
	case []any:
		for i := range typed {
			typed[i] = r.jsonValue(typed[i])
		}
		return typed
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			redactedKey := r.text(key)
			if sensitiveJSONKey(redactedKey) {
				result[redactedKey] = redactedValue
				continue
			}
			result[redactedKey] = r.jsonValue(item)
		}
		return result
	default:
		return value
	}
}

func sensitiveJSONKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	switch normalized {
	case "api_key", "access_token", "authorization", "session_id", "thread_id":
		return true
	default:
		return false
	}
}
