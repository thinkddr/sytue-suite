// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
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

func TestMatrixIsolatedNativeConsentResolvesOnlyItsOwnersImmutableTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	acme, beta := tenantCtx("acme"), tenantCtx("beta")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	isolated, err := st.CreateMatrixNativeConsentForIdentity(acme, "alice", challenge, "drive", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := st.CreateMatrixNativeConsent(acme, "alice", challenge)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := st.MatrixNativeConsentTenant(beta, "alice", isolated)
	if err != nil || selected.Slug != "acme" {
		t.Fatal("browser organization replaced the grant", selected, err)
	}
	for _, test := range []struct{ subject, state string }{
		{"bob", isolated}, {"alice", isolated + "unknown"}, {"alice", legacy},
	} {
		if _, err := st.MatrixNativeConsentTenant(beta, test.subject, test.state); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("foreign or legacy grant crossed browser context", err)
		}
	}
	if _, err := st.MatrixNativeConsentTenant(acme, "alice", legacy); err != nil {
		t.Fatal("same-tenant legacy grant broke", err)
	}
	// Routing metadata is not authorization: the original tenant operation must
	// still reject the wrong context, and lifecycle retirement removes routing too.
	if _, err := st.PreviewMatrixNativeDestination(beta, "alice", isolated); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("routing bypassed tenant predicate", err)
	}
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		_, err := tx.Exec(acme, `UPDATE matrix_native_consents SET expires_at=now()-interval '1 second' WHERE tenant=$1 AND subject=$2`, "acme", "alice")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MatrixNativeConsentTenant(beta, "alice", isolated); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("expired grant resolved", err)
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

func TestMatrixNativeConsentPersistsOnlyItsOriginalProduct(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	for _, product := range []string{"hub", "mail", "drive", "docs", "sheets", "slides", "pdf", "calendar", "contacts", "notes", "photos", "rooms", "cast", "erp", "bases"} {
		state, err := st.CreateMatrixNativeConsentFor(ctx, "alice", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", product)
		if err != nil {
			t.Fatal(err)
		}
		got, err := st.PreviewMatrixNativeProduct(ctx, "alice", state)
		if err != nil || got != product {
			t.Fatal("original product was not bound to state", got, err)
		}
		if _, err = st.PreviewMatrixNativeProduct(ctx, "bob", state); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("another actor read a grant", err)
		}
		if _, err = st.PreviewMatrixNativeProduct(tenantCtx("other"), "alice", state); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("another tenant read a grant", err)
		}
		ticket, bound, err := st.AcceptMatrixNativeConsentWithProduct(ctx, "alice", state, "one-use-matrix-login-token")
		if err != nil || ticket == "" || bound != product {
			t.Fatal("ticket lost its original destination", err)
		}
		if _, _, err = st.AcceptMatrixNativeConsentWithProduct(ctx, "alice", state, "one-use-matrix-login-token"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("callback replay issued another ticket", err)
		}
	}
	if _, err := st.CreateMatrixNativeConsentFor(ctx, "alice", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "javascript:alert(1)"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("unregistered redirect product persisted", err)
	}
}

func TestMatrixNativeConsentCannotDowngradeOrCrossIdentityVersions(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	for _, version := range []storage.IdentityVersion{storage.LegacyIdentity, storage.IsolatedIdentity} {
		for _, product := range storage.Products() {
			state, err := st.CreateMatrixNativeConsentForIdentity(ctx, "alice", challenge, product, version)
			if err != nil {
				t.Fatal(err)
			}
			destination, err := st.PreviewMatrixNativeDestination(ctx, "alice", state)
			if err != nil || destination.Product != product || destination.IdentityVersion != version {
				t.Fatal("grant identity changed", err)
			}
			ticket, accepted, err := st.AcceptMatrixNativeConsentWithDestination(ctx, "alice", state, "artificial-matrix-login-token")
			if err != nil || accepted != destination {
				t.Fatal("callback selected another identity", err)
			}
			wrong := storage.LegacyIdentity
			if version == wrong {
				wrong = storage.IsolatedIdentity
			}
			if _, err := st.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, product, wrong); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("ticket crossed identity version", err)
			}
			if _, err := st.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, "other", version); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatal("unregistered product accepted", err)
			}
			other := "mail"
			if product == other {
				other = "drive"
			}
			if _, err := st.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, other, version); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("ticket crossed product", err)
			}
			if version == storage.IsolatedIdentity {
				if _, err := st.FinishMatrixNativeConsent(ctx, "alice", state, ticket, challenge); !errors.Is(err, domain.ErrForbidden) {
					t.Fatal("legacy endpoint consumed isolated grant", err)
				}
			}
			token, err := st.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, product, version)
			if err != nil || token != "artificial-matrix-login-token" {
				t.Fatal("valid grant could not finish", err)
			}
			if _, err := st.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, product, version); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("grant replay", err)
			}
		}
	}
}
