// SPDX-License-Identifier: Apache-2.0
//go:build cgo

package matrix

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

// Delegate every crypto operation to the maintained store. Queue a real state
// writer exactly when a new session has its acknowledged backup marker.
type roomsRotationAfterBackup struct {
	mxcrypto.Store
	client *Client
	room   id.RoomID
	armed  atomic.Bool
	done   chan error
}

func (s *roomsRotationAfterBackup) PutGroupSession(ctx context.Context, session *mxcrypto.InboundGroupSession) error {
	if err := s.Store.PutGroupSession(ctx, session); err != nil {
		return err
	}
	if session.KeyBackupVersion == "" || !s.armed.CompareAndSwap(true, false) {
		return nil
	}
	go func() {
		s.client.stateMu.Lock()
		err := s.RemoveOutboundGroupSession(ctx, s.room)
		s.client.stateMu.Unlock()
		s.done <- err
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for s.client.stateMu.TryRLock() {
		s.client.stateMu.RUnlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return nil
}

func testRoomsRotationWhileBackingUp(t *testing.T, c *Client, room string) {
	t.Helper()
	original := c.crypto.Machine().CryptoStore
	wrapped := &roomsRotationAfterBackup{Store: original, client: c, room: id.RoomID(room), done: make(chan error, 1)}
	wrapped.armed.Store(true)
	c.crypto.Machine().CryptoStore = wrapped
	defer func() { c.crypto.Machine().CryptoStore = original }()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if ref, err := c.SendText(ctx, room, "must not use a session retired during backup"); !errors.Is(err, ErrIdentity) || ref.EventID != "" {
		t.Fatal("Rooms used a session retired while backing up", err)
	}
	select {
	case err := <-wrapped.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("membership writer could not progress", ctx.Err())
	}
}
