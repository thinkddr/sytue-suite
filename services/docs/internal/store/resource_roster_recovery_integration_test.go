//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

// Every token is produced through the same SDK verifiers and ORIGINAL Source.
// This helper never constructs a private Verified type or fakes a Sending hold.
func prepareOriginalCaptureBCancellation(t testing.TB, ctx context.Context, capture *resourcepg.BootstrapCaptureStore, pending resource.VerifiedBootstrapRosterManifest, current resource.VerifiedSession, chain *matrixprooftest.Fixture, original resource.OriginalBootstrapSource) resource.VerifiedBootstrapCaptureCancellation {
	t.Helper()
	source, err := capture.HistoricalSource(ctx, current, pending.Plan().Source.Resource, pending.Plan().ID)
	if err != nil {
		t.Fatal("current owner historical Source", err)
	}
	old, err := resource.RestoreBootstrapCapture(ctx, pending.Plan().Source.Resource, pending.Plan().ID, source)
	if err != nil {
		t.Fatal("original captured SDK evidence", err)
	}
	query, err := resource.PrepareBootstrapCaptureCancellation(ctx, current, old)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedBootstrapCaptureCancellation{BootstrapCaptureCancellation: query, Signatures: matrixprooftest.Sign(t, chain.Device, query, chain.Origin.UserID, current.Actor().DeviceID)}
	verified, err := resource.VerifyBootstrapCaptureCancellation(ctx, signed, current, old, original)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}
