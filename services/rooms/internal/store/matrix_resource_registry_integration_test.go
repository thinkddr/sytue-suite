//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Only directory liveness is a synthetic witness. SDK chain/signatures, hybrid
// decapsulation, durable consent, RLS, first-root decision and registry/provider
// are production implementations. This does not claim a live Synapse flow.
type resourceRegistryDirectory struct {
	matrixGate
	devices map[string]*matrixprooftest.Fixture
}

func (g *resourceRegistryDirectory) ResourceSessionOrigin(ctx context.Context, scope storage.Scope, device string) (matrixproof.Origin, error) {
	f := g.devices[device]
	if f == nil || f.Origin.Scope != scope {
		return matrixproof.Origin{}, domain.ErrForbidden
	}
	return f.Origin, nil
}

func (g *resourceRegistryDirectory) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	origin, err := g.ResourceSessionOrigin(ctx, scope, device)
	if err != nil {
		return matrixproof.Device{}, err
	}
	return matrixproof.VerifyUser(ctx, origin, origin.UserID, id.Ed25519(root))
}

func registryRecipient(t testing.TB, f *matrixprooftest.Fixture, key *pqc.Identity) resource.SignedRecipient {
	t.Helper()
	keyID, err := pqc.RecipientID(key.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: f.Origin.Scope, UserID: f.Origin.UserID.String(), Recipient: key.Recipient().String(), KeyID: keyID, Issuer: resource.Actor{UserID: f.Origin.UserID.String(), DeviceID: f.Origin.DeviceID.String(), RootKey: f.Master.PublicKey().String()}}}
	signed.Signatures = matrixprooftest.Sign(t, f.Device, signed, f.Origin.UserID, f.Origin.DeviceID.String())
	return signed
}

