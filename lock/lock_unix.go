//go:build unix

package lock

import (
	"errors"
	"fmt"
	"syscall"
)

func (l *FileLock) tryAcquire(lockType Type) error {
	how := syscall.LOCK_EX | syscall.LOCK_NB
	if lockType == Read {
		how = syscall.LOCK_SH | syscall.LOCK_NB
	}
	if err := syscall.Flock(int(l.f.Fd()), how); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errContended
		}
		return fmt.Errorf("failed to lock %s: %w", l.path, err)
	}
	return nil
}

func (l *FileLock) Release() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
}
