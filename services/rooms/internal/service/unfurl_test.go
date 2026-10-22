// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchUnfurlRejectsLoopback: el fetch a un servidor de loopback (127.0.0.1) se
// corta en el dialer SSRF (barrera 2) SIN entregar la petición. Reusa exactamente la
// misma barrera que los webhooks salientes (ssrfSafeTransport), aquí vía unfurlClient.
func TestFetchUnfurlRejectsLoopback(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="secreto"></head></html>`))
	}))
	defer srv.Close()

	svc := &Service{unfurl: unfurlClient()} // dialer SSRF real
	u := svc.fetchUnfurl(context.Background(), srv.URL)
	if u.HasContent() {
		t.Fatalf("un destino loopback NO debe producir tarjeta: %+v", u)
	}
	if hits != 0 {
		t.Fatalf("el servidor de loopback NO debió recibir la petición (hits=%d)", hits)
	}
}

// TestValidateUnfurlURLRejectsPrivate: la barrera 1 (validateOutgoingURL, reusada por
// Unfurl) rechaza literales privados/loopback y esquemas no http antes de conectar.
func TestValidateUnfurlURLRejectsPrivate(t *testing.T) {
	ctx := context.Background()
	for _, u := range []string{
		"http://127.0.0.1/x",
		"http://169.254.169.254/latest/meta-data/", // metadata SSRF
		"http://[::1]/x",
		"http://10.0.0.1/x",
		"file:///etc/passwd",
	} {
		if err := validateOutgoingURL(ctx, u); err == nil {
			t.Fatalf("validateOutgoingURL(%q) debe RECHAZAR (SSRF)", u)
		}
	}
}

// TestFetchUnfurlParsesPublicShaped ejercita el camino feliz: forzando el transporte al
// servidor de test (evita la barrera del dialer, cuyo destino real es loopback), valida
// que un HTML con OG se parsea y que el og:image relativo/no-http se descarta.
func TestFetchUnfurlParsesPublicShaped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head>
			<meta property="og:title" content="Hola mundo">
			<meta property="og:description" content="una desc">
			<meta property="og:image" content="javascript:alert(1)">
		</head></html>`))
	}))
	defer srv.Close()

	svc := &Service{unfurl: srv.Client()} // sin barrera de dialer (aísla parse/shape)
	u := svc.fetchUnfurl(context.Background(), srv.URL)
	if u.Title != "Hola mundo" || u.Description != "una desc" {
		t.Fatalf("OG mal parseado: %+v", u)
	}
	if u.ImageURL != "" {
		t.Fatalf("og:image con esquema no http debe descartarse, got %q", u.ImageURL)
	}
	if u.URL != srv.URL {
		t.Fatalf("URL no fijada: %q", u.URL)
	}
}

// TestFetchUnfurlIgnoresNonHTML: un Content-Type no HTML (p. ej. una imagen) no se
// parsea — nunca descargamos ni interpretamos binarios ajenos.
func TestFetchUnfurlIgnoresNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG..."))
	}))
	defer srv.Close()

	svc := &Service{unfurl: srv.Client()}
	if u := svc.fetchUnfurl(context.Background(), srv.URL); u.HasContent() {
		t.Fatalf("un recurso no-HTML no debe producir tarjeta: %+v", u)
	}
}

func TestSafeImageURL(t *testing.T) {
	ok := []string{"https://cdn.x.eu/a.png", "http://x.eu/b.jpg"}
	for _, s := range ok {
		if safeImageURL(s) != s {
			t.Fatalf("safeImageURL(%q) debe aceptarse", s)
		}
	}
	bad := []string{"", "javascript:alert(1)", "data:image/png;base64,AAAA", "/relativa.png", "ftp://x/y"}
	for _, s := range bad {
		if got := safeImageURL(s); got != "" {
			t.Fatalf("safeImageURL(%q)=%q, debe descartarse", s, got)
		}
	}
}
