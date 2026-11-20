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

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
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
		// La clave efímera del puente de la app (Safari aparta su almacén): se usa.
		{"web con la clave de su puente", authzRequest{Unlock: true, UnlockKey: clave}, web, true, clave},
		{"web con clave rota", authzRequest{Unlock: true, UnlockKey: "abc"}, web, true, ""},
		{"web con clave pero sin pedirlo", authzRequest{UnlockKey: clave}, web, false, ""},
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

// Passkey preferida, contraseña de respaldo (ADR 0085): en un navegador donde la
// última persona que entró tiene una passkey que abre su cifrado, la página ofrece
// PRIMERO la passkey, como botón principal, y la contraseña debajo; sin esa señal,
// la de siempre. En los dos casos hay un solo botón de passkey y el formulario de
// contraseña sigue ahí.
func TestLaPaginaDeInicioOfrecePrimeroLaPasskeySiLaCuentaLaTiene(t *testing.T) {
	s := &Server{}
	h := securityHeaders(Config{})(http.HandlerFunc(s.handleLoginForm))
	pagina := func(cookie bool) string {
		req := httptest.NewRequest(http.MethodGet, "/login/", http.NoBody)
		if cookie {
			req.AddCookie(&http.Cookie{Name: passkeyFirstCookie, Value: "1"})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		guionesEnLaCSP(t, rec.Body.String(), rec.Header().Get("Content-Security-Policy"))
		return rec.Body.String()
	}
	for _, c := range []struct {
		cookie         bool
		passkeyPrimero bool
	}{{false, false}, {true, true}} {
		p := pagina(c.cookie)
		boton, form := strings.Index(p, `id="passkey-btn"`), strings.Index(p, `<form method="post" action="/login/">`)
		if boton < 0 || form < 0 || strings.Count(p, `id="passkey-btn"`) != 1 || !strings.Contains(p, `name="password"`) {
			t.Fatalf("cookie=%v: falta el botón, el formulario o hay dos botones", c.cookie)
		}
		if (boton < form) != c.passkeyPrimero || strings.Contains(p, `class="pk-btn pk-first"`) != c.passkeyPrimero {
			t.Errorf("cookie=%v: passkey primero = %v, quiero %v", c.cookie, boton < form, c.passkeyPrimero)
		}
		// Y donde el navegador lo permite, la passkey en el autocompletado del campo.
		if !strings.Contains(p, "mediation:'conditional'") || !strings.Contains(p, "'username webauthn'") {
			t.Errorf("cookie=%v: sin mediación condicional", c.cookie)
		}
	}

	// La señal la pone el relevo según los sobres de la persona: con uno de passkey,
	// sí; con solo el de contraseña, se quita.
	rec := httptest.NewRecorder()
	s.recordarPasskeyPrimero(rec, []recoverywrap.Envelope{{Kind: recoverywrap.KindPassword}, {Kind: recoverywrap.KindPasskey}})
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].Value != "1" || !c[0].HttpOnly || c[0].MaxAge <= 0 {
		t.Fatalf("con passkey: %+v", c)
	}
	rec = httptest.NewRecorder()
	s.recordarPasskeyPrimero(rec, []recoverywrap.Envelope{{Kind: recoverywrap.KindPassword}})
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("sin passkey: %+v", c)
	}
}

// /e2e/passkey solo vuelve a una ruta de Sytue ID o a una app de la suite: nunca a
// otro sitio (sería una redirección abierta con la sesión).
func TestLaPasskeyNuevaSoloVuelveALaSuite(t *testing.T) {
	s := &Server{cfg: Config{Env: pcfg.EnvProd, Issuer: "https://id.sytue.eu"}}
	for v, quiero := range map[string]string{
		"/account?pk=ok#claves":                       "/account?pk=ok#claves",
		"https://id.sytue.eu/account/seguridad?x=1#c": "/account/seguridad?x=1#c",
		"https://account.sytue.eu/account":            "https://account.sytue.eu/account",
		"https://mail.sytue.eu/mail/":                 "https://mail.sytue.eu/mail/",
		"//evil.example/x":                            "",
		"https://evil.example/":                       "",
		"https://x.apps.sytue.eu/":                    "",
		"/\\evil.example":                             "",
		"javascript:alert(1)":                         "",
		"https://a@mail.sytue.eu/":                    "",
	} {
		got, ok := s.vueltaDelAlta(v)
		if (quiero == "") == ok || got != quiero {
			t.Errorf("%q: %q %v; quiero %q", v, got, ok, quiero)
		}
	}
}

// El puente de «Mi cuenta» (su alias `account.<dominio>` y el del propio origen de
// Sytue ID, /e2e/cuenta) es el único que rehace el sobre de contraseña: lleva
// Argon2id y su CSP compila WebAssembly. El de cualquier otra app, no.
func TestSoloElPuenteDeLaCuentaRehaceLaContrasena(t *testing.T) {
	s := &Server{cfg: Config{Env: pcfg.EnvProd, Issuer: "https://id.sytue.eu"}}
	pedir := func(h http.HandlerFunc, ruta string) (string, string) {
		rec := httptest.NewRecorder()
		securityHeaders(s.cfg)(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ruta, http.NoBody))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", ruta, rec.Code)
		}
		guionesEnLaCSP(t, rec.Body.String(), rec.Header().Get("Content-Security-Policy"))
		return rec.Body.String(), rec.Header().Get("Content-Security-Policy")
	}
	page, csp := pedir(s.handleUnlockBridge, "/e2e/bridge?o="+url.QueryEscape("https://mail.sytue.eu"))
	if strings.Contains(page, `"password":true`) || strings.Contains(page, "npmjs.com/package/hash-wasm") || strings.Contains(csp, "wasm") {
		t.Error("el puente de Mail no debe poder rehacer la contraseña")
	}
	if !strings.Contains(csp, "img-src 'self'") {
		t.Error("la CSP del puente debe permitir img-src 'self' para el favicon declarado en head")
	}
	page, csp = pedir(s.handleUnlockBridge, "/e2e/bridge?o="+url.QueryEscape("https://account.sytue.eu"))
	if !strings.Contains(page, `"password":true`) || !strings.Contains(page, "npmjs.com/package/hash-wasm") || !strings.Contains(csp, "'wasm-unsafe-eval'") {
		t.Error("el puente del alias de «Mi cuenta» debe poder rehacer la contraseña")
	}
	page, csp = pedir(s.handleUnlockBridgeCuenta, "/e2e/cuenta")
	if !strings.Contains(page, `"origin":"https://id.sytue.eu"`) || !strings.Contains(page, `"password":true`) ||
		!strings.Contains(csp, "frame-ancestors 'self';") {
		t.Errorf("/e2e/cuenta: %q", csp)
	}
}
