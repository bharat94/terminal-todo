package main

import (
	"errors"
	"fmt"

	"github.com/bharat94/terminal-todo/store"
)

type lifecycleCommandError struct {
	code    ErrorCode
	message string
}

func (e *lifecycleCommandError) Error() string {
	return e.message
}

func lifecycleError(code ErrorCode, format string, args ...interface{}) error {
	return &lifecycleCommandError{
		code:    code,
		message: fmt.Sprintf(format, args...),
	}
}

func updateLifecycleStore(mutate func(*store.TaskStore) error) *store.TaskStore {
	s, err := store.Update(tasksBinPath(), mutate)
	if err != nil {
		var commandErr *lifecycleCommandError
		if errors.As(err, &commandErr) {
			fail(commandErr.code, "%s", commandErr.message)
		}
		fail(ErrStoreCorrupted, "%v", err)
	}
	return s
}
