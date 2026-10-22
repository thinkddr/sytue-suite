// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// Una respuesta de /_pages va al dominio de un CLIENTE: HSTS sin subdominios ni
// preload y sin COEP. El resto de la API conserva las cabeceras de fábrica.
func TestCabecerasDeLasWebs(t *testing.T) {
	r, g := construir(Deps{Store: store.New(nil)})
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	g.publico.Get("/prueba", ok)
	r.Get("/prueba", ok)

	pedir := func(ruta string) http.Header {
		req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		req.TLS = &tls.ConnectionState{}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d", ruta, w.Code)
		}
		return w.Header()
	}
	web := pedir("/_pages/prueba")
	if got := web.Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HSTS de una web = %q", got)
	}
	if got := web.Get("Cross-Origin-Embedder-Policy"); got != "" {
		t.Errorf("COEP en una web = %q", got)
	}
	api := pedir("/prueba")
	if api.Get("Cross-Origin-Embedder-Policy") == "" || api.Get("Strict-Transport-Security") == "max-age=31536000" {
		t.Errorf("fuera de /_pages cambian las cabeceras de fábrica: %v", api)
	}
}

// El plazo largo amplía también el de escritura de la conexión: sin eso, una
// publicación o un zip se cortaría en el WriteTimeout del servidor.
func TestConPlazoAmpliaLaConexion(t *testing.T) {
	r := httpx.NewRouter(httpx.RouterOptions{})
	r.With(conPlazo(5*time.Second)).Get("/lenta", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = io.WriteString(w, "hecho")
	})
	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/lenta", http.NoBody)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("la conexión se cortó en el WriteTimeout: %v", err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); string(b) != "hecho" {
		t.Fatalf("cuerpo %q", b)
	}
}

func TestClaveDeWebhook(t *testing.T) {
	pedir := func(ruta, xff string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, ruta, http.NoBody)
		req.RemoteAddr = "10.42.0.7:41000" // Traefik, dentro del clúster
		req.Header.Set("X-Forwarded-For", xff)
		return req
	}
	// Por el túnel del piloto (sin la IP de quien llama): un cubo por proveedor.
	for ruta, quiero := range map[string]string{
		"/webhooks/mollie/0b0c": "webhook:mollie",
		"/webhooks/stripe/x":    "webhook:stripe",
		"/webhooks/inventado/x": "webhook:otro",
		"/webhooks/":            "webhook:otro",
	} {
		if got := claveDeWebhook(pedir(ruta, "127.0.0.1")); got != quiero {
			t.Errorf("%s: %s, quiero %s", ruta, got, quiero)
		}
	}
	// WEB-11. Con la IP real, cada IP tiene el suyo: quien inunda no gasta el de la pasarela.
	if got := claveDeWebhook(pedir("/webhooks/mollie/0b0c", "203.0.113.9")); got != "webhook:mollie:203.0.113.9" {
		t.Errorf("con IP real: %s", got)
	}
	if got := claveDeWebhook(pedir("/webhooks/inventado/x", "203.0.113.9")); got != "webhook:otro" {
		t.Errorf("proveedor inventado con IP real: %s", got)
	}
}

// El router completo se construye con los montajes vacíos del esqueleto y sirve
// las sondas; /api/v1 no existe hasta que un área registre algo.
func TestRouterDelEsqueleto(t *testing.T) {
	r := NewRouter(Deps{Store: store.New(nil)})
	if _, ok := r.(chi.Routes); !ok {
		t.Fatal("el router no es de chi: ingressroutes no podría recorrerlo")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if w.Code != http.StatusOK {
		t.Fatalf("/healthz: %d", w.Code)
	}
}
