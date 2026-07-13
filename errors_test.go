package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitCodesAreStable(t *testing.T) {
	cases := map[ErrorCode]int{
		ErrTaskNotFound:        1,
		ErrInvalidArgs:         1,
		ErrNotOwner:            1,
		ErrDependency:          1,
		ErrNotInitialized:      2,
		ErrStoreCorrupted:      2,
		ErrSchemaVersion:       2,
		ErrLockContention:      3,
		ErrCycleDetected:       4,
		ErrAlreadyClaimed:      5,
		ErrNoWork:              6,
		ErrAgentAtCapacity:     7,
		ErrIdempotencyConflict: 8,
	}
	for code, want := range cases {
		t.Run(string(code), func(t *testing.T) {
			assert.Equal(t, want, exitCode(code))
		})
	}
}
