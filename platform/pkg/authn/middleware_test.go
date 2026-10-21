// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi", true},
		{"bearer abc", "abc", true},
		{"Basic xyz", "", false},
		{"", "", false},
		// "Bearer " sin token es un token vacío: deny-by-default (§0.7). Debe
		// tratarse como "sin bearer" (ok=false) para que RequireAuth responda 401
		// en lugar de intentar verificar una cadena vacía.
		{"Bearer ", "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		got, ok := bearerToken(r)
		if ok != c.ok || got != c.want {
			t.Errorf("header %q -> (%q,%v), want (%q,%v)", c.header, got, ok, c.want, c.ok)
		}
	}
}

func TestBearerTokenSSEQuery(t *testing.T) {
	// SSE (Accept: text/event-stream) sin cabecera: se acepta el token en query.
	r := httptest.NewRequest(http.MethodGet, "/stream?access_token=tok123", http.NoBody)
	r.Header.Set("Accept", "text/event-stream")
	if got, ok := bearerToken(r); !ok || got != "tok123" {
		t.Errorf("SSE query -> (%q,%v), want (tok123,true)", got, ok)
	}

	// Petición normal (no SSE): el token en query se IGNORA (deny-by-default).
	r = httptest.NewRequest(http.MethodGet, "/n?access_token=tok123", http.NoBody)
	r.Header.Set("Accept", "application/json")
	if got, ok := bearerToken(r); ok || got != "" {
		t.Errorf("query no-SSE -> (%q,%v), want (\"\",false)", got, ok)
	}

	// La cabecera Authorization tiene prioridad sobre la query.
	r = httptest.NewRequest(http.MethodGet, "/stream?access_token=fromquery", http.NoBody)
	r.Header.Set("Accept", "text/event-stream")
	r.Header.Set("Authorization", "Bearer fromheader")
	if got, _ := bearerToken(r); got != "fromheader" {
		t.Errorf("prioridad cabecera -> %q, want fromheader", got)
	}
}

func TestAnyAudienceMatches(t *testing.T) {
	allow := []string{"sytue-one", "sytue-mail", "sytue-hub"}
	if !anyAudienceMatches([]string{"https://id.sytue.localhost", "sytue-mail"}, allow) {
		t.Error("token con audiencia de la suite debe aceptarse")
	}
	if anyAudienceMatches([]string{"sytue-evil", "otra"}, allow) {
		t.Error("token sin audiencia en la allowlist debe rechazarse")
	}
	if anyAudienceMatches(nil, allow) {
		t.Error("token sin audiencias debe rechazarse")
	}
}

// El handshake WebSocket acepta el token en query por la misma razón que SSE: la API
// WebSocket del navegador no deja fijar cabeceras. Sin esta excepción, el editor de
// Docs recibía 401 en el upgrade y se quedaba en su sondeo de 4 s — co-edición «en
// vivo» que en realidad llegaba con cuatro segundos de retraso.
//
// La excepción se acota a los DOS casos, así que el test vigila las dos mitades: que el
// upgrade autentique y que una petición corriente con el mismo token en la URL NO lo
// haga. Un token en la query se queda en logs, historiales y referrers; ampliar esto sin
// querer es regalar sesiones.
func TestBearerTokenWebSocketUpgrade(t *testing.T) {
	upgrade := func(target string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, target, http.NoBody)
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Connection", "Upgrade")
		return r
	}

	if got, ok := bearerToken(upgrade("/ws?access_token=tok123")); !ok || got != "tok123" {
		t.Errorf("upgrade WS -> (%q,%v), want (tok123,true)", got, ok)
	}

	// Chrome manda «Connection: keep-alive, Upgrade»: la detección compara por
	// contenido, no por igualdad, o el navegador mayoritario se quedaría fuera.
	r := upgrade("/ws?access_token=tok123")
	r.Header.Set("Connection", "keep-alive, Upgrade")
	if got, ok := bearerToken(r); !ok || got != "tok123" {
		t.Errorf("Connection compuesto -> (%q,%v), want (tok123,true)", got, ok)
	}

	// Sin las cabeceras del upgrade NO es un handshake: el token de la query se ignora.
	plain := httptest.NewRequest(http.MethodGet, "/ws?access_token=tok123", http.NoBody)
	if got, ok := bearerToken(plain); ok || got != "" {
		t.Errorf("sin upgrade -> (%q,%v), want (\"\",false)", got, ok)
	}

	// `Upgrade: websocket` sin `Connection: Upgrade` tampoco: media cabecera la pone
	// cualquiera, y aceptarla abriría la excepción a peticiones normales.
	medio := httptest.NewRequest(http.MethodGet, "/ws?access_token=tok123", http.NoBody)
	medio.Header.Set("Upgrade", "websocket")
	if got, ok := bearerToken(medio); ok || got != "" {
		t.Errorf("upgrade a medias -> (%q,%v), want (\"\",false)", got, ok)
	}
}
