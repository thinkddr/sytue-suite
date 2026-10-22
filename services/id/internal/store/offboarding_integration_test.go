// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del INVARIANTE DE BAJA DE ORGANIZACIÓN contra PostgreSQL real
// (pgtest): quitar a alguien de una organización le REVOCA las credenciales de esa
// org, no solo su fila de membresía.
//
// Hallazgo de la caza del patrón «autorización apoyada en la fila equivocada» (la
// misma clase que 46b9fde, 89849f7 y 16c3b70, pero un nivel más arriba y con alcance
// de TODA la suite): RemoveMembership borraba memberships y group_members y dejaba
// vivo el refresh token con su `org_id`. Como userClaims resuelve el `tenant` con
// GetOrgByID —que solo comprueba que la organización exista, NO la membresía—, a la
// persona dada de baja se le seguía emitiendo un access token con el claim `tenant`
// de esa org, y de ese claim deriva su aislamiento toda la suite.
//
// La verja complementaria (revalidar la membresía en cada canje de refresh) vive en
// http/token.go: grantRefresh. Aquí se fija la parte de datos.
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestRemoveMembershipRevokesOrgCredentials: la baja de una org revoca los refresh
// tokens de ESA org y deja intactos los de otra a la que el usuario sigue perteneciendo.
func TestRemoveMembershipRevokesOrgCredentials(t *testing.T) {
	st, ctx := newStore(t)

	acme, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg(acme): %v", err)
	}
	otra, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}
	ada := seedUser(t, ctx, st, "ada@acme.example")

	for _, org := range []domain.Organization{acme, otra} {
		if err := st.UpsertMembership(ctx, nil, domain.Membership{
			OrgID: org.ID, UserID: ada.ID, Role: domain.RoleMember,
		}); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", org.Slug, err)
		}
	}

	// Un refresh token por org, como los que emite el canje real.
	mkRefresh := func(orgID, raw string) string {
		t.Helper()
		id, err := st.InsertRefreshToken(ctx, nil, domain.RefreshToken{
			TokenHash: hashOf(raw),
			ClientID:  "sytue-console",
			UserID:    ada.ID,
			OrgID:     orgID,
			Scope:     "openid profile",
			AuthTime:  time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("InsertRefreshToken(%s): %v", orgID, err)
		}
		return id
	}
	mkRefresh(acme.ID, "rt-acme")
	mkRefresh(otra.ID, "rt-otra")

	// Antes de la baja, los dos sirven.
	for _, raw := range []string{"rt-acme", "rt-otra"} {
		rt, err := st.GetRefreshToken(ctx, nil, hashOf(raw))
		if err != nil {
			t.Fatalf("GetRefreshToken(%s): %v", raw, err)
		}
		if rt.RevokedAt != nil {
			t.Fatalf("%s no debería estar revocado antes de la baja", raw)
		}
	}

	// BAJA de acme.
	if err := st.RemoveMembership(ctx, nil, acme.ID, ada.ID); err != nil {
		t.Fatalf("RemoveMembership: %v", err)
	}

	// El de acme queda revocado…
	rtAcme, err := st.GetRefreshToken(ctx, nil, hashOf("rt-acme"))
	if err != nil {
		t.Fatalf("GetRefreshToken(rt-acme) tras la baja: %v", err)
	}
	if rtAcme.RevokedAt == nil {
		t.Fatal("la baja de la org DEBE revocar el refresh token de esa org: sin ello se siguen mintando access tokens con su claim tenant")
	}

	// …y el de la otra org NO: salir de una no cierra la sesión en las demás.
	rtOtra, err := st.GetRefreshToken(ctx, nil, hashOf("rt-otra"))
	if err != nil {
		t.Fatalf("GetRefreshToken(rt-otra): %v", err)
	}
	if rtOtra.RevokedAt != nil {
		t.Fatal("la baja de una org NO debe revocar las credenciales de otra org del mismo usuario")
	}

	// Y la membresía ya no existe (lo que hace fallar la verja de grantRefresh).
	if _, err := st.GetMembership(ctx, acme.ID, ada.ID); err == nil {
		t.Fatal("la membresía de acme debería haber desaparecido")
	}
}
