// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webdav

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Un HTML o un SVG subido a Drive no sale por WebDAV como página: el contenido va
// como descarga y en una CSP `sandbox`, lo sirva como lo sirva la biblioteca.
func TestElContenidoSalePorWebDAVComoDescarga(t *testing.T) {
	// Lo que haría la biblioteca con un `.html`: tipo por extensión y nada más.
	biblioteca := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<script>alert(1)</script>"))
	})
	h := comoDescarga(biblioteca)

	for _, metodo := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(metodo, "/dav/pagina.html", http.NoBody))
		if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
			t.Errorf("%s: Content-Disposition = %q, esperaba attachment", metodo, got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
			t.Errorf("%s: Content-Security-Policy = %q, esperaba sandbox", metodo, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", metodo, got)
		}
	}
	// El XML del protocolo no es una descarga.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("PROPFIND", "/dav/", http.NoBody))
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Errorf("PROPFIND lleva Content-Disposition = %q", got)
	}
}

// El proveedor de Android manda PROPFIND, MKCOL y MOVE como POST con
// X-HTTP-Method-Override; cualquier otra cosa en esa cabecera no cambia nada.
func TestMetodoDeVerdad(t *testing.T) {
	for _, c := range []struct{ metodo, cabecera, quiero string }{
		{http.MethodPost, "propfind", "PROPFIND"},
		{http.MethodPost, "MOVE", "MOVE"},
		{http.MethodPost, "DELETE", http.MethodPost},
		{http.MethodPost, "", http.MethodPost},
		{http.MethodGet, "PROPFIND", http.MethodGet},
	} {
		r := httptest.NewRequest(c.metodo, "/dav/", http.NoBody)
		r.Header.Set("X-HTTP-Method-Override", c.cabecera)
		metodoDeVerdad(r)
		if r.Method != c.quiero {
			t.Errorf("%s + %q = %s, quería %s", c.metodo, c.cabecera, r.Method, c.quiero)
		}
	}
}
