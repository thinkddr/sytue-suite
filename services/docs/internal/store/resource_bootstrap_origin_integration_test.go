//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

func bootstrapOriginSigned(t testing.TB, f resourcetest.Fixture, plan resource.VerifiedBootstrapPlan, current resource.VerifiedSession, c resource.BootstrapCandidate) publication.SignedRequest {
	t.Helper()
	value := plan.Value()
	generation, err := current.Binding().Generation.Int64()
	if err != nil {
		t.Fatal(err)
	}
	req := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: current.Binding().Namespace, Scope: current.Scope(), Target: storage.PublicationTarget{RoomID: value.RoomID, DeviceID: value.Creator.DeviceID}, TransactionID: c.Proof.TransactionID, CipherSHA256: c.Proof.OuterCipherSHA256, AuthorityGeneration: generation}
	return publication.SignedRequest{Request: req, Signatures: matrixprooftest.Sign(t, f.Chain.Device, req, f.Chain.Origin.UserID, value.Creator.DeviceID)}
}

func bootstrapOriginPreparedDenied(t testing.TB, ctx context.Context, r *resourcepg.Relay, f resourcetest.Fixture, plan resource.VerifiedBootstrapPlan, current resource.VerifiedSession, c resource.BootstrapCandidate, verified resource.VerifiedBootstrapCandidate) *resourcepg.BootstrapOriginSource {
	t.Helper()
	source, err := r.BootstrapOriginSource(ctx, plan)
	if err != nil {
		t.Fatal("fixed original bootstrap source", err)
	}
	signed := bootstrapOriginSigned(t, f, plan, current, c)
	if _, err = source.BootstrapOriginHold(ctx, plan.Value().Source.Resource, plan.Value().ID, resource.BootstrapBaseline); err == nil {
		t.Fatal("prepared original slot exposed as send hold")
	}
	if _, err = source.VerifyRequest(ctx, verified, current, signed); err == nil {
		t.Fatal("prepared slot became origin send authority")
	}
	return source
}

func bootstrapOriginAmbiguousAllowed(t testing.TB, ctx context.Context, source *resourcepg.BootstrapOriginSource, f resourcetest.Fixture, plan resource.VerifiedBootstrapPlan, current resource.VerifiedSession, c resource.BootstrapCandidate, verified resource.VerifiedBootstrapCandidate) resource.VerifiedBootstrapOriginRequest {
	t.Helper()
	signed := bootstrapOriginSigned(t, f, plan, current, c)
	permit, err := source.VerifyRequest(ctx, verified, current, signed)
	if err != nil || permit.Request().Value() != signed.Request || permit.SessionBinding() != current.Binding() || permit.PlanSHA256() != plan.SHA256() {
		t.Fatal("real original Sending/Ambiguous denied", err)
	}
	key := plan.Value().Source.Resource
	key.Tenant = "foreign"
	if _, err := source.BootstrapOriginHold(ctx, key, plan.Value().ID, resource.BootstrapBaseline); err == nil {
		t.Fatal("source replaced owner resource tenant")
	}
	if _, err := source.BootstrapOriginHold(ctx, plan.Value().Source.Resource, "00000000-0000-4000-8000-000000000001", resource.BootstrapBaseline); err == nil {
		t.Fatal("source replaced plan")
	}
	return permit
}
