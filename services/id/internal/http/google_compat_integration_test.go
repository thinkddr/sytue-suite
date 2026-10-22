// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Una app escrita para Google, registrada por el administrador con scopes de
// Google, recorre el flujo de Google contra Sytue ID: autoriza con los parámetros de
// Google, canjea, lee userinfo y tokeninfo con la forma de Google, y lee el
// directorio de SU organización solo si quien inicia sesión la administra.
func TestCuentaDeGoogleEmulada(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")
	if _, err := l.st.AddOrgMailDomain(l.ctx, nil, org.ID, "acme.example", "tok"); err != nil {
		t.Fatalf("AddOrgMailDomain: %v", err)
	}
	if err := l.st.MarkOrgMailDomainVerified(l.ctx, nil, org.ID, "acme.example"); err != nil {
		t.Fatalf("MarkOrgMailDomainVerified: %v", err)
	}

	const redirectURI = "https://crm.example/oauth/google/callback"
	const g = "https://www.googleapis.com/auth/"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "CRM escrito para Google",
		"redirect_uris": []string{redirectURI},
		"scopes": []string{
			"openid", "email", "offline_access", g + "calendar.readonly",
			g + "admin.directory.user.readonly", g + "admin.directory.group.readonly", "admin",
		},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: %d %s", rr.Code, rr.Body)
	}
	clientID, _ := created["client_id"].(string)
	secret, _ := created["client_secret"].(string)
	concedidos, _ := created["scopes"].([]any)
	if !slices.Contains(concedidos, any(g+"calendar.readonly")) || slices.Contains(concedidos, any("admin")) {
		t.Fatalf("scopes de la app = %v: los de Google sí, los de administración no", concedidos)
	}

	sess := l.memberSession(t, "owner@acme.example", org.ID, "pwd", "mfa")
	owner, _ := l.st.GetUserByEmail(l.ctx, "owner@acme.example")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: owner.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatal(err)
	}
	grupo, err := l.st.CreateGroup(l.ctx, nil, org.ID, "Ventas", "Equipo comercial")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.AddGroupMember(l.ctx, nil, org.ID, grupo.ID, owner.ID); err != nil {
		t.Fatal(err)
	}

	scope := strings.Join([]string{
		"openid", g + "userinfo.email", g + "calendar.readonly",
		g + "admin.directory.user.readonly", g + "admin.directory.group.readonly",
	}, " ")
	tokens := l.flujoGoogle(t, clientID, secret, redirectURI, scope, sess)
	at, _ := tokens["access_token"].(string)
	if tokens["refresh_token"] == nil {
		t.Error("access_type=offline tiene que dar refresh token")
	}
	crudo, _ := jwt.ParseString(at, jwt.WithVerify(false))
	if !slices.Contains(crudo.Audience(), domain.GoogleAPIsAudience) || crudo.Audience()[1] != clientID {
		t.Fatalf("aud del access token = %v", crudo.Audience())
	}
	idt, _ := jwt.ParseString(tokens["id_token"].(string), jwt.WithVerify(false))
	if hd, _ := idt.Get("hd"); hd != "acme.example" {
		t.Errorf("hd del ID token = %v", hd)
	}

	rr, info := l.do(t, http.MethodGet, "/compat/google/oauth2/v3/userinfo", at, nil)
	if rr.Code != http.StatusOK || info["email"] != "owner@acme.example" || info["hd"] != "acme.example" {
		t.Fatalf("userinfo v3: %d %v", rr.Code, info)
	}
	rr, v2 := l.do(t, http.MethodGet, "/compat/google/oauth2/v2/userinfo", at, nil)
	if rr.Code != http.StatusOK || v2["id"] != owner.ID || v2["verified_email"] == nil {
		t.Fatalf("userinfo v2: %d %v", rr.Code, v2)
	}
	rr, ti := l.do(t, http.MethodGet, "/compat/google/oauth2/v3/tokeninfo?access_token="+url.QueryEscape(at), "", nil)
	if rr.Code != http.StatusOK || ti["aud"] != clientID || !strings.Contains(ti["scope"].(string), g+"calendar.readonly") ||
		ti["access_type"] != "offline" {
		t.Fatalf("tokeninfo: %d %v", rr.Code, ti)
	}
	if rr, _ := l.do(t, http.MethodGet, "/compat/google/oauth2/v3/tokeninfo?access_token=basura", "", nil); rr.Code != http.StatusBadRequest {
		t.Errorf("tokeninfo de un token falso: %d", rr.Code)
	}

	rr, lista := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users?customer=my_customer&maxResults=1", at, nil)
	if rr.Code != http.StatusOK || lista["kind"] != "admin#directory#users" || lista["nextPageToken"] == nil {
		t.Fatalf("users (página 1): %d %v", rr.Code, lista)
	}
	vistos := map[string]bool{}
	for pagina := lista; ; {
		us, _ := pagina["users"].([]any)
		for _, u := range us {
			vistos[u.(map[string]any)["id"].(string)] = true
		}
		siguiente, _ := pagina["nextPageToken"].(string)
		if siguiente == "" {
			break
		}
		_, pagina = l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users?maxResults=1&pageToken="+siguiente, at, nil)
	}
	if !vistos[owner.ID] || len(vistos) < 2 {
		t.Fatalf("paginando se ven los miembros: %v", vistos)
	}
	rr, u := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users/owner@acme.example", at, nil)
	if rr.Code != http.StatusOK || u["id"] != owner.ID || u["isAdmin"] != true {
		t.Fatalf("users/{email}: %d %v", rr.Code, u)
	}
	if rr, _ := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users/nadie@otra.example", at, nil); rr.Code != http.StatusNotFound {
		t.Errorf("users de alguien de fuera: %d", rr.Code)
	}
	rr, gs := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/groups?userKey=owner@acme.example", at, nil)
	if lg, _ := gs["groups"].([]any); rr.Code != http.StatusOK || len(lg) != 1 || lg[0].(map[string]any)["directMembersCount"] != "1" {
		t.Fatalf("groups: %d %v", rr.Code, gs)
	}
	rr, ms := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/groups/"+grupo.ID+"/members", at, nil)
	if lm, _ := ms["members"].([]any); rr.Code != http.StatusOK || len(lm) != 1 || lm[0].(map[string]any)["email"] != "owner@acme.example" {
		t.Fatalf("members: %d %v", rr.Code, ms)
	}

	// Un miembro normal, con la MISMA app y los mismos scopes, no ve el directorio.
	sessMiembro := l.memberSession(t, "ana@acme.example", org.ID)
	delMiembro := l.flujoGoogle(t, clientID, secret, redirectURI, scope, sessMiembro)
	if rr, _ := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users", delMiembro["access_token"].(string), nil); rr.Code != http.StatusForbidden {
		t.Errorf("directorio con token de un miembro normal: %d", rr.Code)
	}
	// Y el owner, sin el scope de directorio, tampoco.
	sinDirectorio := l.flujoGoogle(t, clientID, secret, redirectURI, "openid "+g+"calendar.readonly", sess)
	if rr, _ := l.do(t, http.MethodGet, "/compat/google/admin/directory/v1/users", sinDirectorio["access_token"].(string), nil); rr.Code != http.StatusForbidden {
		t.Errorf("directorio sin su scope: %d", rr.Code)
	}
	// La audiencia de Google solo llega con scopes de API: el login a secas no la lleva.
	soloLogin := l.flujoGoogle(t, clientID, secret, redirectURI, "openid email", sess)
	plano, _ := jwt.ParseString(soloLogin["access_token"].(string), jwt.WithVerify(false))
	if slices.Contains(plano.Audience(), domain.GoogleAPIsAudience) {
		t.Errorf("un token solo de login no abre las APIs: aud=%v", plano.Audience())
	}

	rr, disc := l.do(t, http.MethodGet, "/compat/google/.well-known/openid-configuration", "", nil)
	if rr.Code != http.StatusOK || !strings.HasSuffix(disc["authorization_endpoint"].(string), "/compat/google/o/oauth2/v2/auth") {
		t.Fatalf("discovery: %d %v", rr.Code, disc)
	}
}

// flujoGoogle recorre el flujo tal como lo lanza una librería de Google: el auth de
// Google con access_type=offline, su redirección al authorize y el canje.
func (l *liveServer) flujoGoogle(t *testing.T, clientID, secret, redirectURI, scope string, sess *http.Cookie) map[string]any {
	t.Helper()
	verifier, _ := domain.GenerateToken(32)
	sum := sha256.Sum256([]byte(verifier))
	auth := "/compat/google/o/oauth2/v2/auth?" + url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "scope": {scope},
		"access_type": {"offline"}, "include_granted_scopes": {"true"}, "prompt": {"consent"}, "state": {"s"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}.Encode()
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, auth, http.NoBody))
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/oauth2/authorize?") {
		t.Fatalf("auth de Google: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	req := httptest.NewRequest(http.MethodGet, rec.Header().Get("Location"), http.NoBody)
	req.AddCookie(sess)
	rec = httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")
	if rec.Code != http.StatusFound || code == "" {
		t.Fatalf("authorize: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {verifier}}
	req = httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	rec = httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("token: %d %s", rec.Code, rec.Body.String())
	}
	return out
}
