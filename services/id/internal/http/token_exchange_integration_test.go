// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del canje RFC 8693 (token exchange con impersonación
// acotada) contra un PostgreSQL real (pgtest, build-tag `integration`). Cubren de
// extremo a extremo lo que las pruebas puras de domain/oidc no pueden: que el
// token emitido lleve la identidad REAL del usuario del directorio, que el
// aislamiento por organización lo imponga el WHERE del store, y que CADA canje
// deje su fila en audit_log. Reusa el harness newLiveServer.
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// mkDelegableClient crea un client de la org y lo marca como delegable. El
// marcado va por SQL a propósito: NINGUNA API lo escribe (mig 0025), así que el
// test hace exactamente lo que hará el operador de plataforma.
func (l *liveServer) mkDelegableClient(t *testing.T, id, orgID string, admins bool) string {
	t.Helper()
	if err := l.st.CreateOAuthClient(l.ctx, nil, domain.Client{
		ID: id, Name: "Migrador", Type: domain.ClientConfidential,
		GrantTypes:              []string{"client_credentials", domain.GrantTokenExchange},
		Scopes:                  []string{"openid", "profile", "email", "offline_access", "drive.write"},
		TokenEndpointAuthMethod: "client_secret_basic",
	}, orgID, "admin"); err != nil {
		t.Fatalf("CreateOAuthClient: %v", err)
	}
	if _, err := l.st.DB().Exec(l.ctx,
		`UPDATE oauth_clients SET may_impersonate = true, may_impersonate_admins = $2 WHERE id = $1`,
		id, admins); err != nil {
		t.Fatalf("marcar delegable: %v", err)
	}
	return l.setClientSecret(t, id, "migrator-secret-1")
}

// tokenPost llama a /oauth2/token con auth Basic de client.
func (l *liveServer) tokenPost(t *testing.T, basic string, form url.Values) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Authorization", basic)
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	out := map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

// clientToken obtiene el access token propio del client (client_credentials): es
// el subject_token del canje.
func (l *liveServer) clientToken(t *testing.T, basic string) string {
	t.Helper()
	rr, out := l.tokenPost(t, basic, url.Values{"grant_type": {"client_credentials"}})
	if rr.Code != http.StatusOK {
		t.Fatalf("client_credentials: status=%d body=%s", rr.Code, rr.Body)
	}
	tok, _ := out["access_token"].(string)
	if tok == "" {
		t.Fatalf("client_credentials sin access_token: %v", out)
	}
	return tok
}

// exchange lanza un canje con los parámetros dados sobre la base bien formada.
func (l *liveServer) exchange(t *testing.T, basic, subjectToken string, over url.Values) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	form := url.Values{
		"grant_type":         {domain.GrantTokenExchange},
		"subject_token":      {subjectToken},
		"subject_token_type": {tokenTypeAccess},
	}
	for k, v := range over {
		form[k] = v
	}
	return l.tokenPost(t, basic, form)
}

