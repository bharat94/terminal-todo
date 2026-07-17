//go:build windows

package lock

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func (l *FileLock) tryAcquire(lockType Type) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if lockType == Write {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var overlapped windows.Overlapped
	h := windows.Handle(l.f.Fd())
	if err := windows.LockFileEx(h, flags, 0, 1, 0, &overlapped); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return errContended
		}
		return fmt.Errorf("failed to lock %s: %w", l.path, err)
	}
	return nil
}

func (l *FileLock) Release() error {
	var overlapped windows.Overlapped
	h := windows.Handle(l.f.Fd())
	if err := windows.UnlockFileEx(h, 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("failed to unlock %s: %w", l.path, err)
	}
	return nil
}
