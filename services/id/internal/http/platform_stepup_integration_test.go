// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Step-up MFA de la ADMINISTRACIÓN DE PLATAFORMA (contra PostgreSQL real).
//
// La API global exige segundo factor SIEMPRE (platform.go), pero nadie lo pedía
// al entrar: el alcance de plataforma no es un rol de organización y la verja de
// /authorize solo miraba roles. El admin global entraba en platform.sytue.eu sin
// MFA y la consola respondía 403 en todas sus llamadas, sin decir cómo salir.
package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const platformClient = "sytue-platform"

// authorizeLoc corre /authorize con la sesión dada y devuelve el Location y las
// cookies emitidas (a diferencia de authorizeCode, no exige que haya código: lo
// que se comprueba aquí es justo el desvío previo).
func (l *liveServer) authorizeLoc(t *testing.T, sess *http.Cookie) (string, []*http.Cookie) {
	t.Helper()
	authz := "/oauth2/authorize?" + url.Values{
		"client_id":             {platformClient},
		"redirect_uri":          {"https://platform.sytue.eu/auth/callback"},
		"response_type":         {"code"},
		"scope":                 {"openid profile email offline_access"},
		"state":                 {"st-1"},
		"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
		"code_challenge_method": {"S256"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, authz, http.NoBody)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("/authorize código=%d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Header().Get("Location"), rec.Result().Cookies()
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestPlatformAdminExigeSegundoFactorEnAuthorize(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "global@id.test", "")

	// Sesión de quien entró solo con contraseña: amr=["pwd"], sin segundo factor.
	// Se inserta como en el resto de tests del paquete (el harness no fija
	// SessionTTL, así que la que abriría el propio /login/ nacería caducada).
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: u.ID,
		AuthTime: now, AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	sess := &http.Cookie{Name: sessionCookieName, Value: raw}

	// Sin alcance global no cambia nada: un usuario corriente sigue pasando.
	if loc, _ := l.authorizeLoc(t, sess); !strings.Contains(loc, "code=") {
		t.Fatalf("usuario corriente sin MFA: Location=%q, quiero el código de autorización", loc)
	}

	// Con alcance global y SIN segundo factor enrolado: a enrolarlo. Mandarle al
	// formulario del código sería un callejón sin salida —no tiene ninguno—.
	if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, u.ID, "prueba"); err != nil {
		t.Fatalf("UpsertPlatformAdmin: %v", err)
	}
	loc, _ := l.authorizeLoc(t, sess)
	if loc != "/account?mfa=required" {
		t.Fatalf("admin global sin MFA: Location=%q, quiero /account?mfa=required", loc)
	}

	// Con segundo factor enrolado: step-up. Y CON token de continuación emitido —
	// sin él, el código correcto acababa en «Vuelve a iniciar sesión» y volver a
	// entrar traía otra vez aquí: un bucle cerrado.
	l.enrollMFA(t, u.ID)
	loc, cookies := l.authorizeLoc(t, sess)
	if !strings.Contains(loc, "step_up=1") {
		t.Fatalf("admin global con MFA: Location=%q, quiero el step-up", loc)
	}
	if cookieNamed(cookies, mfaCookieName) == nil {
		t.Fatalf("el step-up no emitió el token de continuación MFA: cookies=%v", cookies)
	}
}

// TestCuentaPersonalNoExigeSegundoFactorEnAuthorize — Una persona sola en su
// cuenta gratuita es owner de su propia organización (crearCuentaPersonal), pero
// NO administra una empresa: exigirle step-up MFA en cada authorize la metía en
// un bucle de redirecciones infinito (ADR 0038).
func TestCuentaPersonalNoExigeSegundoFactorEnAuthorize(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{
		Slug: domain.PersonalSlug("consumidor@personal.test"), Name: "consumidor@personal.test", Kind: domain.OrgKindPersonal,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	u := caOrgUser(t, l, org, "consumidor@personal.test")

	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: u.ID, OrgID: org.ID,
		AuthTime: now, AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	sess := &http.Cookie{Name: sessionCookieName, Value: raw}

	authz := "/oauth2/authorize?" + url.Values{
		"client_id":             {"sytue-one"},
		"redirect_uri":          {"https://corporate.sytue.eu/one/auth/callback"},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {"st-personal"},
		"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
		"code_challenge_method": {"S256"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, authz, http.NoBody)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("código=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "mfa=required") || strings.Contains(loc, "/account") {
		t.Fatalf("cuenta personal fue desviada a step-up: loc=%s", loc)
	}
	if !strings.HasPrefix(loc, "https://corporate.sytue.eu/one/auth/callback?code=") {
		t.Fatalf("se esperaba callback con code, recibido: %s", loc)
	}
}
