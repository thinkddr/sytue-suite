// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

var guionEnLinea = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// guionesEnLaCSP comprueba que cada <script> en línea de la página tiene su hash en
// la CSP: si falta uno, el navegador lo bloquea y el desbloqueo no ocurre, sin
// ningún error visible.
func guionesEnLaCSP(t *testing.T, page, csp string) {
	t.Helper()
	guiones := guionEnLinea.FindAllStringSubmatch(page, -1)
	if len(guiones) == 0 {
		t.Fatal("la página no lleva guiones")
	}
	for _, g := range guiones {
		sum := sha256.Sum256([]byte(g[1]))
		if !strings.Contains(csp, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
			t.Errorf("guion sin hash en la CSP: %.60q…", g[1])
		}
	}
}

// El puente es la única página del IdP que vive en un iframe, y por eso la más
// delicada: solo se deja incrustar por UN origen de la suite, el que pide, con la
// misma regla que CORS (nunca `x.apps.sytue.eu`, donde corre código de clientes),
// y sin X-Frame-Options que lo contradiga.
func TestElPuenteSoloSeDejaIncrustarPorUnOrigenDeLaSuite(t *testing.T) {
	s := &Server{cfg: Config{Env: pcfg.EnvProd, Issuer: "https://id.sytue.eu"}}
	h := securityHeaders(s.cfg)(http.HandlerFunc(s.handleUnlockBridge))
	for origen, permitido := range map[string]bool{
		"https://mail.sytue.eu":          true,
		"https://corporate.sytue.eu":     true, // host de organización: una etiqueta
		"https://x.apps.sytue.eu":        false,
		"https://pr-1.preview.sytue.eu":  false,
		"https://mail.sytue.eu.evil.com": false,
		"https://evil.example":           false,
		"https://mail.sytue.eu/":         false,
		"https://mail.sytue.eu/x":        false,
		"http://mail.sytue.eu":           false,
		"https://id.sytue.eu":            false,
		"https://mail.sytue.localhost":   false, // dev, no en producción
		"":                               false,
		"https://mail.sytue.eu?x=1":      false,
		"https://a@mail.sytue.eu":        false,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/e2e/bridge?o="+url.QueryEscape(origen), http.NoBody))
		if !permitido {
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SytueE2E") {
				t.Errorf("%q: status %d; quiero 404 sin guion", origen, rec.Code)
			}
			continue
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if rec.Code != http.StatusOK || !strings.Contains(csp, "frame-ancestors "+origen+";") {
			t.Fatalf("%q: status %d, CSP %q", origen, rec.Code, csp)
		}
		if strings.Contains(csp, "*") || strings.Contains(csp, "'unsafe-inline'") || strings.Contains(csp, "connect-src 'self'") {
			t.Errorf("%q: CSP demasiado ancha: %q", origen, csp)
		}
		if xfo := rec.Header().Get("X-Frame-Options"); xfo != "" {
			t.Errorf("%q: X-Frame-Options %q impide el iframe que la CSP permite", origen, xfo)
		}
		if !strings.Contains(rec.Body.String(), `{"origin":"`+origen+`"}`) {
			t.Errorf("%q: el guion no recibe el origen validado", origen)
		}
		guionesEnLaCSP(t, rec.Body.String(), csp)
	}
}

// En desarrollo, los hosts de la suite van por http en *.localhost.
func TestElPuenteEnDesarrolloAceptaLaSuiteLocal(t *testing.T) {
	s := &Server{cfg: Config{Env: pcfg.EnvDev, Issuer: "https://id.sytue.localhost"}}
	if _, ok := s.origenDelPuente("https://mail.sytue.localhost"); !ok {
		t.Error("https://mail.sytue.localhost rechazado en dev")
	}
	if _, ok := s.origenDelPuente("https://x.apps.sytue.localhost"); ok {
		t.Error("x.apps aceptado en dev")
	}
}

// La página de inicio de sesión deriva la KEK en el navegador: lleva Argon2id, lo
// común y su guion, y la CSP deja compilar WebAssembly (solo eso) y conoce el hash
// de cada guion. La del segundo factor no tiene contraseña: sin Argon2id.
func TestLaPaginaDeInicioDeSesionDerivaEnElNavegador(t *testing.T) {
	s := &Server{}
	h := securityHeaders(Config{})(http.HandlerFunc(s.handleLoginForm))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login/?return_to=%2Foauth2%2Fauthorize%3Fclient_id%3Dx", http.NoBody))
	page, csp := rec.Body.String(), rec.Header().Get("Content-Security-Policy")
	for _, parte := range []string{"npmjs.com/package/hash-wasm", "window.SytueE2E", "/login/unlock/salt", "E.withPRF(pk)", "E.rememberPasskey(c)"} {
		if !strings.Contains(page, parte) {
			t.Errorf("la página no lleva %q", parte)
		}
	}
	if !strings.Contains(csp, "script-src 'wasm-unsafe-eval' 'sha256-") || strings.Contains(csp, "'unsafe-eval'") && !strings.Contains(csp, "'wasm-unsafe-eval'") {
		t.Errorf("CSP de la página de inicio de sesión: %q", csp)
	}
	guionesEnLaCSP(t, page, csp)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login/?step_up=1", http.NoBody))
	if strings.Contains(rec.Body.String(), "npmjs.com/package/hash-wasm") || !strings.Contains(rec.Body.String(), "E.rememberPasskey(c)") {
		t.Error("el segundo factor debe pedir la PRF de la passkey y no cargar Argon2id")
	}
	if strings.Contains(rec.Header().Get("Content-Security-Policy"), "wasm") {
		t.Error("el segundo factor no compila WebAssembly")
	}
	guionesEnLaCSP(t, rec.Body.String(), rec.Header().Get("Content-Security-Policy"))
}

