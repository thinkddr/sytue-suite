// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMatrixNativeConsumerIsImmutableAndCannotUseLegacyExchange(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	service := store.New(harness.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	state, err := service.CreateMatrixNativeConsentForConsumer(ctx, "alice", challenge, "drive", "pdf", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewMatrixNativeDestination(ctx, "alice", state)
	if err != nil || preview.Product != "drive" || preview.NativeConsumer != "pdf" || preview.IdentityVersion != storage.IsolatedIdentity {
		t.Fatal("consumer grant lost immutable destination", err)
	}
	ticket, destination, err := service.AcceptMatrixNativeConsentWithDestination(ctx, "alice", state, "synthetic-native-consumer-login-token")
	if err != nil || destination != preview {
		t.Fatal("accept changed consumer grant", err)
	}
	if _, err := service.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, "drive", storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("legacy client consumed consumer grant", err)
	}
	for _, test := range []struct {
		actor, tenant, consumer, challenge string
		version                            storage.IdentityVersion
	}{
		{"alice", "acme", "docs", challenge, storage.IsolatedIdentity},
		{"alice", "acme", "pdf", challenge, storage.LegacyIdentity},
		{"bob", "acme", "pdf", challenge, storage.IsolatedIdentity},
		{"alice", "other", "pdf", challenge, storage.IsolatedIdentity},
		{"alice", "acme", "pdf", "wrong-challenge", storage.IsolatedIdentity},
	} {
		if _, err := service.FinishMatrixNativeConsentForConsumer(tenantCtx(test.tenant), test.actor, state, ticket, test.challenge, "drive", test.consumer, test.version); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("consumer grant crossed authority", test, err)
		}
	}
	token, err := service.FinishMatrixNativeConsentForConsumer(ctx, "alice", state, ticket, challenge, "drive", "pdf", storage.IsolatedIdentity)
	if err != nil || token != "synthetic-native-consumer-login-token" {
		t.Fatal("exact consumer grant could not finish", err)
	}
	if _, err := service.FinishMatrixNativeConsentForConsumer(ctx, "alice", state, ticket, challenge, "drive", "pdf", storage.IsolatedIdentity); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("consumer grant replay", err)
	}
	for _, test := range []struct{ product, consumer string }{{"drive", "drive"}, {"drive", "pdf "}, {"drive", "unregistered"}, {"mail", "pdf"}} {
		if _, err := service.CreateMatrixNativeConsentForConsumer(ctx, "alice", challenge, test.product, test.consumer, storage.IsolatedIdentity); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatal("arbitrary consumer route accepted", test, err)
		}
	}
}

