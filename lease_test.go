package main

import (
	"errors"
	"testing"
	"time"

	"terminal-todo/store"

	"github.com/stretchr/testify/assert"
)

func TestRenewLeaseExtendsOwnedActiveLeaseFromHeartbeatTime(t *testing.T) {
	s := store.NewTaskStore()
	task := s.AddTask("Long-running work", nil)
	now := time.Unix(1_800_000_000, 0)
	task.Status = store.StatusInProgress
	task.Owner = "agent-a"
	task.LeaseExpires = uint64(now.Add(15 * time.Minute).UnixMilli())
	previousExpiry := task.LeaseExpires

	renewed, err := renewLease(s, task.ID, "agent-a", time.Hour, now)

	assert.NoError(t, err)
	assert.Same(t, task, renewed)
	assert.Equal(t, uint64(now.Add(time.Hour).UnixMilli()), renewed.LeaseExpires)
	assert.Len(t, s.Events, 1)
	assert.Equal(t, store.EventLeaseRenewed, s.Events[0].Type)
	assert.Equal(t, "agent-a", s.Events[0].Actor)
	assert.Equal(t, "1h0m0s", s.Events[0].Data["ttl"])
	assert.Equal(t, formatTimestamp(previousExpiry), s.Events[0].Data["previous_expires"])
}

func TestRenewLeaseRejectsWrongOwnerWithoutMutation(t *testing.T) {
	s := store.NewTaskStore()
	task := s.AddTask("Owned work", nil)
	now := time.Unix(1_800_000_000, 0)
	task.Status = store.StatusInProgress
	task.Owner = "agent-a"
	task.LeaseExpires = uint64(now.Add(time.Hour).UnixMilli())
	previousExpiry := task.LeaseExpires

	_, err := renewLease(s, task.ID, "agent-b", time.Hour, now)

	assert.True(t, errors.Is(err, errLeaseNotOwner))
	assert.Equal(t, previousExpiry, task.LeaseExpires)
	assert.Empty(t, s.Events)
}

func TestRenewLeaseRejectsInactiveAndExpiredLeases(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, configure := range []func(*store.Task){
		func(task *store.Task) {},
		func(task *store.Task) {
			task.Status = store.StatusInProgress
			task.Owner = "agent-a"
			task.LeaseExpires = uint64(now.UnixMilli())
		},
	} {
		s := store.NewTaskStore()
		task := s.AddTask("Inactive work", nil)
		configure(task)

		_, err := renewLease(s, task.ID, "agent-a", time.Hour, now)

		assert.True(t, errors.Is(err, errLeaseNotActive))
		assert.Empty(t, s.Events)
	}
}
