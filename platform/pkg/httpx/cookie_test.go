// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSetSecureCookieDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSecureCookie(rec, CookieOptions{Name: "sytue_session", Value: "v", MaxAge: time.Hour, Domain: ".sytue.eu"})
	cs := rec.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies=%d", len(cs))
	}
	c := cs[0]
	if !c.HttpOnly {
		t.Error("debe ser HttpOnly por defecto")
	}
	if !c.Secure {
		t.Error("debe ser Secure por defecto")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Error("debe ser SameSite=Strict por defecto")
	}
	if c.Path != "/" {
		t.Errorf("path incorrecto: %q", c.Path)
	}
	// Intent: ámbito de dominio para SSO cross-subdominio (la cookie debe valer en
	// *.sytue.eu, no host-only). Pasamos Domain=".sytue.eu", pero net/http elimina
	// el punto inicial al serializar (Cookie.String: "A leading dot is okay but
	// won't be sent") y al parsear (RFC 6265 §4.1.2.3 / §5.2.3: el punto inicial es
	// obsoleto y se ignora). El resultado correcto en la cabecera CRUDA es
	// "Domain=sytue.eu", que por RFC 6265 abarca sytue.eu y TODOS sus subdominios,
	// satisfaciendo el SSO cross-subdominio. Verificamos ese comportamiento real:
	// que se emite un atributo Domain con ámbito de dominio (no host-only).
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "Domain=sytue.eu") {
		t.Errorf("Set-Cookie sin ámbito de dominio sytue.eu (SSO cross-subdominio): %q", setCookie)
	}
	if c.Domain != "sytue.eu" {
		t.Errorf("Domain parseado inesperado (esperado ámbito de dominio sytue.eu): %q", c.Domain)
	}
}

func TestClearCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearCookie(rec, "sytue_session", "/", ".sytue.eu")
	c := rec.Result().Cookies()[0]
	if c.MaxAge >= 0 {
		t.Fatalf("MaxAge debe ser negativo para borrar, got %d", c.MaxAge)
	}
}
