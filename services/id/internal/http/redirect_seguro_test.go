// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRutaInternaSeguraRechazaLaBarraInvertida es el caso que se colaba: la
// comprobación era «empieza por / y no por //», y `/\evil.com` cumple las dos.
// Para el navegador no es una ruta —en la posición de la autoridad, `\` vale
// como `/` en http/https—, así que el IdP mandaba a `https://evil.com` desde su
// propio dominio.
func TestRutaInternaSeguraRechazaLaBarraInvertida(t *testing.T) {
	malas := []string{
		`/\evil.com`,
		`/\/evil.com`,
		`//evil.com`,
		`\\evil.com`,
		"https://evil.com",
		"//evil.com/path",
		"/cuenta\r\nSet-Cookie: a=b", // los controles se eliminan antes de interpretar
		"/cuenta\tx",
		"/\tevil.com",
		"",
		"cuenta",
	}
	for _, p := range malas {
		if rutaInternaSegura(p) {
			t.Errorf("rutaInternaSegura(%q) = true, es un destino externo o manipulable", p)
		}
		if got := safeInternalPath(p); got != "" {
			t.Errorf("safeInternalPath(%q) = %q, quiero \"\"", p, got)
		}
	}
	buenas := []string{"/", "/account", "/oauth2/authorize?client_id=x&state=y", "/a/b/c#frag"}
	for _, p := range buenas {
		if !rutaInternaSegura(p) {
			t.Errorf("rutaInternaSegura(%q) = false, es una ruta interna legítima", p)
		}
	}
}

// TestRedirectAfterLoginNoSaleDelIdP comprueba la regla donde de verdad se
// aplica: el Location que sale por el cable tras un login correcto.
func TestRedirectAfterLoginNoSaleDelIdP(t *testing.T) {
	for _, destino := range []string{`/\evil.com`, "//evil.com", "https://evil.com"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/login/", http.NoBody)
		redirectAfterLogin(rec, r, destino)
		if got := rec.Header().Get("Location"); got != "/" {
			t.Errorf("return_to=%q ⇒ Location=%q, quiero \"/\"", destino, got)
		}
	}
	rec := httptest.NewRecorder()
	redirectAfterLogin(rec, httptest.NewRequest(http.MethodPost, "/login/", http.NoBody), "/account")
	if got := rec.Header().Get("Location"); got != "/account" {
		t.Errorf("un return_to interno debe conservarse: Location=%q", got)
	}
}
