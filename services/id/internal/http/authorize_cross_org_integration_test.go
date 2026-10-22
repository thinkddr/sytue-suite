// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// NEG-05: una app OIDC acotada a una organización (las de terceros, mig 0017) NO
// puede obtener un código —ni, por tanto, tokens— para una sesión de OTRA
// organización.
//
// El agujero: `handleAuthorize` validaba la app, la URL de retorno, PKCE y los
// scopes, y con una sesión SSO viva emitía el código SIN comparar la organización
// de la app (`client.OrgID`) con la de la sesión (`sess.OrgID`). El código se
// llevaba el tenant de la sesión, así que una app registrada por la organización A
// recibía un token con alcance sobre el Drive, la agenda y los contactos de una
// persona de la organización B con solo que su navegador, con sesión abierta,
// cargara la dirección de autorización. La Directory API ya exigía que la app
// fuera de la organización del token (`google_compat.go`); esto lleva esa misma
// atadura al punto donde nace el código.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// authorizeResult lanza /oauth2/authorize con la sesión dada y devuelve el código
// (si lo hubo) y el `error` de OAuth de la redirección (si lo hubo).
func (l *liveServer) authorizeResult(t *testing.T, clientID, redirectURI, scope string, sess *http.Cookie) (code, oauthErr string) {
	t.Helper()
	verifier, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	authz := "/oauth2/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {scope},
		"state":                 {"st-1"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, authz, http.NoBody)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("/authorize código=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	return loc.Query().Get("code"), loc.Query().Get("error")
}

func TestAppDeOrgNoSirveASesionDeOtraOrg(t *testing.T) {
	l := newLiveServer(t)

	// Organización A registra una app (client acotado a A).
	orgA, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")
	const redirectURI = "https://crm.example/oauth/google/callback"
	const g = "https://www.googleapis.com/auth/"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "CRM de A",
		"redirect_uris": []string{redirectURI},
		"scopes":        []string{"openid", "email", "offline_access", g + "calendar.readonly"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: %d %s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)
	if clientID == "" {
		t.Fatalf("alta sin client_id: %v", created)
	}

	scope := "openid email " + g + "calendar.readonly"

	// 1) Sesión de la organización B + app de A ⇒ se DENIEGA (NEG-05).
	orgB, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("CreateOrg B: %v", err)
	}
	sessB := l.memberSession(t, "victima@beta.example", orgB.ID)
	code, oauthErr := l.authorizeResult(t, clientID, redirectURI, scope, sessB)
	if code != "" {
		t.Fatalf("NEG-05: /authorize emitió un código para una sesión de otra organización (code=%q)", code)
	}
	if oauthErr != "access_denied" {
		t.Fatalf("se esperaba error=access_denied, se obtuvo %q", oauthErr)
	}

	// 2) Control positivo: la misma app con una sesión de SU organización sí
	// obtiene código (el arreglo no rompe el caso legítimo).
	sessA := l.memberSession(t, "ana@acme.example", orgA.ID)
	code, oauthErr = l.authorizeResult(t, clientID, redirectURI, scope, sessA)
	if code == "" {
		t.Fatalf("la app de A con una sesión de A debería emitir código (error=%q)", oauthErr)
	}

	// 3) Defensa en profundidad en el canje: un código cuya organización no es la
	// de la app no se canjea aunque llegue directo a /token (código previo al
	// despliegue del arreglo, u otra vía). Se inserta a mano un código de B para la
	// app de A y se intenta canjear.
	victima, _ := l.st.GetUserByEmail(l.ctx, "victima@beta.example")
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := l.st.InsertAuthCode(l.ctx, nil, domain.AuthCode{
		CodeHash:    domain.HashToken(raw),
		ClientID:    clientID,
		UserID:      victima.ID,
		OrgID:       orgB.ID,
		RedirectURI: redirectURI,
		Scope:       "openid email",
		ExpiresAt:   time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("InsertAuthCode: %v", err)
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {raw}, "redirect_uri": {redirectURI}}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("canje de un código de otra organización: código=%d body=%s", rec.Code, rec.Body.String())
	}
}

// refrescar canjea un refresh token con la auth del client y devuelve el código HTTP.
func (l *liveServer) refrescar(t *testing.T, clientID, secret, refresh string) int {
	t.Helper()
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	return rec.Code
}

// La misma atadura al REFRESCAR: un refresh token de una app de A para una sesión
// de B —el que pudo emitirse antes de cerrar NEG-05, con treinta días de vida y
// rotación— deja de valer. El de una persona de la propia organización sigue.
func TestRefreshDeAppDeOrgNoSirveAOtraOrg(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")
	const redirectURI = "https://crm.example/oauth/callback"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "CRM de A",
		"redirect_uris": []string{redirectURI},
		"scopes":        []string{"openid", "email", "offline_access"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: %d %s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)

	orgB, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("CreateOrg B: %v", err)
	}
	l.memberSession(t, "victima@beta.example", orgB.ID)
	l.memberSession(t, "ana@acme.example", orgA.ID)
	victima, _ := l.st.GetUserByEmail(l.ctx, "victima@beta.example")
	ana, _ := l.st.GetUserByEmail(l.ctx, "ana@acme.example")

	emitir := func(userID, orgID string) string {
		raw, err := domain.GenerateToken(32)
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		now := time.Now()
		if _, err := l.st.InsertRefreshToken(l.ctx, nil, domain.RefreshToken{
			TokenHash: domain.HashToken(raw), ClientID: clientID, UserID: userID, OrgID: orgID,
			Scope: "openid email offline_access", AuthTime: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("InsertRefreshToken: %v", err)
		}
		return raw
	}

	// Refresh de la app de A para una persona de B: se rechaza.
	if code := l.refrescar(t, clientID, secret, emitir(victima.ID, orgB.ID)); code != http.StatusBadRequest {
		t.Fatalf("NEG-05: el refresh de una app de A para una sesión de B devolvió %d, quiero 400", code)
	}
	// Control: el de una persona de la propia organización sigue funcionando.
	if code := l.refrescar(t, clientID, secret, emitir(ana.ID, orgA.ID)); code != http.StatusOK {
		t.Fatalf("el refresh de la app de A para una persona de A devolvió %d, quiero 200", code)
	}
}
