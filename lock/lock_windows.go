//go:build windows

package lock

import "errors"

func (l *FileLock) tryAcquire(lockType Type) error {
	return errors.New("file locking is not supported on Windows yet; see https://github.com/bharat94/terminal-todo/issues")
}

func (l *FileLock) Release() error {
	return l.f.Close()
}
