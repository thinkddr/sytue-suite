// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodyBytes(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := MaxBodyBytes(10)(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 100))))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("cuerpo grande debe rechazarse, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("ok")))
	if rec.Code != http.StatusOK {
		t.Fatalf("cuerpo pequeño debe pasar, got %d", rec.Code)
	}
}

func TestCORSAllowlist(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://one.sytue.eu"}, AllowCredentials: true})(next)

	// Origen permitido.
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://one.sytue.eu")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://one.sytue.eu" {
		t.Fatalf("origen permitido no reflejado: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("faltan credenciales permitidas")
	}

	// Origen no permitido: sin cabecera.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("origen no permitido no debe recibir cabecera CORS")
	}
}

func TestCORSWildcardSubdomain(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://*.sytue.localhost", "https://*.sytue.eu"}, AllowCredentials: true})(next)

	// Cualquier subdominio de un apex permitido se refleja.
	for _, origin := range []string{"https://mail.sytue.localhost", "https://one.sytue.eu"} {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("subdominio %q no reflejado: %q", origin, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}

	// El dominio del TENANT, que es donde corre la suite tras el login. Sin esto,
	// las notificaciones, la presencia y el conmutador de aplicaciones fallaban
	// en silencio contra el Hub, y el fallo se leía como una avería del producto.
	for _, origin := range []string{"https://corporate.sytue.eu", "https://acme.sytue.eu"} {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("dominio de tenant %q no reflejado: %q", origin, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}

	// DOS ETIQUETAS NO. `algo.apps.sytue.eu` son las aplicaciones que los
	// clientes alojan en Sytue Cloud: código de terceros. Darles CORS con
	// credenciales contra nuestras API sería dejarles llamar con la sesión de
	// quien las visite. Es la razón por la que este comodín no se podía usar y
	// las allowlists se escribían a mano.
	for _, origin := range []string{
		"https://algo.apps.sytue.eu", "https://x.preview.sytue.eu", "https://a.b.sytue.localhost",
	} {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("subdominio de segundo nivel %q recibió CORS", origin)
		}
	}

	// Apex ajeno / esquema http / subdominio de otro apex / apex desnudo: rechazados.
	for _, origin := range []string{"https://sytue.evil.com", "http://mail.sytue.localhost", "https://mail.notsytue.eu", "https://sytue.eu"} {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("origen no permitido %q recibió cabecera CORS", origin)
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("no debe llegar al handler") })
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://one.sytue.eu"}})(next)
	req := httptest.NewRequest(http.MethodOptions, "/", http.NoBody)
	req.Header.Set("Origin", "https://one.sytue.eu")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("faltan métodos permitidos en preflight")
	}
}
