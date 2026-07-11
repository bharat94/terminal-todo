//go:build windows

package lock

import (
	"fmt"
	"syscall"
)

func (l *FileLock) tryAcquire(lockType Type) error {
	flags := uint32(syscall.LOCKFILE_FAIL_IMMEDIATELY)
	if lockType == Write {
		flags |= syscall.LOCKFILE_EXCLUSIVE_LOCK
	}
	var overlapped syscall.Overlapped
	h := syscall.Handle(l.f.Fd())
	if err := syscall.LockFileEx(h, flags, 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("failed to lock %s: %w", l.path, err)
	}
	return nil
}

func (l *FileLock) Release() error {
	var overlapped syscall.Overlapped
	h := syscall.Handle(l.f.Fd())
	if err := syscall.UnlockFileEx(h, 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("failed to unlock %s: %w", l.path, err)
	}
	return nil
}
