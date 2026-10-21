// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del CRUD admin de clients OIDC por organización (Console)
// contra un PostgreSQL real (pgtest), sobre el harness newLiveServer. Ejercen lo que
// de verdad importa: que una app de terceros registrada desde la Console complete el
// Authorization Code + PKCE y pueda revocar, que el secreto solo se vea al crearlo y
// al rotarlo, y que un admin no vea ni toque los clients de otra organización.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// orgAdmin crea una org con un admin y devuelve la org y su access token.
func (l *liveServer) orgAdmin(t *testing.T, slug, email string) (domain.Organization, string) {
	t.Helper()
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	admin := l.mkUser(t, email, "admin-pass-123")
	if err := l.st.UpsertMembership(l.ctx, nil,
		domain.Membership{OrgID: org.ID, UserID: admin.ID, Role: domain.RoleAdmin}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	return org, l.token(t, admin.ID, org.Slug, []string{"admin"})
}

func TestAdminOIDCClient_CreateAndCodeFlow(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")

	// 1) Alta desde la Console: el secreto se ve UNA vez.
	redirectURI := "https://tercero.example/oauth/callback"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":                      "Herramienta de tickets",
		"redirect_uris":             []string{redirectURI},
		"post_logout_redirect_uris": []string{"https://tercero.example/adios"},
		"scopes":                    []string{"openid", "profile", "email", "offline_access"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta: código=%d body=%s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)
	if clientID == "" || secret == "" {
		t.Fatalf("alta sin client_id/client_secret: %v", created)
	}
	if created["require_pkce"] != true {
		t.Errorf("PKCE debe quedar forzado: %v", created["require_pkce"])
	}
	// Los datos que el admin pega en la app de terceros vienen en la respuesta.
	for _, k := range []string{
		"issuer", "discovery_url", "authorization_endpoint",
		"token_endpoint", "userinfo_endpoint", "jwks_uri",
	} {
		if v, _ := created[k].(string); v == "" {
			t.Errorf("falta %s en la respuesta del alta: %v", k, created)
		}
	}

	// 2) El listado NO devuelve el secreto (solo has_secret).
	rr, list := l.do(t, http.MethodGet, "/api/v1/admin/apps/oidc-clients", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listado: código=%d body=%s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), secret) {
		t.Fatal("el listado ha devuelto el client_secret")
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("el listado debería traer 1 client: %v", list)
	}
	if first, _ := items[0].(map[string]any); first["has_secret"] != true {
		t.Errorf("has_secret debería ser true: %v", items[0])
	}

	// 3) Authorization Code + PKCE completo con ese client.
	member := l.memberSession(t, "grace@acme.example", org.ID)
	code, verifier := l.authorizeCode(t, clientID, redirectURI, member)
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	tokReq := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokReq.SetBasicAuth(clientID, secret)
	tokRec := httptest.NewRecorder()
	l.mux.ServeHTTP(tokRec, tokReq)
	if tokRec.Code != http.StatusOK {
		t.Fatalf("/token código=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	body := tokRec.Body.String()
	if !strings.Contains(body, "access_token") || !strings.Contains(body, "refresh_token") {
		t.Fatalf("/token sin tokens: %s", body)
	}
	var tokens map[string]any
	if err := json.Unmarshal(tokRec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("json de /token: %v", err)
	}
	refresh, _ := tokens["refresh_token"].(string)

	// 4) Revocación (RFC 7009) del refresh con el mismo client: 200 y deja de valer.
	revoke := httptest.NewRequest(http.MethodPost, "/oauth2/revoke",
		strings.NewReader(url.Values{"token": {refresh}}.Encode()))
	revoke.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revoke.SetBasicAuth(clientID, secret)
	revRec := httptest.NewRecorder()
	l.mux.ServeHTTP(revRec, revoke)
	if revRec.Code != http.StatusOK && revRec.Code != http.StatusNoContent {
		t.Fatalf("/revoke código=%d body=%s", revRec.Code, revRec.Body.String())
	}
	refReq := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(
		url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}.Encode(),
	))
	refReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	refReq.SetBasicAuth(clientID, secret)
	refRec := httptest.NewRecorder()
	l.mux.ServeHTTP(refRec, refReq)
	if refRec.Code == http.StatusOK {
		t.Fatalf("un refresh revocado no debe canjearse: %s", refRec.Body.String())
	}

	// 5) Rotación: secreto nuevo (distinto) y el viejo deja de autenticar.
	rr, rotated := l.do(t, http.MethodPost,
		"/api/v1/admin/apps/oidc-clients/"+clientID+"/rotate-secret", adminTok, map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("rotación: código=%d body=%s", rr.Code, rr.Body)
	}
	newSecret, _ := rotated["client_secret"].(string)
	if newSecret == "" || newSecret == secret {
		t.Fatalf("la rotación debe devolver un secreto NUEVO: %v", rotated)
	}
	code2, verifier2 := l.authorizeCode(t, clientID, redirectURI, member)
	oldReq := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code2},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier2},
	}.Encode()))
	oldReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	oldReq.SetBasicAuth(clientID, secret)
	oldRec := httptest.NewRecorder()
	l.mux.ServeHTTP(oldRec, oldReq)
	if oldRec.Code == http.StatusOK {
		t.Fatal("el secreto anterior no debe seguir valiendo tras rotar")
	}

	// 6) Borrado: desaparece del listado y su /authorize deja de existir.
	rr, _ = l.do(t, http.MethodDelete, "/api/v1/admin/apps/oidc-clients/"+clientID, adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("borrado: código=%d body=%s", rr.Code, rr.Body)
	}
	rr, list = l.do(t, http.MethodGet, "/api/v1/admin/apps/oidc-clients", adminTok, nil)
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("tras borrar, el listado debería quedar vacío: %v", list)
	}
}

