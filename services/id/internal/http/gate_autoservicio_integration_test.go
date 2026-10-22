// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El gate «solo clients de la suite» cubre también la gestión de las credenciales
// de la propia persona y la administración que no lleva `/admin/` en la ruta.
//
// Lo que cierra: la app que un administrador registra en su organización, con
// `openid profile email`, recibía al entrar alguien en ella un token con el que
// podía dar de alta una passkey o una contraseña de aplicación en la cuenta de esa
// persona, listar sus sesiones, editar su perfil o —si quien entraba administraba
// la organización— añadirle miembros por `/api/v1/orgs/{slug}/members`. Quien
// entra en una app consiente que la app sepa quién es, no que gestione cómo se
// entra en su cuenta.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestAppDeTercerosNoGestionaLasCredencialesDeLaPersona(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")

	const redirectURI = "https://tercero.example/oauth/callback"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "Herramienta de tickets",
		"redirect_uris": []string{redirectURI},
		"scopes":        []string{"openid", "profile", "email", "offline_access"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: código=%d body=%s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)

	// El owner de la organización entra en esa app (con segundo factor, que
	// /authorize exige a los owner).
	sess := l.memberSession(t, "owner@acme.example", org.ID, "pwd", "mfa")
	owner, err := l.st.GetUserByEmail(l.ctx, "owner@acme.example")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
		OrgID: org.ID, UserID: owner.ID, Role: domain.RoleOwner,
	}); err != nil {
		t.Fatalf("UpsertMembership owner: %v", err)
	}
	appTok := l.tokenDeApp(t, clientID, secret, redirectURI, sess)

	// Con el token de la app NO se gestiona cómo se entra en la cuenta…
	for _, c := range []struct {
		metodo, ruta string
		cuerpo       any
	}{
		{http.MethodPost, "/api/v1/webauthn/register/begin", map[string]any{}},
		{http.MethodGet, "/api/v1/webauthn/credentials", nil},
		{http.MethodPost, "/api/v1/me/app-passwords", map[string]any{"label": "correo"}},
		{http.MethodGet, "/api/v1/me/app-passwords", nil},
		{http.MethodPost, "/api/v1/me/mfa/totp/enroll", map[string]any{}},
		{http.MethodGet, "/api/v1/me/sessions", nil},
		{http.MethodDelete, "/api/v1/me/sessions", nil},
		{http.MethodGet, "/api/v1/me/devices", nil},
		{http.MethodGet, "/api/v1/me/export", nil},
		{http.MethodGet, "/api/v1/me/pim", nil},
		// …ni se edita el perfil…
		{http.MethodPatch, "/api/v1/me", map[string]any{"name": "Otro nombre"}},
		// …ni se administra la organización por la ruta que no lleva /admin/.
		{http.MethodPost, "/api/v1/orgs/acme/members", map[string]any{"email": "nadie@acme.example", "role": "member"}},
		{http.MethodGet, "/api/v1/onboarding/", nil},
	} {
		if rr, _ := l.do(t, c.metodo, c.ruta, appTok, c.cuerpo); rr.Code != http.StatusForbidden {
			t.Errorf("%s %s con token de app de terceros: código=%d, quiero 403", c.metodo, c.ruta, rr.Code)
		}
	}
	// Y no ha quedado nada creado en la cuenta.
	if claves, _ := l.st.ListAppPasswords(l.ctx, owner.ID); len(claves) != 0 {
		t.Errorf("la app creó %d contraseña(s) de aplicación en la cuenta", len(claves))
	}

	// Lo que la app SÍ pidió sigue funcionando: leer el perfil de quien consintió…
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/me", appTok, nil); rr.Code != http.StatusOK {
		t.Fatalf("GET /me con token de app de terceros: código=%d, quiero 200", rr.Code)
	}
	// …y las consultas de directorio que los productos reenvían con el token de
	// quien llama (aquí no se mira el resultado, solo que el gate no las corta).
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/directory:resolve", appTok, map[string]any{"email": "owner@acme.example"}); rr.Code == http.StatusForbidden {
		t.Errorf("directory:resolve con token de app de terceros: 403, y debe seguir abierto")
	}

	// La MISMA persona, con una credencial pedida por un client de la suite,
	// gestiona su cuenta igual que antes.
	suite := l.token(t, owner.ID, org.Slug, []string{"owner"})
	for _, c := range []struct{ metodo, ruta string }{
		{http.MethodGet, "/api/v1/me/app-passwords"},
		{http.MethodGet, "/api/v1/me/sessions"},
		{http.MethodGet, "/api/v1/webauthn/credentials"},
		{http.MethodPost, "/api/v1/webauthn/register/begin"},
	} {
		if rr, _ := l.do(t, c.metodo, c.ruta, suite, nil); rr.Code != http.StatusOK {
			t.Errorf("%s %s con credencial de la suite: código=%d, quiero 200", c.metodo, c.ruta, rr.Code)
		}
	}
}
