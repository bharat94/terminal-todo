package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fault injection.
//
// docs/production-readiness.md lists filesystem fault evidence as a gate on a
// 1.0 claim, and docs/compatibility.md states the durability contract: writes
// go to a temporary file in the destination directory, are flushed, and are
// then atomically renamed into place.
//
// The claims that matter to a fleet are what a reader sees when a writer dies
// mid-write, and what happens to state that is damaged rather than absent. A
// worker that reads a half-written store, or that silently starts from an
// empty graph after corruption, would hand the same work to two agents.

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tasks.bin")
}

// seedStore writes a small committed graph.
func seedStore(t *testing.T, path string) *TaskStore {
	t.Helper()
	s := NewTaskStore()
	s.AddTask("Prerequisite", nil)
	s.AddTask("Dependent", []string{"todo://local/1"})
	require.NoError(t, s.Save(path))
	return s
}

// An interrupted write leaves its temporary file behind. Because the rename is
// the commit point, the committed store must be untouched and every task must
// still be there.
func TestInterruptedWriteLeavesTheCommittedStoreIntact(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	// Simulate a writer that died between creating its temporary file and
	// renaming it into place.
	partial := filepath.Join(filepath.Dir(path), ".tasks-interrupted.tmp")
	require.NoError(t, os.WriteFile(partial, []byte("half-written garbage"), 0o600))

	loaded, err := LoadCurrent(path)
	require.NoError(t, err, "an abandoned temporary file must not affect readers")
	assert.Len(t, loaded.Tasks, 2)

	// A later write must still commit cleanly alongside the debris.
	_, err = Update(path, func(s *TaskStore) error {
		s.AddTask("Added after the interruption", nil)
		return nil
	})
	require.NoError(t, err)

	loaded, err = LoadCurrent(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Tasks, 3)
}

// Truncation is the shape a partially flushed file takes. Every prefix must be
// either rejected or read as a complete store — never silently accepted as an
// empty graph, which would make ready work appear unclaimed.
func TestTruncatedStoreIsRejectedRatherThanReadAsEmpty(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	complete, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, complete)

	for _, cut := range []int{1, len(complete) / 4, len(complete) / 2, len(complete) - 1} {
		if cut <= 0 {
			continue
		}
		truncated := filepath.Join(t.TempDir(), "tasks.bin")
		require.NoError(t, os.WriteFile(truncated, complete[:cut], 0o600))

		loaded, err := Load(truncated)
		if err != nil {
			continue
		}
		assert.NotEmpty(t, loaded.Tasks,
			"a %d-byte prefix loaded as an empty graph; ready work would look unclaimed", cut)
	}
}

// Corruption in the middle of a committed file must not be mistaken for a
// valid store with fewer tasks.
func TestCorruptedStoreDoesNotSilentlyLoseTasks(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for i := range data {
		data[i] ^= 0xFF
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))

	loaded, err := Load(path)
	if err == nil {
		assert.NotEmpty(t, loaded.Tasks, "inverted bytes decoded to an empty graph")
	}
}

// The lock sidecar carries lock identity across atomic replaces. Removing it
// between transactions must not lose committed state or wedge later writers.
func TestMissingLockSidecarDoesNotLoseCommittedState(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	require.NoError(t, os.Remove(path+".lock"))

	loaded, err := LoadCurrent(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Tasks, 2)

	_, err = Update(path, func(s *TaskStore) error {
		s.AddTask("Written after the sidecar was removed", nil)
		return nil
	})
	require.NoError(t, err)

	loaded, err = LoadCurrent(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Tasks, 3)
}

// A stale, empty sidecar left by a crashed writer must not block acquisition.
func TestStaleLockSidecarDoesNotWedgeWriters(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	require.NoError(t, os.WriteFile(path+".lock", nil, 0o600))

	_, err := Update(path, func(s *TaskStore) error {
		s.AddTask("Written despite the stale sidecar", nil)
		return nil
	})
	require.NoError(t, err)

	loaded, err := LoadCurrent(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Tasks, 3)
}

// A mutation that fails must commit nothing. The transaction is what lets a
// worker retry a failed operation without inspecting what partially landed.
func TestFailedMutationCommitsNothing(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = Update(path, func(s *TaskStore) error {
		s.AddTask("Should not survive", nil)
		s.AddEvent(EventTaskCreated, 99, "w1", nil)
		return assert.AnError
	})
	require.Error(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a failed mutation wrote to the committed store")

	loaded, err := LoadCurrent(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Tasks, 2)
}

// Committing must not leave temporary files behind. An accumulating .tmp
// litter in a long-lived project directory is both a leak and a source of
// confusing debris during incident review.
func TestSuccessfulWritesLeaveNoTemporaryFiles(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)

	for i := 0; i < 5; i++ {
		_, err := Update(path, func(s *TaskStore) error {
			s.AddTask("Churn", nil)
			return nil
		})
		require.NoError(t, err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp",
			"a committed write left %s behind", entry.Name())
	}
}

// A store the reader cannot open must fail loudly. Returning an empty graph
// would let a worker believe there is no work rather than that it cannot see
// the work.
func TestUnreadableStoreFailsRatherThanReturningAnEmptyGraph(t *testing.T) {
	path := storePath(t)
	seedStore(t, path)
	require.NoError(t, os.WriteFile(path, []byte("not messagepack at all"), 0o600))

	_, err := Load(path)
	assert.Error(t, err, "an undecodable store must not read as an empty graph")
}
