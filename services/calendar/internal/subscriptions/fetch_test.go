// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestParseMaxAge cubre el parseo de Cache-Control: max-age.
func TestParseMaxAge(t *testing.T) {
	cases := map[string]time.Duration{
		"max-age=3600":          3600 * time.Second,
		"public, max-age=120":   120 * time.Second,
		"no-store":              0,
		"no-cache, max-age=600": 0, // no-cache invalida
		"":                      0,
		"max-age=abc":           0,
	}
	for cc, want := range cases {
		if got := parseMaxAge(cc); got != want {
			t.Errorf("parseMaxAge(%q) = %v, want %v", cc, got, want)
		}
	}
}

const sampleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a@x\r\n" +
	"DTSTART:20260101T090000Z\r\nDTEND:20260101T100000Z\r\nSUMMARY:Hola\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// TestDoFetchConditional304 verifica el GET condicional: un 200 con ETag y luego
// un 304 (If-None-Match) => NotModified, sin retraer el cuerpo (no cambia nada).
// doFetch se prueba sin la barrera SSRF (que bloquearía 127.0.0.1); la barrera se
// cubre aparte en ssrf_test.go.
func TestDoFetchConditional304(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Wed, 01 Jan 2026 00:00:00 GMT")
		w.Header().Set("Cache-Control", "max-age=1800")
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(sampleICS))
	}))
	defer srv.Close()
	ctx := context.Background()

	// Primera vez: 200 con cuerpo + validadores.
	fr, err := doFetch(ctx, srv.Client(), srv.URL, Conditional{}, 1<<20)
	if err != nil {
		t.Fatalf("doFetch inicial: %v", err)
	}
	if fr.NotModified || fr.Body != sampleICS || fr.ETag != `"v1"` {
		t.Fatalf("fetch inicial inesperado: notmod=%v etag=%q body=%d bytes", fr.NotModified, fr.ETag, len(fr.Body))
	}
	if fr.MaxAge != 1800*time.Second {
		t.Errorf("MaxAge = %v, want 1800s", fr.MaxAge)
	}

	// Segunda vez con el ETag: 304 => NotModified, cuerpo vacío (no se retrae nada).
	fr2, err := doFetch(ctx, srv.Client(), srv.URL, Conditional{ETag: fr.ETag}, 1<<20)
	if err != nil {
		t.Fatalf("doFetch condicional: %v", err)
	}
	if !fr2.NotModified || fr2.Body != "" {
		t.Fatalf("condicional 304 no respetado: notmod=%v body=%d bytes", fr2.NotModified, len(fr2.Body))
	}
}

// TestDoFetchCapAndErrors verifica el tope de bytes y el mapeo de estados no-2xx.
func TestDoFetchCapAndErrors(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer big.Close()
	if _, err := doFetch(context.Background(), big.Client(), big.URL, Conditional{}, 1024); err == nil {
		t.Error("doFetch con cuerpo > tope = nil, want error de tamaño")
	}

	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFound.Close()
	if _, err := doFetch(context.Background(), notFound.Client(), notFound.URL, Conditional{}, 1<<20); err == nil {
		t.Error("doFetch con 404 = nil, want error")
	}
}
