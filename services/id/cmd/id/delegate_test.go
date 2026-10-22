// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestDelegatedClientSpecPasaLaPuertaDelCanje es el camino crítico del
// aprovisionamiento: el client que escribe este subcomando tiene que superar
// EXACTAMENTE la puerta deny-by-default del canje (domain.AuthorizeImpersonationClient).
// Si alguien le quitara el grant, lo hiciera público o le cambiara el método de
// autenticación, la migración fallaría en producción con un unauthorized_client
// opaco; aquí falla en el test.
func TestDelegatedClientSpecPasaLaPuertaDelCanje(t *testing.T) {
	c := delegatedClientSpec("acme")
	// La columna la fija el UPSERT; el org_id lo pone el propio subcomando tras
	// resolver el slug (y el CHECK de la mig 0025 lo exige).
	c.MayImpersonate = true
	c.OrgID = "11111111-1111-1111-1111-111111111111"

	if err := domain.AuthorizeImpersonationClient(c); err != nil {
		t.Fatalf("el client aprovisionado no puede canjear: %v", err)
	}
	if c.ID != delegatedClientPrefix+"acme" {
		t.Errorf("client_id = %q; debe derivarse del slug para ser idempotente", c.ID)
	}
	if c.IsPublic() {
		t.Error("un client delegable NUNCA puede ser público: solo diría su id, no lo probaría")
	}
	if c.MayImpersonateAdmins {
		t.Error("actuar como owner/admin se concede aparte; este subcomando no lo hace")
	}
}

// TestDelegatedClientSinOrgNoCanjea: sin organización propia no hay "misma
// organización" contra la que acotar el sujeto — la puerta debe cerrarse. Es el
// motivo por el que el client GLOBAL de la pasarela no sirve para esto.
func TestDelegatedClientSinOrgNoCanjea(t *testing.T) {
	c := delegatedClientSpec("acme")
	c.MayImpersonate = true // sin OrgID
	if err := domain.AuthorizeImpersonationClient(c); err == nil {
		t.Fatal("un client delegable SIN organización pudo canjear")
	}
}

// TestDelegatedClientScopesSinEscalada: los scopes declarados son los mínimos de
// identidad. Nada de admin (sería actuar con todos los permisos de la org) ni de
// offline_access (delegación indefinida sin nueva traza).
func TestDelegatedClientScopesSinEscalada(t *testing.T) {
	for _, sc := range delegatedClientSpec("acme").Scopes {
		if sc == "admin" || sc == "offline_access" {
			t.Errorf("scope %q no puede formar parte de un client de migración", sc)
		}
	}
	// Y el recorte de dominio confirma que, aun pidiéndolos, no viajarían.
	c := delegatedClientSpec("acme")
	got := domain.ImpersonationScope("openid admin offline_access", c, domain.RoleMember)
	if got != "openid" {
		t.Fatalf("scope efectivo = %q; se esperaba solo «openid»", got)
	}
}
