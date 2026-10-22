// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// El sondeo se prueba con un cliente normal a propósito: el de producción lleva el
// guard anti-SSRF, que RECHAZA 127.0.0.1 —que es exactamente lo que debe hacer— y
// dejaría el test comprobando el guard en vez de la traducción de respuestas.
func TestProbeOneTraduceLaRespuesta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/health":
			w.WriteHeader(http.StatusNoContent) // 204 también es sano
		case "/login":
			http.Redirect(w, r, "/", http.StatusFound)
		case "/roto":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cl := srv.Client()
	// Igual que el cliente de producción: NO se siguen redirecciones. Perseguir un
	// 3xx haría que se juzgara el destino y no la ruta que se está evaluando —y en
	// el prober de verdad es además lo que impide que un 3xx lleve a la red interna.
	cl.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx := context.Background()

	casos := []struct {
		path       string
		wantStatus int
		wantSano   bool
		porque     string
	}{
		{"/healthz", 200, true, "un 200 es la respuesta que se busca"},
		{"/health", 204, true, "204 sin cuerpo es una sonda perfectamente válida"},
		{"/login", 302, false, "una redirección al login es una app que arranca, no una app sana"},
		{"/roto", 500, false, "un 500 no puede pasar por sano"},
		{"/no-existe", 404, false, "la ruta no está: hay que verlo, no ocultarlo"},
	}
	for _, c := range casos {
		got := probeOne(ctx, cl, srv.URL+c.path, c.path)
		if got.Status != c.wantStatus || got.Healthy != c.wantSano {
			t.Errorf("%s -> status %d sano=%v; esperado %d/%v (%s)",
				c.path, got.Status, got.Healthy, c.wantStatus, c.wantSano, c.porque)
		}
		if got.Path != c.path {
			t.Errorf("el resultado debe conservar su ruta, trae %q", got.Path)
		}
	}
}

// Sin nadie al otro lado el estado queda en 0, que NO es un 404: la interfaz tiene
// que poder distinguir «tu app no tiene ruta de salud» de «tu app no está en pie».
func TestProbeOneSinRespuestaNoEsUn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cl := srv.Client()
	url := srv.URL + "/healthz"
	srv.Close() // se cierra ANTES de sondear: no hay nadie escuchando

	got := probeOne(context.Background(), cl, url, "/healthz")
	if got.Status != 0 || got.Healthy {
		t.Errorf("sin respuesta debe quedar status 0 y no sano, salió %d/%v", got.Status, got.Healthy)
	}
}
