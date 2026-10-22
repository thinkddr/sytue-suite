// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El barrido solo alcanza a las superficies de la suite de ESTE entorno. Un
// redirect_uri de dev (o de un tercero registrado por DCR) no puede convertir el
// cierre de sesión en una petición a un host ajeno desde el navegador del usuario.
func TestBarridoSoloAlcanzaLaSuiteDelEntorno(t *testing.T) {
	dentro := map[string]string{
		"https://mail.sytue.eu/auth/callback":     "https://mail.sytue.eu/auth/logout?scope=local",
		"https://platform.sytue.eu/auth/callback": "https://platform.sytue.eu/auth/logout?scope=local",
		"https://sytue.eu/auth/callback":          "https://sytue.eu/auth/logout?scope=local",
		// La forja impone su callback y cierra por su propia ruta (Gitea 1.27:
		// `m.Get("/user/logout", auth.SignOut)`), no por el BFF de la suite.
		"https://git.sytue.eu/user/oauth2/sytue-id/callback": "https://git.sytue.eu/user/logout",
		// EL HOST DE ORGANIZACIÓN. Ahí las diez superficies viven juntas bajo su
		// prefijo, y el cierre tiene que ir al MISMO prefijo: bajo ese host solo
		// `/` tiene regla propia, así que `/auth/logout` a pelo es un 404 —se vio
		// en producción—. Exigiendo la raíz, estos diez clients se descartaban
		// enteros y cerrar sesión desde la dirección de la organización dejaba
		// abierta toda la suite sin decir nada.
		"https://corporate.sytue.eu/mail/auth/callback": "https://corporate.sytue.eu/mail/auth/logout?scope=local",
		"https://corporate.sytue.eu/one/auth/callback":  "https://corporate.sytue.eu/one/auth/logout?scope=local",
	}
	for in, want := range dentro {
		if got := sweepURI(in, "sytue.eu"); got != want {
			t.Errorf("sweepURI(%q) = %q, quiero %q", in, got, want)
		}
	}
	fuera := []string{
		"https://mail.sytue.localhost/auth/callback", // otro entorno
		"http://localhost:4322/auth/callback",
		"https://sytue.eu.evil.com/auth/callback", // sufijo sin punto
		"https://evil.com/auth/callback",
		"javascript:alert(1)",
		"",
		// Callback que no es de ninguna aplicación que sepamos cerrar: mejor no
		// pedir nada que un 404 por cada cierre de sesión.
		"https://raro.sytue.eu/otro/callback",
		// Y el sufijo no puede aflojar la regla: `/auth/callback` tiene que ser
		// un segmento completo, no el final de otra palabra.
		"https://raro.sytue.eu/noesauth/callback",
	}
	for _, in := range fuera {
		if got := sweepURI(in, "sytue.eu"); got != "" {
			t.Errorf("sweepURI(%q) = %q, quiero descartarlo", in, got)
		}
	}
}

// La página de cierre tiene que pedir el borrado a TODAS las superficies y llevar
// al destino. Si alguna de las dos cosas se pierde, cerrar sesión deja sesiones
// vivas o al usuario plantado en una pantalla intermedia.
func TestPaginaDeCierreBarreYNavega(t *testing.T) {
	uris := []string{
		"https://mail.sytue.eu/auth/logout?scope=local",
		"https://one.sytue.eu/auth/logout?scope=local",
	}
	page := logoutSweepPage(uris, "https://www.sytue.eu/")
	for _, u := range uris {
		if !strings.Contains(page, `<img src="`+strings.ReplaceAll(u, "&", "&amp;")+`"`) {
			t.Errorf("la página no barre %q", u)
		}
	}
	if !strings.Contains(page, `data-next="https://www.sytue.eu/"`) {
		t.Error("la página no lleva el destino post-logout")
	}
	// Sin JS el usuario tiene que poder seguir a mano.
	if !strings.Contains(page, "<noscript>") {
		t.Error("la página no ofrece salida sin JavaScript")
	}
}

// La URL de la pantalla de cierre lleva el `id_token_hint` —un JWT del usuario—
// en la query, y esa pantalla carga un subrecurso de CADA superficie de la suite.
// Sin `Referrer-Policy: no-referrer`, el navegador mandaría esa URL entera en la
// cabecera Referer de las once peticiones: el ID token repartido a once orígenes
// en cada cierre de sesión. La cabecera ya estaba por otros motivos; desde que
// existe el barrido es además un requisito suyo, y esto lo deja escrito.
func TestLaPantallaDeCierreNoFiltraElIDTokenPorReferer(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/logout?id_token_hint=eyJhbGciOi", http.NoBody)
	securityHeaders(Config{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q; con cualquier otro valor el id_token_hint viaja "+
			"en el Referer a las once superficies del barrido", got)
	}
}

// El <script> de barrido va inline y la CSP del IdP no permite 'unsafe-inline':
// si su hash no está en script-src, el navegador lo bloquea y el cierre se queda
// colgado en la pantalla intermedia. Se calculan de la misma constante, así que
// esto solo puede fallar si alguien añade el script a la página y no a la CSP.
func TestElScriptDeBarridoEstaEnLaCSP(t *testing.T) {
	sum := sha256.Sum256([]byte(logoutSweepScript))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(cspScriptHashes, hash) {
		t.Errorf("falta el hash del script de barrido en script-src: %s", cspScriptHashes)
	}
}

// Cerrar sesión sin un destino registrado NO puede acabar en JSON: quien pulsa
// «Cerrar sesión» en /account es una persona con un navegador, y ese enlace no
// lleva client_id —no hay ninguno: se cierra la sesión del propio IdP—. Antes
// veía {"status":"logged_out"} en crudo y creía que algo había fallado.
func TestPostLogoutPorDefectoEsElSitioPublico(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://localhost/id")
	// El dominio de la suite se deriva del issuer, como en producción.
	t.Setenv("SYTUE_ID_ISSUER", "https://id.sytue.eu")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostLogoutDefaultURI != "https://www.sytue.eu/" {
		t.Errorf("destino por defecto = %q, se esperaba el sitio público", cfg.PostLogoutDefaultURI)
	}
	// Nunca un host ajeno: el destino es configuración del servicio, no un
	// parámetro de la petición (ahí estaría el open-redirect).
	if !strings.HasSuffix(cfg.PostLogoutDefaultURI, ".sytue.eu/") {
		t.Errorf("el destino por defecto se ha ido fuera del dominio: %q", cfg.PostLogoutDefaultURI)
	}
}

// Un valor explícito manda sobre el derivado (despliegues con otro dominio
// público o una landing distinta).
func TestPostLogoutExplicitoManda(t *testing.T) {
	t.Setenv("SYTUE_ID_DATABASE_DSN", "postgres://localhost/id")
	t.Setenv("SYTUE_ID_POST_LOGOUT_DEFAULT_URI", "https://www.ejemplo.test/adios")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostLogoutDefaultURI != "https://www.ejemplo.test/adios" {
		t.Errorf("no se respetó el destino explícito: %q", cfg.PostLogoutDefaultURI)
	}
}
