// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	mediastore "git.sytue.eu/sytue/one/services/rooms/internal/media/store"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func documentCall(t *testing.T, h *pgtest.Harness, ctx context.Context, doc uuid.UUID) mediadomain.Room {
	t.Helper()
	room, err := mediastore.New(h.Pool).CreateRoom(ctx, mediastore.NewRoomRecord{Name: "doc-" + uuid.NewString(), Title: "Call", CreatedBy: "alice", Metadata: map[string]any{"source": map[string]any{"kind": "document", "ref_id": doc.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	return room
}

func documentProof(ctx context.Context, doc uuid.UUID, actor string, version, expires int64) context.Context {
	return mediadomain.WithDocumentCallAccess(ctx, mediadomain.DocumentCallAccess{DocumentID: doc.String(), Actor: actor, Tenant: "acme", Version: version, ExpiresAtMS: expires})
}

func TestDocumentCallFenceBlocksJoinWithPreCommitAuthorization(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	doc := uuid.New()
	st.SetDocumentCallsEnabled(true)
	st.SetCallMatrixGate(&callMatrixFake{})
	transport := &callTransportFake{}
	st.SetCallMediaTransport(transport)
	first := documentCall(t, h, ctx, doc)
	if _, err := st.PrepareCallEncryption(documentProof(ctx, doc, "alice", 0, 0), first.ID, "alice", "A", uuid.New(), mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	if err := st.AdvanceDocumentCallAccess(ctx, doc, "acme", 4); err != nil {
		t.Fatal(err)
	}
	closed, err := mediastore.New(h.Pool).GetRoom(ctx, first.ID)
	if err != nil || closed.Status != mediadomain.RoomEnded || len(transport.closed) == 0 {
		t.Fatalf("revoker acknowledged before SFU closure: %+v transport=%+v %v", closed, transport, err)
	}
	// This call was created after the revoker's snapshot but before Docs commits.
	next := documentCall(t, h, ctx, doc)
	join := uuid.New()
	if _, err := st.PrepareCallEncryption(documentProof(ctx, doc, "alice", 0, 0), next.ID, "alice", "A", join, mediadomain.RoleHost); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("old authorization passed new fence: %v", err)
	}
	binding, err := st.PrepareCallEncryption(documentProof(ctx, doc, "alice", 4, 0), next.ID, "alice", "A", join, mediadomain.RoleHost)
	if err != nil {
		t.Fatal(err)
	}
	for _, replay := range []int64{3, 4} {
		if err := st.AdvanceDocumentCallAccess(ctx, doc, "acme", replay); err != nil {
			t.Fatal(err)
		}
	}
	current, err := st.CallEncryption(documentProof(ctx, doc, "alice", 4, 0), next.ID, "alice", "A", join)
	if err != nil || current.Generation != binding.Generation {
		t.Fatalf("old callback revoked new call: %+v %v", current, err)
	}
}

func TestDocumentLinkDeadlineCapsEpochAndRemovesExpiredGuest(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	doc := uuid.New()
	st.SetDocumentCallsEnabled(true)
	st.SetCallMatrixGate(&callMatrixFake{})
	st.SetCallMediaTransport(&callTransportFake{})
	room := documentCall(t, h, ctx, doc)
	owner, guest := uuid.New(), uuid.New()
	if _, err := st.PrepareCallEncryption(documentProof(ctx, doc, "alice", 0, 0), room.ID, "alice", "A", owner, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Minute).Truncate(time.Millisecond)
	binding, err := st.PrepareCallEncryption(documentProof(ctx, doc, "bob", 0, expiry.UnixMilli()), room.ID, "bob", "B", guest, mediadomain.RoleSpeaker)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ExpiresAtMS != expiry.UnixMilli() {
		t.Fatalf("epoch extended link lifetime: %d > %d", binding.ExpiresAtMS, expiry.UnixMilli())
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_call_members SET document_access_until=now()-interval '1 second' WHERE call_id=$1 AND subject='bob'`, room.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ExpireCallEncryptions(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var removed bool
		if err := tx.QueryRow(ctx, `SELECT removed FROM matrix_call_members WHERE tenant='acme' AND call_id=$1 AND subject='bob'`, room.ID).Scan(&removed); err != nil {
			return err
		}
		if !removed {
			t.Error("expiry worker left revoked guest active before any client poll")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	current, err := st.CallEncryption(documentProof(ctx, doc, "alice", 0, 0), room.ID, "alice", "A", owner)
	if err != nil {
		t.Fatal(err)
	}
	if current.Generation <= binding.Generation || len(current.Members) != 1 || current.Members[0].Subject != "alice" {
		t.Fatalf("expired guest retained key recipient: %+v", current)
	}
	if _, err := st.CallEncryption(documentProof(ctx, doc, "bob", 0, expiry.UnixMilli()), room.ID, "bob", "B", guest); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("expired guest read new binding: %v", err)
	}
}

func TestDocumentIdentityFenceRejectsOldProofAndPreservesOtherCallMembers(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	doc := uuid.New()
	st.SetDocumentCallsEnabled(true)
	st.SetCallMatrixGate(&callMatrixFake{})
	st.SetCallMediaTransport(&callTransportFake{})
	room := documentCall(t, h, ctx, doc)
	alice, bob := uuid.New(), uuid.New()
	proof := func(actor string, version int64) context.Context {
		return mediadomain.WithDocumentCallAccess(ctx, mediadomain.DocumentCallAccess{DocumentID: doc.String(), Actor: actor, Tenant: "acme", IdentityVersion: version})
	}
	if _, err := st.PrepareCallEncryption(proof("alice", 0), room.ID, "alice", "A", alice, mediadomain.RoleHost); err != nil {
		t.Fatal(err)
	}
	before, err := st.PrepareCallEncryption(proof("bob", 0), room.ID, "bob", "B", bob, mediadomain.RoleSpeaker)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AdvanceDocumentIdentityAccess(ctx, "bob", "acme", 9); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var removed bool
		if err := tx.QueryRow(ctx, `SELECT removed FROM matrix_call_members WHERE tenant='acme' AND call_id=$1 AND subject='bob'`, room.ID).Scan(&removed); err != nil {
			return err
		}
		if !removed {
			t.Error("identity callback acknowledged before removing its actor")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	current, err := st.CallEncryption(proof("alice", 0), room.ID, "alice", "A", alice)
	if err != nil || current.Generation <= before.Generation || len(current.Members) != 1 || current.Members[0].Subject != "alice" {
		t.Fatalf("identity revocation retained recipient or removed innocent member: %+v %v", current, err)
	}
	if _, err := st.PrepareCallEncryption(proof("bob", 0), room.ID, "bob", "B", uuid.New(), mediadomain.RoleSpeaker); !errors.Is(err, mediadomain.ErrPermission) {
		t.Fatalf("old identity assertion reopened call: %v", err)
	}
	fresh, err := st.PrepareCallEncryption(proof("bob", 9), room.ID, "bob", "B", uuid.New(), mediadomain.RoleSpeaker)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AdvanceDocumentIdentityAccess(ctx, "bob", "acme", 9); err != nil {
		t.Fatal(err)
	}
	if fresh.Generation <= current.Generation {
		t.Fatal("new actor proof did not rotate epoch")
	}
}
