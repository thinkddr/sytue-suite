// SPDX-License-Identifier: Apache-2.0
package store

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestCalendarIngressUnverifiedIdentityCannotReachDatabaseOrAuthority(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	for _, fault := range []string{"anonymous", "delegated", "subject", "tenant", "aliases"} {
		t.Run(fault, func(t *testing.T) {
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
			claims := authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject, Email: "alice@example.invalid"}
			switch fault {
			case "delegated":
				claims.Delegado = true
			case "subject":
				claims.Subject = "mallory"
			case "tenant":
				claims.Tenant = "foreign"
			case "aliases":
				claims.Email = ""
			}
			if fault != "anonymous" {
				ctx = authn.Into(ctx, claims)
			}
			authority := struct {
				callaccess.PrivateIngressAuthority
			}{}
			if _, err := New(nil).PrivateIngressQueue(authority).Pending(ctx, scope, "11111111-1111-4111-8111-111111111111", "", 10); err == nil {
				t.Fatal("unverified Calendar identity admitted")
			}
		})
	}
}
