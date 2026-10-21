// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Estado del aprovisionamiento SCIM contra un PostgreSQL real (pgtest, build-tag
// `integration`). Lo que hay que fijar aquí es lo que la consola llegó a decir
// mal: que SCIM está activo para una organización a la que el token no apunta.
// El token del despliegue es UNO y está ligado a UNA organización, así que para
// todas las demás el aprovisionamiento NO está encendido por más que el proceso
// lo tenga configurado.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestSCIMStatusSoloEnLaOrganizacionDelToken(t *testing.T) {
	l := newLiveServer(t)

	// La organización a la que apunta el token SCIM del despliegue.
	suya, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: itSCIMOrg, Name: "SCIM Org"})
	if err != nil {
		t.Fatalf("CreateOrg(%s): %v", itSCIMOrg, err)
	}
	admin := l.mkUser(t, "admin@scimorg.example", "una-clave-larga-123")
	if err := l.st.UpsertMembership(l.ctx, nil,
		domain.Membership{OrgID: suya.ID, UserID: admin.ID, Role: domain.RoleAdmin}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	// Alguien aprovisionado por SCIM: el `external_id` solo lo escribe esa vía.
	provisto, err := l.st.CreateUser(l.ctx, nil, domain.User{
		Email: "provisto@scimorg.example", EmailVerified: true, Name: "Provisto",
		ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("CreateUser(provisto): %v", err)
	}
	if err := l.st.UpsertMembership(l.ctx, nil,
		domain.Membership{OrgID: suya.ID, UserID: provisto.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership(provisto): %v", err)
	}

	tokSuya := l.token(t, admin.ID, suya.Slug, []string{"admin"})
	rr, out := l.do(t, http.MethodGet, "/api/v1/admin/scim/status", tokSuya, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["configured"] != true {
		t.Fatalf("configured=%v; quería true en la organización del token", out["configured"])
	}
	if n, _ := out["users_provisioned"].(float64); n != 1 {
		t.Fatalf("users_provisioned=%v; quería 1", out["users_provisioned"])
	}
	// La dirección se da SIEMPRE: es el dato que hace falta para configurarlo.
	if base, _ := out["base_url"].(string); base == "" {
		t.Fatal("base_url vacío: es lo que hay que pegar en Entra u Okta")
	}

	// OTRA organización del mismo despliegue: el token no apunta a ella, así que
	// su aprovisionamiento NO está encendido. Decir «activo» aquí es exactamente
	// el distintivo fingido que se retiró de la consola.
	otra, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}
	admin2 := l.mkUser(t, "admin@otra.example", "una-clave-larga-123")
	if err := l.st.UpsertMembership(l.ctx, nil,
		domain.Membership{OrgID: otra.ID, UserID: admin2.ID, Role: domain.RoleAdmin}); err != nil {
		t.Fatalf("UpsertMembership(otra): %v", err)
	}
	tokOtra := l.token(t, admin2.ID, otra.Slug, []string{"admin"})
	rr, out = l.do(t, http.MethodGet, "/api/v1/admin/scim/status", tokOtra, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status(otra): código=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["configured"] != false {
		t.Fatalf("configured=%v; quería false: el token SCIM no apunta a esta organización", out["configured"])
	}
	if n, _ := out["users_provisioned"].(float64); n != 0 {
		t.Fatalf("users_provisioned=%v; quería 0: no cuenta a los de otra organización", out["users_provisioned"])
	}
	// Sin ningún evento `scim.*` en su auditoría, la fecha no se inventa.
	if _, hay := out["last_event_at"]; hay {
		t.Fatalf("last_event_at presente sin ningún evento: %v", out["last_event_at"])
	}
}
