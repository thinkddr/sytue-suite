// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// El mismo handler, los dos métodos y la MISMA respuesta: es lo que evita que la
// ruta nueva y la vieja se separen con el tiempo.
func TestGetYQueryDanLoMismo(t *testing.T) {
	r := chi.NewRouter()
	httpx.GetYQuery(r, "/buscar", func(w http.ResponseWriter, r *http.Request) {
		p, err := httpx.Consulta(w, r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		_, _ = w.Write([]byte(p.Get("q") + "|" + strings.Join(p["id"], ",")))
	})

	porGet := httptest.NewRecorder()
	r.ServeHTTP(porGet, httptest.NewRequest(http.MethodGet, "/buscar?q=playa&id=a&id=b", http.NoBody))

	req := httptest.NewRequest(httpx.MethodQuery, "/buscar", strings.NewReader("q=playa&id=a&id=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	porQuery := httptest.NewRecorder()
	r.ServeHTTP(porQuery, req)

	if porGet.Code != http.StatusOK || porQuery.Code != http.StatusOK {
		t.Fatalf("GET=%d QUERY=%d, se esperaban 200 (¿QUERY sin registrar en chi?)", porGet.Code, porQuery.Code)
	}
	if porGet.Body.String() != porQuery.Body.String() {
		t.Fatalf("distinta respuesta: GET=%q QUERY=%q", porGet.Body.String(), porQuery.Body.String())
	}
	if porGet.Body.String() != "playa|a,b" {
		t.Fatalf("cuerpo = %q", porGet.Body.String())
	}
}

// Con QUERY manda el CUERPO. Aceptar además la query string obligaría a decidir
// cuál gana cuando se contradicen, y esa decisión no la conoce quien llama.
func TestQueryIgnoraLaQueryStringDeLaURL(t *testing.T) {
	r := chi.NewRouter()
	httpx.GetYQuery(r, "/buscar", func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpx.Consulta(w, r)
		_, _ = w.Write([]byte(p.Get("q")))
	})
	req := httptest.NewRequest(httpx.MethodQuery, "/buscar?q=de-la-url", strings.NewReader("q=del-cuerpo"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Body.String() != "del-cuerpo" {
		t.Fatalf("manda el cuerpo, y salió %q", rec.Body.String())
	}
}

// Un cuerpo en otro formato no se adivina: 415, que es lo que dice el RFC.
func TestQueryRechazaOtroContentType(t *testing.T) {
	r := chi.NewRouter()
	httpx.GetYQuery(r, "/buscar", func(w http.ResponseWriter, r *http.Request) {
		if _, err := httpx.Consulta(w, r); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(httpx.MethodQuery, "/buscar", strings.NewReader(`{"q":"playa"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("Content-Type ajeno -> %d, se esperaba 415", rec.Code)
	}
}

// El POST de búsqueda: mismo handler, y el cuerpo JSON se lee igual porque
// DecodeJSON no mira el método.
func TestPostYQueryComparteCuerpo(t *testing.T) {
	r := chi.NewRouter()
	httpx.PostYQuery(r, "/ediscovery", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Subject string `json:"subject"`
		}
		if err := httpx.DecodeJSON(w, r, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		_, _ = w.Write([]byte(body.Subject))
	})
	for _, metodo := range []string{http.MethodPost, httpx.MethodQuery} {
		req := httptest.NewRequest(metodo, "/ediscovery", strings.NewReader(`{"subject":"u-1"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "u-1" {
			t.Fatalf("%s -> %d %q", metodo, rec.Code, rec.Body.String())
		}
	}
}

// `Accept-Query` es como un cliente DESCUBRE que hay QUERY sin gastar un viaje
// en probarlo y comerse un 405.
func TestAnunciarQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.AnunciarQuery(rec)
	if got := rec.Header().Get("Accept-Query"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Accept-Query = %q", got)
	}
	rec2 := httptest.NewRecorder()
	httpx.AnunciarQuery(rec2, "application/json")
	if got := rec2.Header().Get("Accept-Query"); got != "application/json" {
		t.Fatalf("Accept-Query con tipo propio = %q", got)
	}
}
