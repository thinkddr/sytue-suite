// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	mediastore "git.sytue.eu/sytue/one/services/rooms/internal/media/store"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type callMatrixFake struct {
	err     error
	retired []string
}

func (f *callMatrixFake) EnsureCallRoom(_ context.Context, _ string, id uuid.UUID, generation int64, _ []string) (string, error) {
	return fmt.Sprintf("!%s-%d:matrix.sytue.eu", id, generation), f.err
}

func (f *callMatrixFake) Reconcile(_ context.Context, room string, _ []string, _ bool, _ bool) error {
	f.retired = append(f.retired, room)
	return f.err
}

type callTransportFake struct {
	err     error
	closed  []string
	created []string
	enabled []bool
	removed []string
}

func (f *callTransportFake) PauseCall(_ context.Context, _ string, _ []string, removed []string) error {
	f.removed = append(f.removed, removed...)
	return f.err
}

func (f *callTransportFake) EnableCall(_ context.Context, _, _, _, _ string, publish bool) error {
	f.enabled = append(f.enabled, publish)
	return f.err
}

func (f *callTransportFake) RotateCallTransport(_ context.Context, oldName, newName, _ string, _ uuid.UUID, _ int64, _ int) error {
	f.closed = append(f.closed, oldName)
	f.created = append(f.created, newName)
	return f.err
}

func seedCall(t *testing.T, h *pgtest.Harness, ctx context.Context, st *store.Store, ch uuid.UUID) (mediadomain.Room, *callMatrixFake, *callTransportFake) {
	t.Helper()
	var metadata map[string]any
	if ch != uuid.Nil {
		metadata = map[string]any{"source": map[string]any{"kind": "chat", "ref_id": ch.String()}}
	}
	room, err := mediastore.New(h.Pool).CreateRoom(ctx, mediastore.NewRoomRecord{Name: "encrypted-" + uuid.NewString(), Title: "Call", CreatedBy: "alice", Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	matrix, transport := &callMatrixFake{}, &callTransportFake{}
	st.SetCallMatrixGate(matrix)
	st.SetCallMediaTransport(transport)
	return room, matrix, transport
}

func TestCallEpochDeviceReplacementRejectsStaleConnectionEvents(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	room, matrix, transport := seedCall(t, h, ctx, st, uuid.Nil)
	firstJoin := uuid.New()
	first, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "DEVICE-A", firstJoin, mediadomain.RoleHost)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := st.CallPresence(ctx, room.ID, "alice", firstJoin, "SID-A", first.LiveKitRoom, true); err != nil || !ok {
		t.Fatalf("join: %v %v", ok, err)
	}
	secondJoin := uuid.New()
	second, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "DEVICE-B", secondJoin, mediadomain.RoleHost)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != first.Generation+1 || second.MatrixRoomID == first.MatrixRoomID || second.LiveKitRoom == first.LiveKitRoom || len(second.Members) != 1 || second.LeaderDeviceID != "DEVICE-B" {
		t.Fatalf("device did not rotate both transports: %+v %+v", first, second)
	}
	if _, err := st.CallEncryption(ctx, room.ID, "alice", "DEVICE-A", firstJoin); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("replaced device received epoch: %v", err)
	}
	for _, joined := range []bool{true, false} {
		if ok, err := st.CallPresence(ctx, room.ID, "alice", firstJoin, "SID-A", first.LiveKitRoom, joined); err != nil || ok {
			t.Fatalf("stale event accepted joined=%v: %v %v", joined, ok, err)
		}
	}
	if ok, err := st.CallPresence(ctx, room.ID, "alice", secondJoin, "SID-B", second.LiveKitRoom, true); err != nil || !ok {
		t.Fatalf("new join: %v %v", ok, err)
	}
	// An old room webhook with even the current nonce cannot change this epoch.
	if ok, err := st.CallPresence(ctx, room.ID, "alice", secondJoin, "SID-B", first.LiveKitRoom, false); err != nil || ok {
		t.Fatalf("retired SFU accepted: %v %v", ok, err)
	}
	if ended, err := st.FinishCallTransport(ctx, room.ID, first.LiveKitRoom); err != nil || ended {
		t.Fatalf("old SFU ended active call: %v %v", ended, err)
	}
	current, err := st.CallEncryption(ctx, room.ID, "alice", "DEVICE-B", secondJoin)
	if err != nil || current.Generation != second.Generation {
		t.Fatalf("stale event rotated current epoch: %+v %v", current, err)
	}
	if len(matrix.retired) != 1 || len(transport.created) != 2 || transport.closed[1] != first.LiveKitRoom {
		t.Fatalf("old rooms not retired: %+v %+v", matrix, transport)
	}
	// Reloads lose in-memory media keys, so even the same device receives a fresh epoch.
	third, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "DEVICE-B", uuid.New(), mediadomain.RoleHost)
	if err != nil || third.Generation != second.Generation+1 {
		t.Fatalf("reload reused key generation: %+v %v", third, err)
	}
}

