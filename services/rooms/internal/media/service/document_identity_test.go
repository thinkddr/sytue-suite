// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

func TestCurrentDocumentIdentityDoesNotRestoreStaleAdministration(t *testing.T) {
	original := authn.Claims{Subject: "actor", Roles: []string{"admin", "owner", "member"}}
	denied := currentDocumentIdentity(context.Background(), original, domain.DocumentCallAccess{})
	claims, _ := authn.FromContext(denied)
	if claims.HasAnyRole("admin", "owner") || !claims.HasRole("member") {
		t.Fatalf("stale administrator retained privileges: %+v", claims.Roles)
	}
	if !original.HasRole("owner") || len(original.Roles) != 3 {
		t.Fatal("shared JWT claims were mutated")
	}
	allowed := currentDocumentIdentity(context.Background(), original, domain.DocumentCallAccess{OrganizationAdmin: true})
	claims, _ = authn.FromContext(allowed)
	if !claims.HasRole("admin") {
		t.Fatal("live organization administrator lost privileges")
	}
}
