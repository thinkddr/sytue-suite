//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

// Reuses the actual Synapse SDK account/session and strict non-reentrant lease.
// No signing seam or replacement SDK is used for this source query.
func exerciseNativeResourceSourceQuery(t testing.TB, ctx context.Context, c *Client, current resource.VerifiedSession, key resource.Resource, room string, lease offline.WorkLease) {
	t.Helper()
	signed, err := c.SignResourceBootstrapSourceQuery(ctx, current, key, room, lease)
	if err != nil {
		t.Fatal("native current source SDK signature", err)
	}
	verified, err := resource.VerifyBootstrapSourceQuery(ctx, signed, current)
	if err != nil || verified.Value().Resource != key || verified.Value().Actor != current.Actor() || verified.Value().Target.RoomID != room {
		t.Fatal("native source query binding", err)
	}
	substituted := signed
	substituted.Resource.ID = "another-resource"
	if _, err := resource.VerifyBootstrapSourceQuery(ctx, substituted, current); err == nil {
		t.Fatal("native source signature did not bind resource")
	}
	if _, err := c.SignResourceBootstrapSourceQuery(ctx, current, key, room, nil); err == nil {
		t.Fatal("source signature dropped original work lease")
	}
	foreign := key
	foreign.Tenant = "another-tenant"
	if _, err := c.SignResourceBootstrapSourceQuery(ctx, current, foreign, room, lease); err == nil {
		t.Fatal("native source signature crossed tenant")
	}
}