func TestCallReadyPreservesViewerRoleAndRejectsOldGeneration(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	room, _, transport := seedCall(t, h, ctx, st, uuid.Nil)
	join := uuid.New()
	first, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "VIEWER", join, mediadomain.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReadyCallEncryption(ctx, room.ID, "alice", "VIEWER", join, first.Generation); err != nil {
		t.Fatal(err)
	}
	if len(transport.enabled) != 1 || transport.enabled[0] {
		t.Fatal("ready elevated a viewer")
	}
	second, err := st.PrepareCallEncryption(ctx, room.ID, "bob", "SPEAKER", uuid.New(), mediadomain.RoleSpeaker)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReadyCallEncryption(ctx, room.ID, "alice", "VIEWER", join, first.Generation); !errors.Is(err, mediadomain.ErrEncryptionGeneration) {
		t.Fatalf("old generation enabled publication: %v", err)
	}
	if len(transport.enabled) != 1 {
		t.Fatal("SFU enabled before current key acknowledgement")
	}
	if err := st.ReadyCallEncryption(ctx, room.ID, "alice", "OTHER", join, second.Generation); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("foreign device ready accepted: %v", err)
	}
}

func TestCallKickCannotRejoinThroughHistoricalParticipation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	room, _, _ := seedCall(t, h, ctx, st, uuid.Nil)
	join := uuid.New()
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", join, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeCallMember(ctx, room.ID, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", uuid.New(), mediadomain.RoleHost); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("kicked actor rejoined: %v", err)
	}
	if err := st.ReadmitCallMember(ctx, room.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", uuid.New(), mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
}

func TestCallChannelRevocationRollsBackIfSFUCannotRevoke(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPrivate)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddChannelMember(ctx, ch.ID, "alice", "bob", "Bob"); err != nil {
		t.Fatal(err)
	}
	room, _, transport := seedCall(t, h, ctx, st, ch.ID)
	alice, bob := uuid.New(), uuid.New()
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", alice, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "bob", "B", bob, mediadomain.RoleSpeaker); err != nil {
		t.Fatal(err)
	}
	transport.err = mediadomain.ErrEncryptionUnavailable
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); !errors.Is(err, transport.err) {
		t.Fatalf("ACL removal acknowledged failed revocation: %v", err)
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err != nil {
		t.Fatalf("ACL did not roll back: %v", err)
	}
	transport.err = nil
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CallEncryption(ctx, room.ID, "bob", "B", bob); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("removed channel member received new keys: %v", err)
	}
}

func TestCallPendingJoinExpiresAndLifecycleRevokesStandalone(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	room, _, _ := seedCall(t, h, ctx, st, uuid.Nil)
	join := uuid.New()
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", join, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET pending_until=now()-interval '1 second' WHERE tenant='acme' AND call_id=$1`, room.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ExpireCallEncryptions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CallEncryption(ctx, room.ID, "alice", "A", join); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("expired pending join received epoch: %v", err)
	}
	join = uuid.New()
	if _, err := st.PrepareCallEncryption(ctx, room.ID, "alice", "A", join, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyMatrixAccountAccess(ctx, "alice", "suspended", 17, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CallEncryption(ctx, room.ID, "alice", "A", join); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("suspended user received standalone epoch: %v", err)
	}
}
