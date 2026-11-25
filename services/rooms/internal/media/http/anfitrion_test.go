// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

// porSala marca como de otra organización la petición que lleva a la anfitriona: es
// lo que abre la sala de espera de la reunión de un evento (salaDeEsperaDeFuera). La
// que se queda en su organización no lleva la marca.
func TestPorSalaMarcaAQuienVieneDeFuera(t *testing.T) {
	for _, c := range []struct {
		anfitriona string
		deFuera    bool
	}{{"acme", true}, {"", false}} {
		cambiar := func(r *http.Request, _, _ uuid.UUID) (*http.Request, error) {
			if c.anfitriona == "" {
				return r, nil
			}
			return r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: c.anfitriona})), nil
		}
		var marcada, visto bool
		r := chi.NewRouter()
		r.With(porSala(cambiar)).Get("/rooms/{id}", func(_ http.ResponseWriter, r *http.Request) {
			visto, marcada = true, service.EsDeOtraOrganizacion(r.Context())
		})
		req := httptest.NewRequest(http.MethodGet, "/rooms/"+uuid.NewString(), http.NoBody)
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "otra"}))
		r.ServeHTTP(httptest.NewRecorder(), req)
		if !visto || marcada != c.deFuera {
			t.Fatalf("anfitriona %q: llegó=%v, de fuera=%v; quiero %v", c.anfitriona, visto, marcada, c.deFuera)
		}
	}
}
