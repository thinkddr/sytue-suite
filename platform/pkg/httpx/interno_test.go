// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecretoInterno(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, c := range []struct {
		nombre, secreto, cabecera string
		quiero                    int
	}{
		{"el bueno", "s3cr3t", "Bearer s3cr3t", http.StatusNoContent},
		{"otro", "s3cr3t", "Bearer otro", http.StatusUnauthorized},
		{"sin cabecera", "s3cr3t", "", http.StatusUnauthorized},
		{"sin Bearer", "s3cr3t", "s3cr3t", http.StatusUnauthorized},
		{"sin secreto configurado", "", "Bearer ", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest(http.MethodGet, "/internal/x", http.NoBody)
		if c.cabecera != "" {
			r.Header.Set("Authorization", c.cabecera)
		}
		w := httptest.NewRecorder()
		SecretoInterno(c.secreto)(ok).ServeHTTP(w, r)
		if w.Code != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, w.Code, c.quiero)
		}
	}
}

// Desde fuera (X-Forwarded-For) la ruta no existe, con el secreto bueno o sin él: un
// 401 le confirmaría a quien prueba que está ahí.
func TestSoloDesdeDentro(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, c := range []struct {
		nombre, auth, xff string
		quiero            int
	}{
		{"dentro con secreto", "Bearer s", "", http.StatusNoContent},
		{"dentro sin secreto", "Bearer x", "", http.StatusUnauthorized},
		{"fuera con secreto", "Bearer s", "203.0.113.7", http.StatusNotFound},
		{"fuera sin secreto", "Bearer x", "203.0.113.7", http.StatusNotFound},
	} {
		r := httptest.NewRequest(http.MethodPost, "/internal/x", http.NoBody)
		r.Header.Set("Authorization", c.auth)
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		w := httptest.NewRecorder()
		SoloDesdeDentro("s")(ok).ServeHTTP(w, r)
		if w.Code != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, w.Code, c.quiero)
		}
	}
}

// La cabecera propia no sustituye a la sesión: se exige además. Vacía, otra, o el
// secreto sin configurar = 403, y el nombre de la cabecera es el que se pide.
func TestSecretoEnCabecera(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, c := range []struct {
		nombre, secreto, cabecera, valor string
		quiero                           int
	}{
		{"el bueno", "s3cr3t", "X-Sytue-Publicador", "s3cr3t", http.StatusNoContent},
		{"otro", "s3cr3t", "X-Sytue-Publicador", "otro", http.StatusForbidden},
		{"sin cabecera", "s3cr3t", "", "", http.StatusForbidden},
		{"en otra cabecera", "s3cr3t", "Authorization", "s3cr3t", http.StatusForbidden},
		{"sin secreto configurado", "", "X-Sytue-Publicador", "", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/x", http.NoBody)
		if c.cabecera != "" {
			r.Header.Set(c.cabecera, c.valor)
		}
		w := httptest.NewRecorder()
		SecretoEnCabecera("X-Sytue-Publicador", c.secreto)(ok).ServeHTTP(w, r)
		if w.Code != c.quiero {
			t.Errorf("%s: %d, quiero %d", c.nombre, w.Code, c.quiero)
		}
	}
}
