//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func persistOriginalBootstrapCurrent(t testing.TB, ctx context.Context, st *store.Store, b resourcetest.BootstrapFixture, source resource.OriginalBootstrapSource, registry *resourcetest.SessionSource) (resource.VerifiedBootstrapPlan, *resourcepg.BootstrapCaptureStore) {
	t.Helper()
	manifest, inputs, err := resource.PrepareBootstrapRosterManifest(b.Signed.BootstrapPlan, []resource.BootstrapRosterInput{{Member: b.Roster.Members()[0], Recipient: registry.Record.Recipient}})
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest, Signatures: b.Sign(manifest)}
	pending, err := resource.VerifyBootstrapRosterManifest(ctx, b.Signed, signed, b.Current, source)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.NewResourceBootstrapCaptureStore(st, b.Current.Scope().Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Begin(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Append(ctx, pending, resource.BootstrapRosterPage{Offset: "0", Inputs: inputs}); err != nil {
		t.Fatal(err)
	}
	provider, err := resource.NewSessionProvider(registry, registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := capture.Finalize(ctx, pending, provider)
	if err != nil {
		t.Fatal(err)
	}
	return plan, capture
}

type originalPolicyVaultFixture struct{ anchor resource.Anchor }

func (v originalPolicyVaultFixture) BootstrapPolicyAnchor(ctx context.Context, key resource.Resource, _ string) (resource.Anchor, error) {
	if key != v.anchor.Resource {
		return resource.Anchor{}, resource.ErrRevoked
	}
	return v.anchor, ctx.Err()
}

type originalPolicyAuthorityVaultFixture struct {
	raw     []byte
	current resource.VerifiedSession
}

func (v originalPolicyAuthorityVaultFixture) BootstrapPolicyAuthority(ctx context.Context, key resource.Resource, plan string) (resource.BootstrapPolicyAuthorityEvidence, error) {
	if err := v.current.Revalidate(ctx); err != nil {
		return resource.BootstrapPolicyAuthorityEvidence{}, err
	}
	return resource.DecodeBootstrapPolicyAuthority(ctx, v.raw, key, plan, v.current.Binding())
}

// ORIGINAL Docs freeze/ACL/RLS and accepted SQL rows are real. Registry consent
// is a production SessionProvider using a synthetic registry witness; Megolm
// acceptance and the independent vault witness are metadata fixtures here.
func exerciseAcceptedPolicyRecovery(t testing.TB, ctx context.Context, pool *pg.Pool, capture *resourcepg.BootstrapCaptureStore, b resourcetest.BootstrapFixture, activation resource.VerifiedBootstrapActivation, cap resource.SignedCapability, old *resourcetest.SessionSource) {
	t.Helper()
	anchor := activation.Anchor()
	key, planID := b.Signed.Source.Resource, b.Signed.ID
	archived, err := resource.CaptureBootstrapPlan(ctx, b.Verified)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor resource.BootstrapPlanEvidence
	if json.Unmarshal(archived.EvidenceBytes(), &descriptor) != nil || len(descriptor.Recipients) != 1 {
		t.Fatal("original owner roster transcript")
	}
	capRaw, err := matrixproof.CanonicalBytes(cap)
	if err != nil {
		t.Fatal(err)
	}
	capSHA, err := resource.CipherJSONDigest(capRaw)
	if err != nil {
		t.Fatal(err)
	}
	// The original signed owner capability is also pinned in the authenticated
	// private recovery capsule BEFORE A retires. No Source normal-op row exists.
	capsule, err := resource.EncodeBootstrapPolicyAnchor(ctx, activation, cap)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(capsule)
	// Keep the original public evidence reference for the separate legacy path;
	// it is not inserted into SQL until the capsule-only restore below succeeds.
	ref := resource.CapabilityEvidence{Capability: resource.StatementEvidence{StatementSHA256: capSHA, Signer: descriptor.Plan.Signer}, Recipient: descriptor.Recipients[0], IssuerEnrollment: descriptor.Recipients[0]}
	chain := matrixprooftest.NewDevice(t, b.Base.Chain, "SQL_CURRENT_POLICY_B")
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, registry := resourcetest.EnrolledSession(t, chain, leaf)
	old.Retire()
	b.Base.Chain.Origin.DeviceActive = false
	view, err := capture.AcceptedPolicySource(ctx, current, key, planID)
	if err != nil {
		t.Fatal("Original SQL required live retired A or retained policy", err)
	}
	retained, err := view.RetainedBootstrapPolicy(ctx, key, planID)
	if err != nil || retained.State != "active" || retained.Baseline.State != "accepted" || retained.Checkpoint.State != "accepted" || retained.Capture.OwnerPin.DeviceID.String() == current.Actor().DeviceID {
		t.Fatal("accepted original view changed historical actor/ACK", err)
	}
	if _, err := view.PublicEvidence(ctx, key, capSHA); err == nil {
		t.Fatal("bootstrap-only Source already retained a normal-operation capability")
	}
	private := originalPolicyAuthorityVaultFixture{raw: capsule, current: current}
	fromVault, err := resource.RestoreBootstrapPolicyFromVault(ctx, key, planID, current, private, view)
	if err != nil {
		t.Fatal("Original SQL required a prior normal operation to restore private signed owner grant", err)
	}
	if access, err := fromVault.Access(ctx, resource.Read); err != nil || access.Actor() != current.Actor() {
		t.Fatal("private owner grant did not preserve actual current B", err)
	}
	// Keep the legacy archived-public-capability path separately exercised.
	// This insertion occurs ONLY after the zero-normal-operation vault restore.
	if err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO resource_e2e_public_evidence(tenant,product,sha256,value)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, key.Tenant, key.Product, capSHA, capRaw)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	policy, err := resource.RestoreBootstrapPolicy(ctx, key, planID, ref, current, originalPolicyVaultFixture{anchor}, view)
	if err != nil {
		t.Fatal("fresh policy from Original SQL/current B/public SDK transcript", err)
	}
	reader, err := policy.Access(ctx, resource.Read)
	if err != nil || reader.Actor() != current.Actor() {
		t.Fatal("recovered authority lost current B", err)
	}
	foreign := key
	foreign.Tenant = "other"
	if _, err := view.RetainedBootstrapPolicy(tenantCtx("other"), foreign, planID); err == nil {
		t.Fatal("cross-tenant accepted view escaped original binding")
	}
	resetChain := matrixprooftest.NewUser(t, current.Scope(), chain.Origin.ServerName, chain.Origin.UserID, "RESET_POLICY_ROOT")
	resetLeaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	reset, _ := resourcetest.EnrolledSession(t, resetChain, resetLeaf)
	if _, err := capture.AcceptedPolicySource(ctx, reset, key, planID); err == nil {
		t.Fatal("root reset silently opened original accepted metadata")
	}
	registry.Retire()
	if err := reader.RevalidateSession(ctx); err == nil {
		t.Fatal("Source fence dropped current Provider after restore")
	}
}