func TestMatrixNativeConsumerRollbackSeesAnotherTenantBeforeDroppingBinding(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	service := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	state, err := service.CreateMatrixNativeConsentForConsumer(ctx, "alice", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "drive", "pdf", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	down, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0068_matrix_native_consumers.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	app, err := pgx.Connect(t.Context(), h.AppDSN)
	if err != nil {
		t.Fatal("open restricted test role", err)
	}
	t.Cleanup(func() { _ = app.Close(context.WithoutCancel(t.Context())) })
	if _, err = app.Exec(t.Context(), `SELECT set_config('app.tenant','other',false)`); err != nil {
		t.Fatal(err)
	}
	_, err = app.Exec(t.Context(), string(down))
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "P0001" || failure.Message != "native consumer grants must expire or be retired before rollback" {
		t.Fatal("rollback failed to see a foreign-tenant grant through its maintenance gate", err)
	}
	if _, err = app.Exec(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if destination, err := service.PreviewMatrixNativeDestination(ctx, "alice", state); err != nil || destination.NativeConsumer != "pdf" {
		t.Fatal("failed rollback changed the consumer binding", err)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal("open migration test role", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.WithoutCancel(t.Context())) })
	if _, err = admin.Exec(t.Context(), `DELETE FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(state)); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(t.Context(), string(down)); err != nil {
		t.Fatal("empty consumer grants could not roll back", err)
	}
}

// A rolling-deploy peer still runs the old SELECT/UPDATE/DELETE statements.
// Even its maintenance identity cannot observe or consume new consumer grants.
func TestMatrixNativeConsumerFenceRejectsOldBinaryButKeepsLegacyGrants(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	service := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	state, err := service.CreateMatrixNativeConsentForConsumer(ctx, "alice", challenge, "drive", "pdf", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	ticket, _, err := service.AcceptMatrixNativeConsentWithDestination(ctx, "alice", state, "synthetic-old-binary-fence-token")
	if err != nil {
		t.Fatal(err)
	}
	old, err := pgx.Connect(t.Context(), h.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Close(context.WithoutCancel(t.Context())) })
	if _, err = old.Exec(t.Context(), `SELECT set_config('app.tenant','acme',false),set_config('app.maintenance','on',false)`); err != nil {
		t.Fatal(err)
	}
	var secret string
	err = old.QueryRow(t.Context(), `SELECT login_secret FROM matrix_native_consents WHERE tenant=$1 AND subject=$2 AND state_hash=$3 AND ticket_hash=$4 AND challenge=$5 AND identity_version=$6 AND product=$7 AND expires_at>now() AND ticket_expires_at>now() AND login_secret<>'' FOR UPDATE`, "acme", "alice", domain.HashToken(state), domain.HashToken(ticket), challenge, storage.IsolatedIdentity, "drive").Scan(&secret)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("old binary observed consumer grant through maintenance policy", err)
	}
	for _, query := range []string{`UPDATE matrix_native_consents SET login_secret=login_secret WHERE state_hash=$1`, `DELETE FROM matrix_native_consents WHERE state_hash=$1`} {
		result, err := old.Exec(t.Context(), query, domain.HashToken(state))
		if err != nil || result.RowsAffected() != 0 {
			t.Fatal("old binary changed a native consumer grant", err)
		}
	}
	legacy, err := service.CreateMatrixNativeConsentForIdentity(ctx, "alice", challenge, "drive", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	var product string
	if err = old.QueryRow(t.Context(), `SELECT product FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(legacy)).Scan(&product); err != nil || product != "drive" {
		t.Fatal("consumer fence hid compatible owner grant", err)
	}
	if token, err := service.FinishMatrixNativeConsentForConsumer(ctx, "alice", state, ticket, challenge, "drive", "pdf", storage.IsolatedIdentity); err != nil || token != "synthetic-old-binary-fence-token" {
		t.Fatal("new contract could not exchange fenced consumer grant", err)
	}
}

func TestMatrixNativeConsumerOldRevocationCannotReviveGrantAfterReactivation(t *testing.T) {
	for _, kind := range []string{"identity", "tenant"} {
		t.Run(kind, func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			service := store.New(h.Pool, testCipher(t))
			ctx := tenantCtx("acme")
			create := func(tenant, subject string) string {
				state, err := service.CreateMatrixNativeConsentForConsumer(tenantCtx(tenant), subject, "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "drive", "pdf", storage.IsolatedIdentity)
				if err != nil {
					t.Fatal(err)
				}
				return state
			}
			own, otherTenant, otherActor := create("acme", "alice"), create("beta", "alice"), create("acme", "bob")
			old, err := pgx.Connect(t.Context(), h.AppDSN)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = old.Close(context.WithoutCancel(t.Context())) })
			if _, err = old.Exec(t.Context(), `SELECT set_config('app.tenant','acme',false),set_config('app.maintenance','on',false)`); err != nil {
				t.Fatal(err)
			}
			var deny, restore string
			if kind == "identity" {
				deny = `UPDATE matrix_identity_access SET status='suspended',version=1,applied=true WHERE subject='alice'`
				restore = `UPDATE matrix_identity_access SET status='active',version=2,applied=true WHERE subject='alice'`
			} else {
				deny = `UPDATE matrix_tenant_access SET status='removed',version=1,applied=true WHERE tenant='acme' AND subject='alice'`
				restore = `UPDATE matrix_tenant_access SET status='active',version=2,applied=true WHERE tenant='acme' AND subject='alice'`
			}
			if _, err = old.Exec(t.Context(), deny); err != nil {
				t.Fatal(err)
			}
			if _, err = service.PreviewMatrixNativeDestination(ctx, "alice", own); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("durable denial did not block consumer grant", err)
			}
			if _, err = old.Exec(t.Context(), restore); err != nil {
				t.Fatal(err)
			}
			if _, err = service.PreviewMatrixNativeDestination(ctx, "alice", own); !errors.Is(err, domain.ErrForbidden) {
				t.Fatal("consumer grant revived after old binary revoked and reactivated owner", err)
			}
			_, err = service.PreviewMatrixNativeDestination(tenantCtx("beta"), "alice", otherTenant)
			if kind == "identity" && !errors.Is(err, domain.ErrForbidden) || kind == "tenant" && err != nil {
				t.Fatal("revocation crossed or omitted its tenant boundary", err)
			}
			if _, err = service.PreviewMatrixNativeDestination(ctx, "bob", otherActor); err != nil {
				t.Fatal("revocation removed another actor's grant", err)
			}
			var count int
			if err = old.QueryRow(t.Context(), `SELECT count(*) FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(otherActor)).Scan(&count); err != nil || count != 0 {
				t.Fatal("revocation leaked consumer contract capability to older connection", count, err)
			}
		})
	}
}
