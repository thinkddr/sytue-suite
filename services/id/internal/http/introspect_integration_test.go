// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la introspección de tokens (RFC 7662) y de la edición
// de perfil self-service contra un PostgreSQL real (pgtest, build-tag
// `integration`). Reusan el harness newLiveServer de security_integration_test.go.
package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// setClientSecret fija el secreto de un client sembrado y devuelve el header
// Authorization Basic correspondiente.
func (l *liveServer) setClientSecret(t *testing.T, clientID, secret string) string {
	t.Helper()
	hash, err := domain.HashPassword(secret)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := l.st.SetClientSecretHash(l.ctx, nil, clientID, hash); err != nil {
		t.Fatalf("SetClientSecretHash: %v", err)
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"+secret))
}

// introspect llama al endpoint con auth Basic de client y devuelve la respuesta.
func (l *liveServer) introspect(t *testing.T, basic, token string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	form := url.Values{"token": {token}}
	r := httptest.NewRequest(http.MethodPost, "/oauth2/introspect", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic != "" {
		r.Header.Set("Authorization", basic)
	}
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	out := map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

func TestIntrospectAccessToken(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "ada@id.test", "")
	basic := l.setClientSecret(t, "sytue-console", "console-secret-1")

	at, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: "sytue-console", Scope: "openid profile",
		Tenant: "acme", Roles: []string{"admin"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	// Válido → active + claims estándar mínimos.
	rr, out := l.introspect(t, basic, at)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if active, _ := out["active"].(bool); !active {
		t.Fatalf("se esperaba active:true, cuerpo=%s", rr.Body)
	}
	if out["sub"] != u.ID {
		t.Fatalf("sub=%v, quiero %s", out["sub"], u.ID)
	}
	if out["client_id"] != "sytue-console" || out["token_type"] != "Bearer" {
		t.Fatalf("client_id/token_type: %v", out)
	}
	if out["scope"] != "openid profile" || out["tenant"] != "acme" {
		t.Fatalf("scope/tenant: %v", out)
	}
	if _, ok := out["exp"]; !ok {
		t.Fatalf("falta exp: %v", out)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("falta no-store: %q", rr.Header().Get("Cache-Control"))
	}

	// Manipulado (firma alterada) → active:false, nada más.
	tampered := at[:len(at)-3] + "AAA"
	_, out = l.introspect(t, basic, tampered)
	if active, _ := out["active"].(bool); active {
		t.Fatalf("token manipulado no debe estar active: %v", out)
	}
	if len(out) != 1 {
		t.Fatalf("inactive debe ser solo {active:false}: %v", out)
	}

	// Expirado (emitido en el pasado, exp < now-skew) → active:false.
	old, err := l.iss.IssueAccessToken(oidc.TokenClaims{Subject: u.ID, Audience: "sytue-console"},
		time.Now().UTC().Add(-20*time.Minute))
	if err != nil {
		t.Fatalf("IssueAccessToken(old): %v", err)
	}
	if _, out = l.introspect(t, basic, old); out["active"] == true {
		t.Fatalf("token expirado no debe estar active: %v", out)
	}

	// Sin auth de client → 401 (no escaneo anónimo).
	if rr, _ = l.introspect(t, "", at); rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth de client: status=%d, quiero 401", rr.Code)
	}
}

// TestIntrospectRechazaClientPublico: un client público «autentica» presentando
// solo su client_id, que viaja dentro del binario de la app o del bundle de la
// SPA. Si eso bastara, la introspección sería anónima con un paso extra: quien
// copie ese identificador podría preguntar por cualquier token. RFC 7662 §2.1
// pide proteger el endpoint, y la discovery ya no anuncia `none` para él.
func TestIntrospectRechazaClientPublico(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "publico@id.test", "")
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "pubcli", Name: "Pub Cli"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	pub := domain.Client{
		ID: "app-movil-publica", Name: "App móvil", Type: domain.ClientPublic,
		RedirectURIs: []string{"sytue://cb"}, GrantTypes: []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"}, Scopes: []string{"openid"},
		RequirePKCE: true, TokenEndpointAuthMethod: "none",
	}
	// `admin`, como el resto de los tests de este paquete: `managed_by` tiene un
	// CHECK que solo admite 'static', 'dcr' o 'admin' (mig 0017), así que un
	// "test" de relleno no llega ni a insertarse.
	if err := l.st.CreateOAuthClient(l.ctx, nil, pub, org.ID, "admin"); err != nil {
		t.Fatalf("CreateOAuthClient: %v", err)
	}
	at, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: pub.ID, Scope: "openid",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	// client_id en el formulario, sin secreto: es exactamente el método `none`.
	form := url.Values{"token": {at}, "client_id": {pub.ID}}
	r := httptest.NewRequest(http.MethodPost, "/oauth2/introspect", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("client público: status=%d, quiero 401; cuerpo=%s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "active") {
		t.Fatalf("no debe revelar estado del token: %s", rr.Body)
	}
}

func TestIntrospectRefreshToken(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "grace@id.test", "")
	basic := l.setClientSecret(t, "sytue-console", "console-secret-1")

	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	id, err := l.st.InsertRefreshToken(l.ctx, nil, domain.RefreshToken{
		TokenHash: domain.HashToken(raw), ClientID: "sytue-console", UserID: u.ID,
		Scope: "openid offline_access", AuthTime: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("InsertRefreshToken: %v", err)
	}

	// Válido y del client dueño → active.
	_, out := l.introspect(t, basic, raw)
	if active, _ := out["active"].(bool); !active {
		t.Fatalf("refresh válido debe estar active: %v", out)
	}
	if out["sub"] != u.ID || out["client_id"] != "sytue-console" {
		t.Fatalf("sub/client_id: %v", out)
	}

	// Otro client (no dueño) → active:false, sin revelar que existe.
	otherBasic := l.setClientSecret(t, "sytue-cloud", "cloud-secret-1")
	if _, out = l.introspect(t, otherBasic, raw); out["active"] == true {
		t.Fatalf("un client ajeno no debe ver el refresh: %v", out)
	}

	// Revocado → active:false.
	if err := l.st.RevokeRefreshChain(l.ctx, nil, id); err != nil {
		t.Fatalf("RevokeRefreshChain: %v", err)
	}
	if _, out = l.introspect(t, basic, raw); out["active"] == true {
		t.Fatalf("refresh revocado no debe estar active: %v", out)
	}
}

func TestMeProfileUpdate(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "self@id.test", "")
	other := l.mkUser(t, "other@id.test", "")
	tok := l.token(t, u.ID, "", nil)

	// Actualiza el propio nombre → 200, persistido; email intacto.
	rr, out := l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{"name": "Ada Lovelace"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["name"] != "Ada Lovelace" {
		t.Fatalf("name devuelto=%v", out["name"])
	}
	got, _ := l.st.GetUserByID(l.ctx, u.ID)
	if got.Name != "Ada Lovelace" {
		t.Fatalf("name no persistido: %q", got.Name)
	}
	if got.Email != "self@id.test" {
		t.Fatalf("el email no debe cambiar: %q", got.Email)
	}

	// Nombre en blanco → 400.
	if rr, _ = l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{"name": "   "}); rr.Code != http.StatusBadRequest {
		t.Fatalf("nombre en blanco: status=%d, quiero 400", rr.Code)
	}

	// Solo afecta al sujeto del token: el otro usuario queda intacto (no hay forma
	// de nombrar a otro; el sub gobierna).
	if o, _ := l.st.GetUserByID(l.ctx, other.ID); o.Name != "Test" {
		t.Fatalf("no debe alterar a otro usuario: %q", o.Name)
	}

	// --- Los otros seis campos, y el PARCHEO -------------------------------
	//
	// `SetProfile` escribe las siete columnas de una vez, así que un PATCH que
	// solo trae el nombre TIENE que conservar el resto. Sin esto, guardar el
	// nombre desde la pantalla de cuenta le borraba a cada persona su puesto, su
	// departamento y su ubicación, y nada lo diría: la respuesta es 200.
	rr, out = l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{
		"given_name": "Ada", "family_name": "Lovelace",
		"title": "Analista", "department": "Cálculo", "location": "Londres",
		"locale": "es",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("ficha completa: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["title"] != "Analista" || out["locale"] != "es" {
		t.Fatalf("la respuesta no trae la ficha: %v", out)
	}
	got, _ = l.st.GetUserByID(l.ctx, u.ID)
	if got.Title != "Analista" || got.Department != "Cálculo" || got.Location != "Londres" ||
		got.GivenName != "Ada" || got.FamilyName != "Lovelace" || got.Locale != "es" {
		t.Fatalf("ficha no persistida: %+v", got)
	}

	// Un PATCH con SOLO el nombre no puede llevarse por delante lo demás.
	if rr, _ = l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{"name": "Ada L."}); rr.Code != http.StatusOK {
		t.Fatalf("parche de solo nombre: status=%d", rr.Code)
	}
	got, _ = l.st.GetUserByID(l.ctx, u.ID)
	if got.Name != "Ada L." {
		t.Fatalf("el nombre no se guardó: %q", got.Name)
	}
	if got.Title != "Analista" || got.Department != "Cálculo" || got.Locale != "es" {
		t.Fatalf("el parche borró campos que no venían: %+v", got)
	}

	// Y la cadena vacía SÍ vacía: es la diferencia entre «no viene» y «bórralo»,
	// y sin ella no habría forma de quitarse un puesto que ya no se ocupa.
	if rr, _ = l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{"title": ""}); rr.Code != http.StatusOK {
		t.Fatalf("vaciar el puesto: status=%d", rr.Code)
	}
	got, _ = l.st.GetUserByID(l.ctx, u.ID)
	if got.Title != "" {
		t.Fatalf("el puesto debía quedar vacío: %q", got.Title)
	}
	if got.Department != "Cálculo" {
		t.Fatalf("vaciar uno no vacía los demás: %+v", got)
	}

	// Un idioma que no es de producto cae al inglés, no se guarda tal cual.
	if rr, _ = l.do(t, http.MethodPatch, "/api/v1/me", tok, map[string]string{"locale": "xx"}); rr.Code != http.StatusOK {
		t.Fatalf("idioma desconocido: status=%d", rr.Code)
	}
	got, _ = l.st.GetUserByID(l.ctx, u.ID)
	if got.Locale != "en" {
		t.Fatalf("idioma desconocido debía normalizar a en: %q", got.Locale)
	}
}
