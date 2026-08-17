package cli

import (
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

// updateLifecycleStore is retained as the explicit name at call sites that
// raise lifecycleError values. Classification now lives in updateStore so that
// every command, whichever convention it uses, resolves an error the same way.
func updateLifecycleStore(mutate func(*store.TaskStore) error) *store.TaskStore {
	return updateStore(mutate)
}