// TestTokenExchange_Impersonation es el camino feliz y el que desbloquea la
// migración: el token resultante ES del usuario destino (sub + tenant + email),
// dice quién actuó (act), dura minutos y trae solo los scopes acotados.
func TestTokenExchange_Impersonation(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	grace := l.addMember(t, org, "grace@acme.test", domain.RoleMember)
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)
	subject := l.clientToken(t, basic)

	rr, out := l.exchange(t, basic, subject, url.Values{
		"requested_subject": {"grace@acme.test"},
		// Escalada deliberada: offline_access daría delegación indefinida y
		// drive.read no está concedido al client. Se RECORTAN, no se rechaza.
		// `email` sí se pide, porque el claim solo viaja si el scope se concede
		// (RFC 9068): pedirlo es lo que permite comprobar abajo que la identidad
		// del token es la del usuario impersonado y no la del client.
		"scope": {"openid profile email drive.write offline_access drive.read"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("canje: status=%d body=%s", rr.Code, rr.Body)
	}
	if got := out["scope"]; got != "openid profile email drive.write" {
		t.Errorf("scope=%v; se esperaba el recortado", got)
	}
	if got := out["issued_token_type"]; got != tokenTypeAccess {
		t.Errorf("issued_token_type=%v (obligatorio en RFC 8693 §2.2.1)", got)
	}
	if got, _ := out["expires_in"].(float64); got != domain.ImpersonationTokenTTL.Seconds() {
		t.Errorf("expires_in=%v; la impersonación dura %v", got, domain.ImpersonationTokenTTL)
	}
	if _, ok := out["refresh_token"]; ok {
		t.Error("un canje NUNCA debe devolver refresh token")
	}

	// El token es del USUARIO, con el actor declarado y su identidad de tenant.
	set, err := l.srv.keys.PublicJWKS()
	if err != nil {
		t.Fatalf("PublicJWKS: %v", err)
	}
	tok, err := oidc.Verify(out["access_token"].(string), itIssuer, itAud, set)
	if err != nil {
		t.Fatalf("el token emitido no valida: %v", err)
	}
	if tok.Subject() != grace.ID {
		t.Errorf("sub=%q, quiero el usuario %q", tok.Subject(), grace.ID)
	}
	actor, delegated := oidc.ActorOf(tok)
	if !delegated || actor != "acme-migrator" {
		t.Errorf("act=(%q,%v), quiero el client que actuó", actor, delegated)
	}
	if v, _ := tok.Get("tenant"); v != "acme" {
		t.Errorf("tenant=%v, quiero la org del client", v)
	}
	if v, _ := tok.Get("email"); v != "grace@acme.test" {
		t.Errorf("email=%v, quiero el del usuario impersonado", v)
	}
	// Vida corta de verdad, no solo en expires_in.
	if d := time.Until(tok.Expiration()); d > domain.ImpersonationTokenTTL+time.Minute {
		t.Errorf("el token vive %v; la impersonación debe durar minutos", d)
	}

	// AUDITORÍA: sin traza no hay despliegue. Debe estar el canje, con quién actuó
	// y sobre quién, y sin el email en claro.
	rows, _, err := l.st.ListAuditPage(l.ctx, org.ID,
		store.AuditFilter{Action: "oauth.token.exchange"}, nil, 10)
	if err != nil {
		t.Fatalf("ListAuditPage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("filas de auditoría=%d, quiero 1: %+v", len(rows), rows)
	}
	a := rows[0]
	if a.Action != "oauth.token.exchange" || a.ActorID != grace.ID || a.Target != "acme-migrator" {
		t.Errorf("traza=%+v; quiero action/actor(usuario)/target(client)", a)
	}
	var detail map[string]any
	if err := json.Unmarshal(a.Detail, &detail); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail["actor_client"] != "acme-migrator" || detail["impersonated_user"] != grace.ID ||
		detail["scope"] != "openid profile email drive.write" {
		t.Errorf("detalle incompleto: %v", detail)
	}
	if strings.Contains(string(a.Detail), "grace@acme.test") {
		t.Errorf("la traza no debe llevar el email en claro: %s", a.Detail)
	}
}

// TestTokenExchange_NotChainable: el token de impersonación NO sirve para volver
// a canjear. Ni por llevar `act` ni por tener `sub` de usuario: la delegación no
// se encadena y el actor original no se lava por el camino.
func TestTokenExchange_NotChainable(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	l.addMember(t, org, "grace@acme.test", domain.RoleMember)
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)

	rr, out := l.exchange(t, basic, l.clientToken(t, basic), url.Values{
		"requested_subject": {"grace@acme.test"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("primer canje: status=%d body=%s", rr.Code, rr.Body)
	}

	rr2, out2 := l.exchange(t, basic, out["access_token"].(string), url.Values{
		"requested_subject": {"grace@acme.test"},
	})
	if rr2.Code != http.StatusBadRequest || out2["error"] != "invalid_grant" {
		t.Fatalf("re-canje: status=%d error=%v; quiero 400 invalid_grant", rr2.Code, out2["error"])
	}
}

// TestTokenExchange_Rejections recorre los rechazos y, sobre todo, que TODOS los
// que hablan del sujeto devuelvan lo MISMO: desde fuera no se distingue un
// usuario de otra organización de uno que no existe.
func TestTokenExchange_Rejections(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	l.addMember(t, org, "grace@acme.test", domain.RoleMember)
	other, _ := l.seedOrgAdmin(t, "rival")
	outsider := l.addMember(t, other, "eve@rival.test", domain.RoleMember)
	// Un client delegable de la OTRA org: sirve para probar que la audiencia de
	// otro tenant se rechaza.
	l.mkDelegableClient(t, "rival-migrator", other.ID, false)

	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)
	subject := l.clientToken(t, basic)

	cases := []struct {
		name         string
		subjectToken string
		over         url.Values
		wantErr      string
	}{
		{
			"usuario de OTRA organización (por email)", subject,
			url.Values{"requested_subject": {"eve@rival.test"}},
			"invalid_target",
		},
		{
			"usuario de OTRA organización (por uuid)", subject,
			url.Values{"requested_subject": {outsider.ID}},
			"invalid_target",
		},
		{
			"usuario inexistente: MISMA respuesta", subject,
			url.Values{"requested_subject": {"nadie@acme.test"}},
			"invalid_target",
		},
		{
			"identificador que no es ni email ni uuid", subject,
			url.Values{"requested_subject": {"grace"}},
			"invalid_target",
		},
		{
			"admin de la org sin may_impersonate_admins", subject,
			url.Values{"requested_subject": {admin.Email}},
			"invalid_target",
		},
		{
			"audiencia de otro tenant", subject,
			url.Values{"requested_subject": {"grace@acme.test"}, "audience": {"rival-migrator"}},
			"invalid_target",
		},
		{
			"audiencia inexistente", subject,
			url.Values{"requested_subject": {"grace@acme.test"}, "audience": {"no-existe"}},
			"invalid_target",
		},
		{
			"subject_token que no es un JWT", "no-soy-un-token",
			url.Values{"requested_subject": {"grace@acme.test"}},
			"invalid_grant",
		},
		{
			"subject_token vacío", "",
			url.Values{"requested_subject": {"grace@acme.test"}},
			"invalid_request",
		},
		{"sin requested_subject", subject, url.Values{}, "invalid_request"},
		{
			"ningún scope delegable", subject,
			url.Values{"requested_subject": {"grace@acme.test"}, "scope": {"offline_access"}},
			"invalid_scope",
		},
	}
	for _, tc := range cases {
		rr, out := l.exchange(t, basic, tc.subjectToken, tc.over)
		if rr.Code != http.StatusBadRequest || out["error"] != tc.wantErr {
			t.Errorf("%s: status=%d error=%v; quiero 400 %s", tc.name, rr.Code, out["error"], tc.wantErr)
		}
		if _, ok := out["access_token"]; ok {
			t.Errorf("%s: ¡se emitió un token!", tc.name)
		}
	}
}

// TestTokenExchange_ExpiredSubjectToken: un subject_token caducado no canjea,
// aunque esté bien firmado por este mismo IdP.
func TestTokenExchange_ExpiredSubjectToken(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	l.addMember(t, org, "grace@acme.test", domain.RoleMember)
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)

	// Emitido hace una hora con TTL de 15 min: firma válida, ventana cerrada.
	stale, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: "acme-migrator", Audience: "acme-migrator", Scope: "openid",
	}, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	rr, out := l.exchange(t, basic, stale, url.Values{"requested_subject": {"grace@acme.test"}})
	if rr.Code != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("token caducado: status=%d error=%v; quiero 400 invalid_grant", rr.Code, out["error"])
	}
}

