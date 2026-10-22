// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/calendar/internal/config"
)

// El well-known de CalDAV (RFC 6764) es lo que permite a Apple Calendario y a
// los clientes DAV encontrar el servidor escribiendo solo el dominio: debe
// redirigir (301) al raíz DAV del prefijo CONFIGURADO.
func TestWellKnownCalDAVRedirect(t *testing.T) {
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	tests := []struct {
		name     string
		prefix   string
		caldav   http.Handler
		wantCode int
		wantLoc  string
	}{
		{name: "prefijo por defecto", prefix: "/dav", caldav: stub, wantCode: http.StatusMovedPermanently, wantLoc: "/dav/"},
		{name: "prefijo configurado", prefix: "/caldav", caldav: stub, wantCode: http.StatusMovedPermanently, wantLoc: "/caldav/"},
		{name: "sin servidor CalDAV", prefix: "/dav", caldav: nil, wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(Deps{Cfg: config.Config{CalDAVPrefix: tt.prefix}, CalDAV: tt.caldav})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/caldav", http.NoBody))

			if rec.Code != tt.wantCode {
				t.Fatalf("código=%d, quería %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLoc {
				t.Fatalf("Location=%q, quería %q", got, tt.wantLoc)
			}
		})
	}
}
