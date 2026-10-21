// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del registro dinámico de clients (RFC 7591) contra un PostgreSQL
// real (pgtest), extendiendo el harness de flujos (newLiveServer). Ejerce de punta a
// punta: un admin emite un token de acceso inicial → la app se registra por 7591 →
// corre el Authorization Code + PKCE completo contra el propio servidor con el client
// recién creado y canjea el código por tokens.
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

// memberSession crea un usuario miembro de la org y su cookie de sesión SSO (rol member:
// sin step-up MFA, para que /authorize emita el código directamente).
// memberSession abre una sesión de MIEMBRO con contraseña, que es el caso normal.
//
// `amr` permite pedir otra combinación, y hace falta para quien vaya a ser
// owner/admin: `/authorize` les exige el segundo factor (CONVENTIONS §8) y con
// solo `pwd` desvía a `/account?mfa=required` en vez de devolver el código, así
// que el canje falla en un sitio que no tiene nada que ver con lo que se prueba.
func (l *liveServer) memberSession(t *testing.T, email, orgID string, amr ...string) *http.Cookie {
	t.Helper()
	if len(amr) == 0 {
		amr = []string{"pwd"}
	}
	u := l.mkUser(t, email, "")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: orgID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: u.ID, OrgID: orgID,
		AuthTime: now, AMR: amr, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: raw}
}

func TestDCR_RegisterAndCodeFlow(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	admin := l.mkUser(t, "admin@id.test", "admin-pass-123")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: admin.ID, Role: domain.RoleAdmin}); err != nil {
		t.Fatalf("UpsertMembership admin: %v", err)
	}
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// 1) El admin emite un token de acceso inicial del DCR.
	rr, out := l.do(t, http.MethodPost, "/api/v1/admin/apps/dcr-tokens", adminTok, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("emitir token DCR: código=%d body=%s", rr.Code, rr.Body)
	}
	iat, _ := out["initial_access_token"].(string)
	if iat == "" {
		t.Fatal("no se devolvió initial_access_token")
	}

	// 2) La app se registra por RFC 7591 con ese token (bearer).
	redirectURI := "https://app.example/cb"
	rr, reg := l.do(t, http.MethodPost, "/oauth2/register", iat, map[string]any{
		"client_name":   "App de prueba",
		"redirect_uris": []string{redirectURI},
		"scope":         "openid offline_access",
		"grant_types":   []string{"authorization_code", "refresh_token"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("registro DCR: código=%d body=%s", rr.Code, rr.Body)
	}
	clientID, _ := reg["client_id"].(string)
	clientSecret, _ := reg["client_secret"].(string)
	if clientID == "" || clientSecret == "" {
		t.Fatalf("registro sin client_id/secret: %v", reg)
	}

	// 3) Authorization Code + PKCE contra el propio servidor con el client nuevo.
	member := l.memberSession(t, "grace@acme.example", org.ID)
	verifier, _ := domain.GenerateToken(32)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authz := "/oauth2/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid offline_access"},
		"state":                 {"st-123"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, authz, http.NoBody)
	req.AddCookie(member)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("/authorize código=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("/authorize no devolvió código: %s", loc.String())
	}
	if loc.Query().Get("state") != "st-123" {
		t.Errorf("state no preservado: %q", loc.Query().Get("state"))
	}

	// 4) Canje del código por tokens (Basic auth del client + PKCE verifier).
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	tokReq := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokReq.SetBasicAuth(clientID, clientSecret)
	tokRec := httptest.NewRecorder()
	l.mux.ServeHTTP(tokRec, tokReq)
	if tokRec.Code != http.StatusOK {
		t.Fatalf("/token código=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	if !strings.Contains(tokRec.Body.String(), "access_token") {
		t.Fatalf("/token sin access_token: %s", tokRec.Body.String())
	}
}

func TestDCR_InvalidInitialToken(t *testing.T) {
	l := newLiveServer(t)
	// Sin token → 401.
	rr, _ := l.do(t, http.MethodPost, "/oauth2/register", "", map[string]any{
		"redirect_uris": []string{"https://app.example/cb"},
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("registro sin token debería dar 401, código=%d", rr.Code)
	}
	// Token inventado → 401.
	rr, _ = l.do(t, http.MethodPost, "/oauth2/register", "token-que-no-existe", map[string]any{
		"redirect_uris": []string{"https://app.example/cb"},
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("registro con token inválido debería dar 401, código=%d", rr.Code)
	}
}