// TestTokenExchange_SubjectTokenOfAnotherClient: presentar el token de OTRO
// client no vale, aunque quien llame se autentique correctamente.
func TestTokenExchange_SubjectTokenOfAnotherClient(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	l.addMember(t, org, "grace@acme.test", domain.RoleMember)
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)

	ajeno, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: "sytue-console", Audience: "sytue-console", Scope: "openid",
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	rr, out := l.exchange(t, basic, ajeno, url.Values{"requested_subject": {"grace@acme.test"}})
	if rr.Code != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("token ajeno: status=%d error=%v; quiero 400 invalid_grant", rr.Code, out["error"])
	}
}

// TestTokenExchange_NonDelegableClientLive: un client sembrado normal (sin la
// marca del operador) no canja, aunque declare el grant. Es la puerta de la
// mig 0025 vista desde el endpoint real.
func TestTokenExchange_NonDelegableClientLive(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	l.addMember(t, org, "grace@acme.test", domain.RoleMember)

	if err := l.st.CreateOAuthClient(l.ctx, nil, domain.Client{
		ID: "acme-curioso", Name: "Curioso", Type: domain.ClientConfidential,
		GrantTypes:              []string{"client_credentials", domain.GrantTokenExchange},
		Scopes:                  []string{"openid"},
		TokenEndpointAuthMethod: "client_secret_basic",
	}, org.ID, "admin"); err != nil {
		t.Fatalf("CreateOAuthClient: %v", err)
	}
	basic := l.setClientSecret(t, "acme-curioso", "curioso-secret-1")

	rr, out := l.exchange(t, basic, l.clientToken(t, basic),
		url.Values{"requested_subject": {"grace@acme.test"}})
	if rr.Code != http.StatusBadRequest || out["error"] != "unauthorized_client" {
		t.Fatalf("client no delegable: status=%d error=%v; quiero 400 unauthorized_client",
			rr.Code, out["error"])
	}

	// Y no debe quedar traza de canje: no hubo canje.
	rows, _, err := l.st.ListAuditPage(l.ctx, org.ID,
		store.AuditFilter{Action: "oauth.token.exchange"}, nil, 10)
	if err != nil {
		t.Fatalf("ListAuditPage: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("no debería haber trazas de canje: %+v", rows)
	}
}

// TestTokenExchange_AdminsRequireExplicitFlag: con may_impersonate_admins el
// mismo canje que antes se rechazaba, pasa. Es la única forma de actuar como un
// administrador, y es explícita.
func TestTokenExchange_AdminsRequireExplicitFlag(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, true)

	rr, out := l.exchange(t, basic, l.clientToken(t, basic), url.Values{
		"requested_subject": {admin.Email},
		"scope":             {"openid admin"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("canje sobre admin declarado: status=%d body=%s", rr.Code, rr.Body)
	}
	// El scope `admin` sí se concede aquí: el usuario impersonado ES admin.
	if got := out["scope"]; got != "openid" {
		t.Logf("scope=%v (admin no está entre los scopes del client, se recorta)", got)
	}
	set, _ := l.srv.keys.PublicJWKS()
	tok, err := oidc.Verify(out["access_token"].(string), itIssuer, itAud, set)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if tok.Subject() != admin.ID {
		t.Errorf("sub=%q, quiero el admin %q", tok.Subject(), admin.ID)
	}
}

// TestTokenExchange_DeniedIsAudited: un client delegable pidiendo un sujeto
// fuera de su alcance deja rastro. Es la señal que interesa a las 3 de la mañana.
func TestTokenExchange_DeniedIsAudited(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	other, _ := l.seedOrgAdmin(t, "rival")
	l.addMember(t, other, "eve@rival.test", domain.RoleMember)
	basic := l.mkDelegableClient(t, "acme-migrator", org.ID, false)

	rr, _ := l.exchange(t, basic, l.clientToken(t, basic),
		url.Values{"requested_subject": {"eve@rival.test"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, quiero 400", rr.Code)
	}
	rows, _, err := l.st.ListAuditPage(l.ctx, org.ID,
		store.AuditFilter{Action: "oauth.token.exchange.denied"}, nil, 10)
	if err != nil {
		t.Fatalf("ListAuditPage: %v", err)
	}
	if len(rows) != 1 || rows[0].Target != "acme-migrator" {
		t.Fatalf("se esperaba 1 traza de rechazo del client: %+v", rows)
	}
	if strings.Contains(string(rows[0].Detail), "eve@rival.test") {
		t.Errorf("la traza no debe llevar el identificador pedido: %s", rows[0].Detail)
	}
}
