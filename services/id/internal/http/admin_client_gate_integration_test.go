// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La administración de Sytue ID solo se abre con una credencial pedida por un
// client de la SUITE, no por una app de terceros.
//
// El camino que cierra: un access token de este IdP vale para esta API venga del
// client que venga (el `aud` lleva siempre el resource server) y los claims
// `roles`/`perms` viajan en todos los tokens. Así que la app que un administrador
// registra desde la consola —con `openid profile email` y nada más— recibía, en
// cuanto ese mismo administrador entraba en ella, una credencial que abría el
// directorio, el reseteo de contraseñas y de MFA, los roles y la auditoría de su
// organización. El test recorre ese flujo entero con un client real y un canje
// real, no con un token fabricado a mano.
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// tokenDeApp completa Authorization Code + PKCE contra `clientID` con la sesión
// dada y devuelve el access token que recibiría la app de terceros.
func (l *liveServer) tokenDeApp(t *testing.T, clientID, secret, redirectURI string, sess *http.Cookie) string {
	t.Helper()
	code, verifier := l.authorizeCode(t, clientID, redirectURI, sess)
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/token código=%d body=%s", rec.Code, rec.Body.String())
	}
	var tokens map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("json de /token: %v", err)
	}
	at, _ := tokens["access_token"].(string)
	if at == "" {
		t.Fatalf("/token sin access_token: %s", rec.Body.String())
	}
	return at
}

func TestAppDeTercerosNoAdministraLaOrganizacion(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")

	// Un administrador registra una app de terceros con los scopes de siempre.
	const redirectURI = "https://tercero.example/oauth/callback"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "Herramienta de tickets",
		"redirect_uris": []string{redirectURI},
		// El conjunto self-service de siempre: ni uno solo de administración
		// (dcr.go ya los filtra). offline_access va porque es lo que pide el
		// helper del canje; el punto del test es que ninguno de estos autoriza
		// nada en la administración.
		"scopes": []string{"openid", "profile", "email", "offline_access"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: código=%d body=%s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)

	// El OWNER de la organización inicia sesión en esa app.
	// Con segundo factor: va a ser OWNER, y a esos `/authorize` les exige step-up
	// (CONVENTIONS §8). Sin él no llega a haber código que canjear y el test se
	// caería antes de poder comprobar lo suyo, que es el alcance del token.
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

	// Con esa credencial NO se administra la identidad: ni el directorio…
	for _, ruta := range []string{
		"/api/v1/admin/users",
		"/api/v1/admin/audit",
		"/api/v1/admin/roles",
		"/api/v1/platform/activation",
	} {
		if rr, _ := l.do(t, http.MethodGet, ruta, appTok, nil); rr.Code != http.StatusForbidden {
			t.Errorf("GET %s con token de app de terceros: código=%d, quiero 403", ruta, rr.Code)
		}
	}
	// …ni el reseteo de contraseña de nadie.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/"+owner.ID+"/reset-password", appTok, map[string]any{})
	if rr.Code != http.StatusForbidden {
		t.Errorf("reset de contraseña con token de app de terceros: código=%d, quiero 403", rr.Code)
	}

	// Lo que la app SÍ pidió sigue funcionando: el perfil de quien consintió.
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/me", appTok, nil); rr.Code != http.StatusOK {
		t.Fatalf("/me con token de app de terceros: código=%d, quiero 200", rr.Code)
	}

	// Y el MISMO usuario, con la credencial de la consola, administra igual que
	// antes: el gate mira quién pidió el token, no quién lo usa.
	consola := l.token(t, owner.ID, org.Slug, []string{"owner"})
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/users", consola, nil); rr.Code != http.StatusOK {
		t.Fatalf("/admin/users con la credencial de la consola: código=%d, quiero 200", rr.Code)
	}
}
