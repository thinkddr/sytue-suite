// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/cast/internal/diapositivas"
)

func conID(r *http.Request) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", "7f0c2f4e-8f3a-4f5e-9d3b-0a1b2c3d4e5f")
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

// Sin el servicio enchufado, avanzar es 503 y mirar dice «sin presentación».
func TestDiapositivasSinServicio(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.AvanzarDiapositiva(rec, conID(httptest.NewRequest(http.MethodPut, "/api/v1/lives/x/slide", strings.NewReader(`{"index":0,"total":1}`))))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT sin servicio = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.DiapositivasPublico(rec, conID(httptest.NewRequest(http.MethodGet, "/public/videos/x/slides", http.NoBody)))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"presentation":null}` {
		t.Errorf("GET sin servicio = %d %s", rec.Code, rec.Body)
	}
}

// El cuerpo solo lleva la posición: unas notas, o cualquier otro campo, se rechazan en la
// puerta y no llegan a ningún sitio. (El servicio es uno vacío: la petición no pasa del cuerpo.)
func TestAvanzarRechazaNotasYCamposAjenos(t *testing.T) {
	h := &Handlers{diapositivas: diapositivas.New(nil, nil, nil)}
	for _, cuerpo := range []string{
		`{"index":0,"total":3,"notes":"decir que el pozo cuesta 40.000"}`,
		`{"index":0,"total":3,"html":"<p>x</p>"}`,
		`{"index":0,"total":3} {"index":1}`,
		``,
	} {
		rec := httptest.NewRecorder()
		h.AvanzarDiapositiva(rec, conID(httptest.NewRequest(http.MethodPut, "/api/v1/lives/x/slide", strings.NewReader(cuerpo))))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q = %d, se esperaba un rechazo", cuerpo, rec.Code)
		}
	}
}
