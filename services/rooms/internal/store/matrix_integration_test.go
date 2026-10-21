// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type matrixGate struct {
	err     error
	rooms   []string
	members [][]string
}

func (g *matrixGate) Reconcile(_ context.Context, room string, subjects []string, _ bool, _ bool) error {
	g.rooms = append(g.rooms, room)
	g.members = append(g.members, subjects)
	return g.err
}
func (g *matrixGate) Redact(context.Context, domain.MatrixEventRef) error { return g.err }
func bindMatrix(t *testing.T, st *store.Store, ctx context.Context, ch uuid.UUID, actor string) string {
	t.Helper()
	room := "!" + ch.String() + ":matrix.sytue.eu"
	_, err := st.EnsureMatrixChannel(ctx, ch, actor, func(_ context.Context, state store.MatrixChannel) (string, error) { return room, nil })
	if err != nil {
		t.Fatal(err)
	}
	return room
}

func TestMatrixReferenceStoresNoPlaintextAndRetriesOnce(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	room := bindMatrix(t, st, ctx, ch.ID, "alice")
	legacy, err := st.CreateMessage(ctx, store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", Body: "pre-encryption history"})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Encryption != domain.LegacyEncryption || legacy.MatrixEvent != nil {
		t.Fatalf("legacy relabelled: %+v", legacy)
	}
	checks := 0
	rec := store.NewMessageRecord{ChannelID: ch.ID, AuthorSub: "alice", MatrixEvent: &domain.MatrixEventRef{RoomID: room, EventID: "$opaque"}, ValidateMatrix: func(context.Context, store.MatrixChannel) error { checks++; return nil }}
	first, err := st.CreateMessage(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	again, err := st.CreateMessage(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != again.ID || checks != 2 || first.Body != "" || first.Encryption != domain.MatrixEncryption {
		t.Fatalf("reference retry/content failure: %+v %+v checks=%d", first, again, checks)
	}
	rec.MatrixEvent.EventID = "$clear-leak"
	rec.Body = "never persist this"
	if _, err := st.CreateMessage(ctx, rec); err == nil {
		t.Fatal("database allowed plaintext beside encrypted reference")
	}
	rec.Body = ""
	rec.MatrixEvent.RoomID = "!other:matrix.sytue.eu"
	if _, err := st.CreateMessage(ctx, rec); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("foreign mapping accepted: %v", err)
	}
}

func TestMatrixRemoteRevocationFailureRollsBackMembership(t *testing.T) {
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
	bindMatrix(t, st, ctx, ch.ID, "alice")
	gate := &matrixGate{err: domain.ErrMatrixUnavailable}
	st.SetMatrixGate(gate)
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("local revocation acknowledged remote failure: %v", err)
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err != nil {
		t.Fatalf("membership transaction did not roll back: %v", err)
	}
	gate.err = nil
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	for _, sub := range gate.members[len(gate.members)-1] {
		if sub == "bob" {
			t.Fatal("removed subject still invited")
		}
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("removed user retained local access: %v", err)
	}
}

func TestMatrixLifecycleRejectsStaleActivationAndRetainsDenial(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	calls := 0
	apply := func(context.Context) error { calls++; return nil }
	if err := st.ApplyMatrixAccountAccess(ctx, "alice", "suspended", 20, apply); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyMatrixAccountAccess(ctx, "alice", "active", 19, apply); err != nil {
		t.Fatal(err)
	}
	if err := st.WithMatrixIdentity(ctx, "alice", func(context.Context) error { t.Fatal("denied identity callback executed"); return nil }); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stale active reopened identity: %v", err)
	}
	if calls != 1 {
		t.Fatalf("stale remote operation ran: %d", calls)
	}
	if err := st.ApplyMatrixAccountAccess(ctx, "alice", "active", 21, apply); err != nil {
		t.Fatal(err)
	}
	if err := st.WithMatrixIdentity(ctx, "alice", apply); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 30, apply); err != nil {
		t.Fatal(err)
	}
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "active", 29, func(context.Context) error { t.Fatal("stale proof executed"); return nil }); err != nil {
		t.Fatal(err)
	}
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM matrix_tenant_access WHERE tenant='acme' AND subject='alice'`).Scan(&status, &version); err != nil {
			return err
		}
		if status != "removed" || version != 30 {
			t.Fatalf("tenant denial lost: %s/%d", status, version)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatrixMergeIntoSameTenantPreservesRevocationsAndSignins(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_tenant_access(tenant,subject,status,version,applied) VALUES('acme','alice','removed',42,true)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_native_consents(tenant,state_hash,subject,challenge,expires_at) VALUES('acme','one-use','alice',repeat('x',43),now()+interval '1 minute')`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.merge','on',true)`); err != nil {
			return err
		}
		var note *string
		if err := tx.QueryRow(ctx, `SELECT platform_merge_prepare('acme','acme')`).Scan(&note); err != nil {
			return err
		}
		var kept bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_tenant_access WHERE tenant='acme' AND subject='alice' AND status='removed' AND version=42) AND EXISTS(SELECT 1 FROM matrix_native_consents WHERE tenant='acme' AND state_hash='one-use')`).Scan(&kept); err != nil {
			return err
		}
		if !kept || note != nil {
			t.Fatalf("self merge mutated state: kept=%v note=%v", kept, note)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
