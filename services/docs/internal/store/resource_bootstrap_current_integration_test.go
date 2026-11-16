//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
)

type originalBootstrapPages []resource.BootstrapRosterInput

func (p originalBootstrapPages) BootstrapRosterPage(ctx context.Context, offset resource.Decimal) (resource.BootstrapRosterPage, error) {
	if offset != "0" {
		return resource.BootstrapRosterPage{}, resource.ErrPending
	}
	return resource.BootstrapRosterPage{Offset: "0", Inputs: []resource.BootstrapRosterInput(p)}, ctx.Err()
}

// Live backend fixtures must pass through the complete CURRENT capture verifier;
// retaining an owner-only historical plan can only exercise proof restoration.
func completeOriginalBootstrapCurrent(t testing.TB, ctx context.Context, b resourcetest.BootstrapFixture, source resource.OriginalBootstrapSource, registry *resourcetest.SessionSource) resource.VerifiedBootstrapPlan {
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
	provider, err := resource.NewSessionProvider(registry, registry)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.CompleteBootstrapRosterCapture(ctx, pending, originalBootstrapPages(inputs), provider)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}