func registryEnrollment(t testing.TB, f *matrixprooftest.Fixture, key *pqc.Identity, challenge resource.SessionChallenge) resource.SignedSessionEnrollment {
	t.Helper()
	nonce, err := resource.OpenSessionChallenge(t.Context(), challenge, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	decision := "retain_confirmed_root"
	if challenge.RequiresRootConfirmation {
		decision = "confirm_current_root"
	}
	unsigned, err := resource.PrepareSessionEnrollment(challenge, nonce, decision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedSessionEnrollment{SessionEnrollment: unsigned}
	signed.Signatures = matrixprooftest.Sign(t, f.Device, signed, f.Origin.UserID, f.Origin.DeviceID.String())
	return signed
}

func TestResourceSessionRegistryRequiresConsumedPurposePossessionAndLiveSDK(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := store.New(h.Pool, testCipher(t))
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "docs"}
	a := matrixprooftest.NewUser(t, scope, "matrix.test", id.UserID("@sytue_"+scope.Subject+":matrix.test"), "DOCS_A")
	directory := &resourceRegistryDirectory{devices: map[string]*matrixprooftest.Fixture{"DOCS_A": a}}
	s.SetMatrixGate(directory)
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recipient := registryRecipient(t, a, key)
	if _, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-mail"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("Mail minted Docs consent", err)
	}
	if _, err := s.ResourceSessionChallenge(ctx, scope, "invented-body-grant", "sytue-docus", recipient); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("public SDK root alone enrolled Docs", err)
	}
	grant, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := s.ResourceSessionChallenge(ctx, scope, grant.Grant, "sytue-docus", recipient)
	if err != nil || !challenge.RequiresRootConfirmation {
		t.Fatal("first root not explicitly retained", err)
	}
	same, err := s.ResourceSessionChallenge(ctx, scope, grant.Grant, "sytue-docus", recipient)
	if err != nil || same.ID != challenge.ID {
		t.Fatal("lost challenge ACK regenerated nonce", err)
	}
	proof := registryEnrollment(t, a, key, challenge)
	unsigned := proof
	unsigned.Signatures = nil
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", unsigned); err == nil {
		t.Fatal("unsigned root confirmation pinned authority")
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-mail", proof); err == nil {
		t.Fatal("another client consumed Docs challenge")
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", proof); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", proof); err != nil {
		t.Fatal("identical confirmation not idempotent", err)
	}
	provider, err := s.ResourceSessionProvider()
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(ctx, scope, "DOCS_A")
	if err != nil || current.Actor().DeviceID != "DOCS_A" {
		t.Fatal("live registry+SDK did not resolve", err)
	}
	if err := current.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Resolve(ctx, storage.Scope{Tenant: "acme", Subject: scope.Subject, Product: "mail"}, "DOCS_A"); err == nil {
		t.Fatal("product-scoped device crossed into Mail")
	}
	// Fresh B has the SAME canonical user/root, but a distinct leaf hybrid recipient,
	// its own SDK key and independently consumed Docs consent. A's original evidence is
	// retained, never relabelled as B or replaced in the role-grant history.
	var firstRootSHA string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_subject',$1,true),set_config('app.resource_session_product','docs',true)`, scope.Subject); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT enrollment_sha256 FROM matrix_resource_authorities WHERE subject=$1`, scope.Subject).Scan(&firstRootSHA)
	}); err != nil {
		t.Fatal(err)
	}
	b := matrixprooftest.NewDevice(t, a, "DOCS_B")
	directory.devices["DOCS_B"] = b
	a.Origin.DeviceActive = false
	if err := current.Revalidate(ctx); err == nil {
		t.Fatal("previously resolved A survived SDK device retirement")
	}
	if _, err := provider.Resolve(ctx, scope, "DOCS_A"); err == nil {
		t.Fatal("retired A remained current")
	}
	clonedGrant, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	clonedChallenge, err := s.ResourceSessionChallenge(ctx, scope, clonedGrant.Grant, "sytue-docus", registryRecipient(t, b, key))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", registryEnrollment(t, b, key, clonedChallenge)); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("B reused A leaf recipient despite separate SDK consent", err)
	}
	bGrant, err := s.CreateMatrixResourceWebConsent(ctx, scope.Subject, "docs", "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	bKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	bRecipient := registryRecipient(t, b, bKey)
	bChallenge, err := s.ResourceSessionChallenge(ctx, scope, bGrant.Grant, "sytue-docus", bRecipient)
	if err != nil || bChallenge.RequiresRootConfirmation {
		t.Fatal("fresh B replaced existing root", err)
	}
	if _, err := resource.OpenSessionChallenge(ctx, bChallenge, key, time.Now()); err == nil {
		t.Fatal("retired A opened B leaf recipient challenge")
	}
	bProof := registryEnrollment(t, b, bKey, bChallenge)
	if _, err := s.ConfirmResourceSession(ctx, scope, "sytue-docus", bProof); err != nil {
		t.Fatal(err)
	}
	recovered, err := provider.Resolve(ctx, scope, "DOCS_B")
	if err != nil || recovered.Actor().DeviceID != "DOCS_B" || recovered.Binding().ProductKeyID == current.Binding().ProductKeyID || recovered.Actor().RootKey != current.Actor().RootKey {
		t.Fatal("fresh B did not recover its own product identity", err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_subject',$1,true),set_config('app.resource_session_product','docs',true)`, scope.Subject); err != nil {
			return err
		}
		var after string
		if err := tx.QueryRow(ctx, `SELECT enrollment_sha256 FROM matrix_resource_authorities WHERE subject=$1`, scope.Subject).Scan(&after); err != nil {
			return err
		}
		if after != firstRootSHA {
			t.Fatal("B rewrote first-root evidence A")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMatrixAccountAccess(ctx, scope.Subject, "suspended", 1, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMatrixAccountAccess(ctx, scope.Subject, "active", 2, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Revalidate(ctx); err == nil {
		t.Fatal("reactivation revived old product session generation")
	}
}
