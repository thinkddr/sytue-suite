//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// Uses the SAME actual SDK, isolated recovery vault and original operation
// lease. A historical owner-only proof is valid metadata, never a live binding.
func exerciseNativeOwnerOnlyBootstrapCaptureDenial(t testing.TB, ctx context.Context, c *Client, current resource.VerifiedSession, signed resource.SignedBootstrapPlan, roster resource.VerifiedRoster, source resource.OriginalBootstrapSource, vault *Client, lease offline.WorkLease, journal *offline.Store) {
	t.Helper()
	ownerOnly, err := resource.VerifyBootstrapPlan(ctx, signed, roster, current, source)
	if err != nil {
		t.Fatal("owner-only fixture signature/source unexpectedly invalid", err)
	}
	bypass, err := c.BindResourceBootstrapRoom(ctx, current, ownerOnly, vault, lease, journal)
	if err == nil {
		bypass.Close()
		t.Fatal("native owner-only bootstrap bypassed complete current roster capture")
	}
}
