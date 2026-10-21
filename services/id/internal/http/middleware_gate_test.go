// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"testing"
)

// Qué rutas de /api/v1 quedan reservadas a una credencial pedida por un client de
// la suite, y cuáles siguen abiertas a una app de terceros.
func TestRutaSoloDeLaSuite(t *testing.T) {
	casos := []struct {
		metodo, ruta string
		solo         bool
	}{
		// Administración: lo que ya cubría el gate.
		{http.MethodGet, "/api/v1/admin/users", true},
		{http.MethodGet, "/api/v1/platform/activation", true},
		// Administración sin `/admin/` en la ruta.
		{http.MethodPost, "/api/v1/orgs", true},
		{http.MethodPost, "/api/v1/orgs/acme/members", true},
		{http.MethodGet, "/api/v1/onboarding", true},
		{http.MethodPost, "/api/v1/onboarding/organization", true},
		// Cómo se entra en la cuenta, sus sesiones y sus datos.
		{http.MethodPost, "/api/v1/me/password", true},
		{http.MethodPost, "/api/v1/me/app-passwords", true},
		{http.MethodGet, "/api/v1/me/app-passwords", true},
		{http.MethodDelete, "/api/v1/me/mfa/totp", true},
		{http.MethodGet, "/api/v1/me/sessions", true},
		{http.MethodPut, "/api/v1/me/active-organization", true},
		{http.MethodPost, "/api/v1/me/pim/activate", true},
		{http.MethodGet, "/api/v1/me/export", true},
		{http.MethodPost, "/api/v1/webauthn/register/begin", true},
		{http.MethodDelete, "/api/v1/webauthn/credentials/x", true},
		// El perfil se lee, no se edita.
		{http.MethodGet, "/api/v1/me", false},
		{http.MethodPatch, "/api/v1/me", true},
		// Lo que los productos reenvían con el token de quien llama.
		{http.MethodPost, "/api/v1/directory:resolve", false},
		{http.MethodGet, "/api/v1/directory", false},
		{http.MethodPost, "/api/v1/people:resolve", false},
		{http.MethodGet, "/api/v1/sensitivity-labels", false},
		// Un nombre que solo EMPIEZA igual no es la ruta.
		{http.MethodGet, "/api/v1/mensajes", false},
		{http.MethodGet, "/api/v1/orgsx", false},
	}
	for _, c := range casos {
		if got := rutaSoloDeLaSuite(c.metodo, c.ruta); got != c.solo {
			t.Errorf("rutaSoloDeLaSuite(%s %s) = %v, quiero %v", c.metodo, c.ruta, got, c.solo)
		}
	}
}
