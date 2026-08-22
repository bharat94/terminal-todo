package cli

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type persistedInputError struct {
	err error
}

func (e *persistedInputError) Error() string { return e.err.Error() }
func (e *persistedInputError) Unwrap() error { return e.err }

func persistedInputFailure(err error) error {
	if err == nil {
		return nil
	}
	return &persistedInputError{err: err}
}

func isPersistedInputFailure(err error) bool {
	var inputErr *persistedInputError
	return errors.As(err, &inputErr)
}

// Persisted input limits bound user-controlled state independently of the
// transport's frame size. They apply to new values only so older stores with
// larger fields remain readable and can still be repaired or updated.
const (
	maxTaskTitleBytes        = 1024
	maxActorBytes            = 128
	maxReasonBytes           = 8 * 1024
	maxErrorBytes            = 8 * 1024
	maxLogMessageBytes       = 16 * 1024
	maxMetadataKeyBytes      = 128
	maxMetadataValueBytes    = 16 * 1024
	maxDependencyBytes       = 512
	maxCapabilityBytes       = 128
	maxTagBytes              = 128
	maxAgentDescriptionBytes = 4 * 1024

	maxTaskDependencies = 128
	maxTaskCapabilities = 64
	maxTaskTags         = 64
	maxTaskExtraEntries = 128
)

func validatePersistedString(field, value string, maxBytes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s must be at most %d UTF-8 bytes", field, maxBytes)
	}
	return nil
}

func validateRequiredPersistedString(field, value string, maxBytes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return validatePersistedString(field, value, maxBytes)
}

func validateActor(actor string, required bool) error {
	if actor == "" && !required {
		return nil
	}
	return validateRequiredPersistedString("actor", actor, maxActorBytes)
}

func normalizePersistedValues(values []string) []string {
	if values == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func validatePersistedList(field, itemName string, values []string, maxItems, maxItemBytes int) error {
	if len(values) > maxItems {
		return fmt.Errorf("%s must contain at most %d items", field, maxItems)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot contain an empty %s", field, itemName)
		}
		if err := validatePersistedString(itemName, value, maxItemBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateCapabilities(values []string) error {
	if err := validatePersistedList("capabilities", "capability", values, maxTaskCapabilities, maxCapabilityBytes); err != nil {
		return err
	}
	for _, value := range values {
		if strings.Contains(value, ",") {
			return fmt.Errorf("capability %q cannot contain ','", value)
		}
	}
	return nil
}

func validateTags(values []string) error {
	return validatePersistedList("tags", "tag", values, maxTaskTags, maxTagBytes)
}

func validateDependencies(values []string) error {
	return validatePersistedList("dependencies", "dependency", values, maxTaskDependencies, maxDependencyBytes)
}

func validateExtra(extra map[string]string) error {
	if len(extra) > maxTaskExtraEntries {
		return fmt.Errorf("extra must contain at most %d entries", maxTaskExtraEntries)
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("extra key cannot be empty")
		}
		if err := validatePersistedString("extra key", key, maxMetadataKeyBytes); err != nil {
			return err
		}
		if err := validatePersistedString("extra value", value, maxMetadataValueBytes); err != nil {
			return err
		}
	}
	return nil
}

// Legacy stores may already exceed a collection limit. Such a collection can
// be left unchanged or reduced, but requests cannot grow it further.
func validateProjectedCardinality(field string, current, projected, maxItems int) error {
	if projected > maxItems && projected > current {
		return fmt.Errorf("%s must contain at most %d items", field, maxItems)
	}
	return nil
}

func projectedExtraCount(current, updates map[string]string) int {
	count := len(current)
	for key := range updates {
		if _, exists := current[key]; !exists {
			count++
		}
	}
	return count
}