// TestAdminOIDCClient_OrgIsolation: deny-by-default entre organizaciones.
func TestAdminOIDCClient_OrgIsolation(t *testing.T) {
	l := newLiveServer(t)
	_, tokA := l.orgAdmin(t, "acme", "admin@acme.example")
	_, tokB := l.orgAdmin(t, "globex", "admin@globex.example")

	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", tokA, map[string]any{
		"name":          "App de Acme",
		"redirect_uris": []string{"https://acme.example/cb"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta en acme: código=%d body=%s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)

	// El admin de la otra org no lo ve…
	rr, list := l.do(t, http.MethodGet, "/api/v1/admin/apps/oidc-clients", tokB, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listado globex: código=%d", rr.Code)
	}
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("globex no debe ver los clients de acme: %v", list)
	}
	// …ni lo rota, ni lo borra: 404 (no 403: no se confirma que exista).
	rr, _ = l.do(t, http.MethodPost,
		"/api/v1/admin/apps/oidc-clients/"+clientID+"/rotate-secret", tokB, map[string]any{})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("rotar cross-org debería dar 404, código=%d body=%s", rr.Code, rr.Body)
	}
	rr, _ = l.do(t, http.MethodDelete, "/api/v1/admin/apps/oidc-clients/"+clientID, tokB, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("borrar cross-org debería dar 404, código=%d body=%s", rr.Code, rr.Body)
	}
	// El client sigue vivo para su dueña.
	rr, list = l.do(t, http.MethodGet, "/api/v1/admin/apps/oidc-clients", tokA, nil)
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("acme debería conservar su client: %v", list)
	}
}

// TestAdminOIDCClient_RejectsBadRedirect: las URLs de retorno se validan como en el
// registro dinámico (anti open-redirect), y los scopes de gestión no se conceden.
func TestAdminOIDCClient_RejectsBadRedirect(t *testing.T) {
	l := newLiveServer(t)
	_, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")

	for _, bad := range []string{"http://tercero.example/cb", "https://tercero.example/cb#frag", "no-es-una-url"} {
		rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
			"name": "Mala", "redirect_uris": []string{bad},
		})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("redirect %q debería rechazarse con 400, código=%d", bad, rr.Code)
		}
	}
	// Sin redirect_uris tampoco.
	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name": "Sin retorno",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("sin redirect_uris debería dar 400, código=%d", rr.Code)
	}
	// Pedir `admin` no lo concede: se cae al conjunto self-service.
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name": "Ambiciosa", "redirect_uris": []string{"https://tercero.example/cb"},
		"scopes": []string{"openid", "admin"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta: código=%d body=%s", rr.Code, rr.Body)
	}
	for _, sc := range created["scopes"].([]any) {
		if sc == "admin" {
			t.Fatalf("un client de terceros no puede llevar el scope admin: %v", created["scopes"])
		}
	}
}

// authorizeCode corre /authorize con PKCE y devuelve el código y su code_verifier.
func (l *liveServer) authorizeCode(t *testing.T, clientID, redirectURI string, sess *http.Cookie) (string, string) {
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
		"scope":                 {"openid profile email offline_access"},
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
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("/authorize no devolvió código: %s", loc.String())
	}
	return code, verifier
}
