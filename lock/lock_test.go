package lock

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	defer os.Remove(l.Path())

	if !fileExists(l.Path()) {
		t.Error("Open should create a lock file on disk")
	}
	if got := l.Path(); got != path+".lock" {
		t.Errorf("Path() = %q, want %q", got, path+".lock")
	}
}

func TestAcquireRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	defer os.Remove(l1.Path())

	l2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()

	if err := l1.Acquire(Read); err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := l2.Acquire(Read); err != nil {
			t.Error("concurrent readers should both acquire shared lock:", err)
			return
		}
		if err := l2.Release(); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
}

func TestAcquireWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	defer os.Remove(l1.Path())

	l2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()

	if err := l1.Acquire(Write); err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	err = l2.AcquireWithTimeout(Write, 100*time.Millisecond)
	if err == nil {
		t.Fatal("concurrent writer should not acquire exclusive lock")
	}
}

func TestAcquireWithTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	defer os.Remove(l1.Path())

	l2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()

	if err := l1.Acquire(Write); err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	timeout := 50 * time.Millisecond
	start := time.Now()
	err = l2.AcquireWithTimeout(Write, timeout)
	if err == nil {
		t.Fatal("expected timeout error when lock is held")
	}
	if elapsed := time.Since(start); elapsed < timeout-10*time.Millisecond {
		t.Errorf("timeout returned too early: %v (expected >= %v)", elapsed, timeout)
	}
}

func TestRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	defer os.Remove(l1.Path())

	l2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()

	if err := l1.Acquire(Write); err != nil {
		t.Fatal(err)
	}

	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}

	if err := l2.Acquire(Write); err != nil {
		t.Fatal("should acquire exclusive lock after release:", err)
	}
	l2.Release()
}

func TestClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := l.AcquireWithTimeout(Read, time.Second); err == nil {
		t.Error("expected error acquiring lock on closed file")
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Errorf("permanent lock error was retried for %v", elapsed)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	lRead, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lRead.Close()
	defer os.Remove(lRead.Path())

	lWrite, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lWrite.Close()

	if err := lRead.Acquire(Read); err != nil {
		t.Fatal(err)
	}

	writeDone := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		close(ready)
		if err := lWrite.Acquire(Write); err != nil {
			t.Error("write should eventually acquire:", err)
			return
		}
		close(writeDone)
	}()
	<-ready
	time.Sleep(20 * time.Millisecond)

	select {
	case <-writeDone:
		t.Fatal("write should not acquire while read is held")
	default:
	}

	if err := lRead.Release(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("write should acquire after read releases")
	}

	lWrite.Release()
}

func TestDoubleRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	defer os.Remove(l.Path())

	if err := l.Acquire(Write); err != nil {
		t.Fatal(err)
	}

	if err := l.Release(); err != nil {
		t.Fatal(err)
	}

	if err := l.Release(); err != nil {
		t.Log("second release returned error (allowed):", err)
	}
}
