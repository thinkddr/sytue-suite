// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// El canje: sin cabecera, reto Basic; con la contraseña buena, el Bearer de Sytue ID
// llega al siguiente y la segunda petición no vuelve a preguntar; con la mala, reto.
func TestAppPasswordsBasic(t *testing.T) {
	var canjes atomic.Int32
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/app-passwords/token" || r.Header.Get("Authorization") != "Bearer s3" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		var c struct{ Identity, Password string }
		_ = json.NewDecoder(r.Body).Decode(&c)
		if c.Password != "buena" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		canjes.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-" + c.Identity, "expires_in": 900})
	}))
	defer id.Close()

	if NewAppPasswords(id.URL, "") != nil {
		t.Fatal("sin secreto no se canjea")
	}
	a := NewAppPasswords(id.URL, "s3")
	var visto string
	h := a.Basic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = r.Header.Get("Authorization")
	}))
	pedir := func(usuario, clave string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PROPFIND", "/dav/", http.NoBody)
		if usuario != "" {
			r.SetBasicAuth(usuario, clave)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if w := pedir("", ""); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("sin credenciales: %d %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
	for range 2 {
		if w := pedir("ana@x.eu", "buena"); w.Code != http.StatusOK || visto != "Bearer tok-ana@x.eu" {
			t.Fatalf("buena: %d %q", w.Code, visto)
		}
	}
	if canjes.Load() != 1 {
		t.Fatalf("canjes = %d, la segunda tenía que salir de la caché", canjes.Load())
	}
	if w := pedir("ana@x.eu", "mala"); w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("mala: %d", w.Code)
	}

	// Un Bearer pasa sin tocarse.
	r := httptest.NewRequest("PROPFIND", "/dav/", http.NoBody)
	r.Header.Set("Authorization", "Bearer propio")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if visto != "Bearer propio" {
		t.Fatalf("bearer: %q", visto)
	}
}
