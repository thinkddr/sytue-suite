//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestResourceNativePurposeNeedsVisibleApprovalAndConsumesExactPKCE(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	state, err := s.CreateMatrixResourceNativeConsent(ctx, "alice", challenge, "docs", "sytue-desktop")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewMatrixNativeDestination(ctx, "alice", state)
	if err != nil || preview.Purpose != "resources:docs" || preview.ResourceApproved {
		t.Fatal("native purpose was not fixed before SSO", err)
	}
	if _, err := s.AcceptMatrixNativeConsent(ctx, "alice", state, "synthetic-before-visible-resource-consent"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("SSO callback bypassed visible resource consent", err)
	}
	for _, test := range []struct{ actor, tenant string }{{"bob", "acme"}, {"alice", "other"}} {
		if err := s.ApproveMatrixResourceNativeConsent(tenantCtx(test.tenant), test.actor, state); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("another actor/tenant approved", err)
		}
	}
	if err := s.ApproveMatrixResourceNativeConsent(ctx, "alice", state); err != nil {
		t.Fatal(err)
	}
	if err := s.ApproveMatrixResourceNativeConsent(ctx, "alice", state); err != nil {
		t.Fatal("identical approval not idempotent", err)
	}
	ticket, original, err := s.AcceptMatrixNativeConsentWithDestination(ctx, "alice", state, "synthetic-resource-login-token")
	if err != nil || !original.ResourceApproved || original.Purpose != "resources:docs" {
		t.Fatal(err)
	}
	if _, err := s.FinishMatrixNativeConsentForIdentity(ctx, "alice", state, ticket, challenge, "docs", storage.LegacyIdentity); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("legacy finish stole resource authority", err)
	}
	for _, test := range []struct{ actor, client, pkce string }{{"alice", "sytue-mobile", challenge}, {"bob", "sytue-desktop", challenge}, {"alice", "sytue-desktop", "wrong-challenge"}} {
		if _, err := s.FinishMatrixResourceNativeConsent(ctx, test.actor, state, ticket, test.pkce, test.client); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("resource finish crossed original actor/client/PKCE", err)
		}
	}
	// Rolling-deploy code knows the consumer flag but not the resource contract.
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_native_consumer_contract','1',true)`); err != nil {
			return err
		}
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(state)).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			return errors.New("old binary saw resource grant under maintenance")
		}
		command, err := tx.Exec(ctx, `DELETE FROM matrix_native_consents WHERE state_hash=$1`, domain.HashToken(state))
		if err == nil && command.RowsAffected() != 0 {
			return errors.New("old binary consumed resource grant")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	grant, err := s.FinishMatrixResourceNativeConsent(ctx, "alice", state, ticket, challenge, "sytue-desktop")
	if err != nil || grant.LoginToken != "synthetic-resource-login-token" || grant.Product != "docs" || grant.Namespace != "resources:docs" || grant.ConsentID == "" || grant.Grant == "" {
		t.Fatal("exact approved exchange did not create scoped grant", err)
	}
	if _, err := s.FinishMatrixResourceNativeConsent(ctx, "alice", state, ticket, challenge, "sytue-desktop"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("consumed PKCE replayed", err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_subject','alice',true),set_config('app.resource_session_product','docs',true)`); err != nil {
			return err
		}
		var product, namespace, client, hash string
		var account, tenant int64
		if err := tx.QueryRow(ctx, `SELECT product,namespace,client_id,grant_hash,account_version,tenant_version FROM matrix_resource_consents WHERE id=$1`, grant.ConsentID).Scan(&product, &namespace, &client, &hash, &account, &tenant); err != nil {
			return err
		}
		if product != "docs" || namespace != "resources:docs" || client != "sytue-desktop" || hash != domain.HashToken(grant.Grant) || account != 0 || tenant != 0 {
			t.Fatal("grant came from finish body or lost lifecycle binding")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResourceNativeLegacyPurposeCannotMintResourceGrantAndDenialRetiresPurpose(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	legacy, err := s.CreateMatrixNativeConsentFor(ctx, "alice", challenge, "docs")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.AcceptMatrixNativeConsent(ctx, "alice", legacy, "synthetic-legacy-purpose-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishMatrixResourceNativeConsent(ctx, "alice", legacy, ticket, challenge, "sytue-desktop"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("legacy purpose promoted to resource grant", err)
	}
	if token, err := s.FinishMatrixNativeConsentForIdentity(ctx, "alice", legacy, ticket, challenge, "docs", storage.LegacyIdentity); err != nil || token != "synthetic-legacy-purpose-token" {
		t.Fatal("legacy exchange regressed", err)
	}
	state, err := s.CreateMatrixResourceNativeConsent(ctx, "alice", challenge, "docs", "sytue-desktop")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMatrixAccountAccess(ctx, "alice", "suspended", 1, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMatrixAccountAccess(ctx, "alice", "active", 2, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewMatrixNativeDestination(ctx, "alice", state); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("reactivation revived old purpose", err)
	}
}