// El relevo solo se pinta cuando la app de la suite lo pide; una app de terceros
// o una nativa sin clave válida reciben la redirección de siempre.
func TestElRelevoSoloSiUnaAppDeLaSuiteLoPide(t *testing.T) {
	// El generador de P-256: un punto válido cualquiera.
	g, err := hex.DecodeString("04" + "6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296" +
		"4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5")
	if err != nil {
		t.Fatal(err)
	}
	clave := base64.RawURLEncoding.EncodeToString(g)
	web := domain.Client{ID: "sytue-mail", Type: domain.ClientConfidential}
	nativa := domain.Client{ID: "sytue-desktop", Type: domain.ClientPublic}
	terceros := domain.Client{ID: "app", Type: domain.ClientPublic, OrgID: "acme"}
	for _, c := range []struct {
		nombre string
		req    authzRequest
		client domain.Client
		ok     bool
		key    string
	}{
		{"web lo pide", authzRequest{Unlock: true}, web, true, ""},
		{"web no lo pide", authzRequest{}, web, false, ""},
		{"nativa con clave", authzRequest{UnlockKey: clave}, nativa, true, clave},
		{"nativa con clave rota", authzRequest{UnlockKey: "abc"}, nativa, false, ""},
		{"nativa sin clave", authzRequest{Unlock: true}, nativa, false, ""},
		{"terceros", authzRequest{Unlock: true, UnlockKey: clave}, terceros, false, ""},
	} {
		key, ok := quiereRelevo(c.req, c.client)
		if ok != c.ok || key != c.key {
			t.Errorf("%s: (%q, %v); quiero (%q, %v)", c.nombre, key, ok, c.key, c.ok)
		}
	}
}

// Los datos del relevo van en un bloque JSON dentro del documento: un destino
// hostil no puede cerrarlo y meter un guion.
func TestElRelevoNoSeDejaInyectarPorElDestino(t *testing.T) {
	raw, err := json.Marshal(relevoDatos{Sub: "u", Next: "https://x/</script><script>alert(1)</script>"})
	if err != nil {
		t.Fatal(err)
	}
	page := relevoPage("es", "https://x/cb?code=1&state=2", string(raw))
	if strings.Contains(page, "<script>alert") {
		t.Fatal("el destino cierra el bloque de datos")
	}
	for _, id := range []string{`id="e2e-relay"`, `id="e2e-lost"`, `id="e2e-passkey-first" hidden`, `id="e2e-restart" disabled`, `id="e2e-understood"`, "<noscript>"} {
		if !strings.Contains(page, id) {
			t.Errorf("al relevo le falta %s", id)
		}
	}
	guionesEnLaCSP(t, page, cspScriptHashes)
}

// `prompt=login` obliga a entrar otra vez, y la vuelta ya no lo lleva: si lo
// llevara, cada vuelta pediría otro inicio de sesión.
func TestLaVueltaDePromptLoginNoLoRepite(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?client_id=a&prompt=login&state=s", http.NoBody)
	if !pidePromptLogin(r) {
		t.Fatal("prompt=login no detectado")
	}
	if got := uriSinPrompt(r); strings.Contains(got, "prompt") || !strings.Contains(got, "state=s") {
		t.Fatalf("vuelta = %q", got)
	}
	r = httptest.NewRequest(http.MethodGet, "/oauth2/authorize?prompt=consent+login", http.NoBody)
	if !pidePromptLogin(r) {
		t.Fatal("prompt con varios valores")
	}
	r = httptest.NewRequest(http.MethodGet, "/oauth2/authorize?prompt=none&x=1", http.NoBody)
	if pidePromptLogin(r) {
		t.Fatal("prompt=none no es login")
	}
}

// La página de cierre de sesión borra el almacén del desbloqueo antes de irse.
func TestElCierreDeSesionOlvidaLaClaveDeDesbloqueo(t *testing.T) {
	page := logoutSweepPage(nil, "https://www.sytue.eu/")
	if !strings.Contains(page, "E.store.clear()") || !strings.Contains(page, "window.SytueE2E") {
		t.Fatal("el cierre no borra el almacén del desbloqueo")
	}
	guionesEnLaCSP(t, page, cspScriptHashes)
}
