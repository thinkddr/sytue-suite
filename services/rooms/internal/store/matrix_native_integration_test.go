// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestMatrixNativeHandoffBindsActorTenantPKCEAndConsumesOnce(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	state, err := st.CreateMatrixNativeConsent(ctx, "alice", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PreviewMatrixNativeConsent(tenantCtx("other"), "alice", state); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong tenant preview: %v", err)
	}
	if err := st.PreviewMatrixNativeConsent(ctx, "bob", state); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong actor preview: %v", err)
	}
	const token = "matrix-short-lived-login-token"
	ticket, err := st.AcceptMatrixNativeConsent(ctx, "alice", state, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var secret string
		if err := tx.QueryRow(ctx, `SELECT login_secret FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(state)).Scan(&secret); err != nil {
			return err
		}
		if !strings.HasPrefix(secret, "encv1:") || strings.Contains(secret, token) {
			t.Fatal("Matrix token was stored in cleartext")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinishMatrixNativeConsent(ctx, "alice", state, ticket, "wrong-challenge"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("PKCE bypass: %v", err)
	}
	if _, err := st.FinishMatrixNativeConsent(ctx, "bob", state, ticket, challenge); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("actor bypass: %v", err)
	}
	got, err := st.FinishMatrixNativeConsent(ctx, "alice", state, ticket, challenge)
	if err != nil || got != token {
		t.Fatalf("finish %q %v", got, err)
	}
	if _, err := st.FinishMatrixNativeConsent(ctx, "alice", state, ticket, challenge); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ticket replay: %v", err)
	}
}

func TestMatrixDraftRoomsOnlyProvisionForActorAndStopAfterDenial(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	calls := 0
	provision := func(context.Context, string) (string, error) { calls++; return "!draft:matrix.sytue.eu", nil }
	rooms, err := st.EnsureMatrixDraftRooms(ctx, "alice", provision)
	if err != nil || len(rooms) != 1 {
		t.Fatalf("rooms=%v err=%v", rooms, err)
	}
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 99, func(context.Context) error { return nil }); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatalf("missing remote revoker acknowledged access removal: %v", err)
	}
	if _, err := st.EnsureMatrixDraftRooms(ctx, "alice", provision); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("denied draft provision: %v", err)
	}
	if calls != 1 {
		t.Fatalf("denial created a new room: calls=%d", calls)
	}
}
