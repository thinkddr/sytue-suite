// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// nativa es una app nativa de la suite pidiendo entrar: su client público, su vuelta y
// el secreto de su dispositivo ("" = una versión que no lo manda).
type nativa struct {
	client, vuelta, secreto string
}

func (n nativa) hash() string {
	if n.secreto == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(n.secreto))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// pedirAutorizacion abre /authorize como la app y devuelve la respuesta y el
// verificador de PKCE con el que canjear.
func (l *liveServer) pedirAutorizacion(t *testing.T, n nativa, sess *http.Cookie, ruta string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	verifier, _ := domain.GenerateToken(32)
	if ruta == "" {
		sum := sha256.Sum256([]byte(verifier))
		q := url.Values{
			"client_id": {n.client}, "redirect_uri": {n.vuelta}, "response_type": {"code"},
			"scope": {"openid offline_access"}, "state": {"st"},
			"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		}
		if h := n.hash(); h != "" {
			q.Set(paramDispositivo, h)
		}
		ruta = "/oauth2/authorize?" + q.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	return rec, verifier
}

var enlacePermitir = regexp.MustCompile(`<a class="cta" href="([^"]+)"`)

// codigoDe saca el código de una redirección a la app ("" si no la hubo).
func codigoDe(rec *httptest.ResponseRecorder) string {
	if rec.Code != http.StatusFound {
		return ""
	}
	u, _ := url.Parse(rec.Header().Get("Location"))
	return u.Query().Get("code")
}

// canjear cambia el código como lo hace la app (client público, sin secreto de client).
func (l *liveServer) canjear(t *testing.T, n nativa, code, verifier, secreto string) int {
	t.Helper()
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {n.vuelta},
		"client_id": {n.client}, "code_verifier": {verifier},
	}
	if secreto != "" {
		form.Set(paramSecretoDispositivo, secreto)
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	return rec.Code
}

// NAT-02. Con una sesión abierta en el navegador, una app nativa (client público) se
// llevaba el código sin que nadie viera nada. Ahora la primera vez en cada dispositivo
// Sytue ID pregunta, y después ya no; el código queda atado al dispositivo, así que el
// «ya no pregunto» no sirve a quien copie el hash sin tener el secreto.
//
// Visto en rojo contra main: /authorize respondía 302 con código a la primera.
func TestUnaAppNativaPreguntaUnaVezPorDispositivo(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	ana := l.memberSession(t, "ana@acme.example", org.ID)
	escritorio := nativa{client: "sytue-desktop", vuelta: "http://127.0.0.1:8921/auth/callback", secreto: "secreto-del-portatil-de-ana-32-bytes!"}

	// 1) Primera vez: pregunta, no hay código.
	rec, _ := l.pedirAutorizacion(t, escritorio, ana, "")
	if rec.Code != http.StatusOK || codigoDe(rec) != "" || !strings.Contains(rec.Body.String(), `class="cta"`) {
		t.Fatalf("primera vez: %d %s; quiero la pantalla de consentimiento", rec.Code, rec.Header().Get("Location"))
	}
	m := enlacePermitir.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatal("la pantalla no trae el enlace de «Permitir»")
	}
	permitir := html.UnescapeString(m[1])

	// 2) El «Permitir» de la sesión de Ana no vale en la de otra persona (ni lo
	// fabrica una página ajena): vuelve a preguntar.
	bea := l.memberSession(t, "bea@acme.example", org.ID)
	if rec, _ := l.pedirAutorizacion(t, escritorio, bea, permitir); codigoDe(rec) != "" {
		t.Fatal("el «Permitir» de Ana ha dado un código en la sesión de Bea")
	}

	// 3) Ana permite: código, atado al dispositivo.
	rec, _ = l.pedirAutorizacion(t, escritorio, ana, permitir)
	code := codigoDe(rec)
	if code == "" {
		t.Fatalf("Ana permite: %d %s", rec.Code, rec.Body.String())
	}
	// El verificador de PKCE es el de la petición que pintó la pantalla; se rehace el
	// flujo entero para tener uno conocido: ya no pregunta (paso 4).
	_ = code

	// 4) Segunda vez en el mismo dispositivo: no pregunta.
	rec, verifier := l.pedirAutorizacion(t, escritorio, ana, "")
	code = codigoDe(rec)
	if code == "" {
		t.Fatalf("segunda vez en el mismo dispositivo: %d; no tenía que preguntar", rec.Code)
	}
	// Y el código solo lo canjea quien tiene el secreto.
	if st := l.canjear(t, escritorio, code, verifier, ""); st != http.StatusBadRequest {
		t.Errorf("canje sin el secreto del dispositivo: %d, quiero 400", st)
	}
	if st := l.canjear(t, escritorio, code, verifier, "otro-secreto"); st != http.StatusBadRequest {
		t.Errorf("canje con un secreto ajeno: %d, quiero 400", st)
	}
	if st := l.canjear(t, escritorio, code, verifier, escritorio.secreto); st != http.StatusOK {
		t.Errorf("canje con el secreto del dispositivo: %d, quiero 200", st)
	}

	// 5) Otro dispositivo, u otra app del móvil: pregunta otra vez.
	otro := escritorio
	otro.secreto = "secreto-de-otro-equipo-de-32-bytes!!"
	if rec, _ := l.pedirAutorizacion(t, otro, ana, ""); codigoDe(rec) != "" {
		t.Error("otro dispositivo se ha llevado el código sin preguntar")
	}
	movil := nativa{client: "sytue-mobile", vuelta: "sytue-mail://auth/callback", secreto: escritorio.secreto}
	if rec, _ := l.pedirAutorizacion(t, movil, ana, ""); codigoDe(rec) != "" || !strings.Contains(rec.Body.String(), "Sytue Mail") {
		t.Errorf("Sytue Mail en el móvil: %d; quiero la pregunta con su nombre", rec.Code)
	}

	// 6) Una versión que no manda dispositivo pregunta SIEMPRE.
	vieja := nativa{client: "sytue-desktop", vuelta: escritorio.vuelta}
	for i := range 2 {
		if rec, _ := l.pedirAutorizacion(t, vieja, ana, ""); codigoDe(rec) != "" {
			t.Fatalf("app sin dispositivo, vez %d: código sin preguntar", i+1)
		}
	}
}

// Los tres redirects huérfanos de la 0071 (`sytue://`, `sytue-cloud://` y
// `sytue-admin://`) no los declara ninguna app: otra app del teléfono podía registrar
// el esquema y llevarse el código. La 0114 los quita.
func TestSinRedirectsMovilesHuerfanos(t *testing.T) {
	l := newLiveServer(t)
	c, err := l.st.GetClient(l.ctx, "sytue-mobile")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"sytue://auth/callback", "sytue-cloud://auth/callback", "sytue-admin://auth/callback"} {
		if c.ValidRedirectURI(h) {
			t.Errorf("%s sigue registrado en sytue-mobile", h)
		}
	}
	if !c.ValidRedirectURI("sytue-mail://auth/callback") {
		t.Error("se ha ido también la vuelta de Sytue Mail")
	}
}
