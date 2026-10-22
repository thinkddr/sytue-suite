// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Tras EnableWebDAVMethods, un PROPFIND montado con r.Handle debe enrutar al
// handler (no 405). Sin el registro, chi lo rechazaría con MethodNotAllowed.
func TestEnableWebDAVMethodsRoutesPropfind(t *testing.T) {
	EnableWebDAVMethods()
	r := NewRouter(RouterOptions{})
	r.Handle("/dav/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("PROPFIND", "/dav/calendars/u/", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PROPFIND -> %d, want 200 (¿verbo WebDAV no registrado en chi?)", rec.Code)
	}
}
