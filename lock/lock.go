package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Type int

const (
	Read Type = iota
	Write
)

const suffix = ".lock"

var errContended = errors.New("lock is held by another process")

type FileLock struct {
	f    *os.File
	path string
}

func Open(path string) (*FileLock, error) {
	lockPath := filepath.Clean(path) + suffix
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}
	return &FileLock{f: f, path: lockPath}, nil
}

func (l *FileLock) Close() error {
	return l.f.Close()
}

func (l *FileLock) Path() string {
	return l.path
}

// AcquireWithTimeout attempts to acquire the lock with a retry loop.
// A zero or negative timeout means block indefinitely.
func (l *FileLock) AcquireWithTimeout(lockType Type, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := l.tryAcquire(lockType)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errContended) {
			return err
		}
		if timeout > 0 && time.Now().After(deadline) {
			return fmt.Errorf("lock acquisition timed out after %v", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *FileLock) Acquire(lockType Type) error {
	return l.AcquireWithTimeout(lockType, 0)
}
