// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// authenticateClient autentica el client del token endpoint por client_secret
// (Basic o post) o lo reconoce como público (PKCE puro, token_endpoint_auth_method
// = none). Devuelve el client y si la autenticación fue válida.
func (s *Server) authenticateClient(r *http.Request) (domain.Client, bool) {
	var id, secret string
	if bid, bsecret, ok := r.BasicAuth(); ok {
		id, secret = bid, bsecret
	} else {
		id = r.Form.Get("client_id")
		secret = r.Form.Get("client_secret")
	}
	if id == "" {
		return domain.Client{}, false
	}
	client, err := s.store.GetClient(r.Context(), id)
	if err != nil {
		// Verificación señuelo para uniformar el tiempo de respuesta y no filtrar
		// por timing qué client_ids existen (auditoría F7).
		domain.VerifyDecoy(secret)
		return domain.Client{}, false
	}

	// Client público: se identifica por client_id, sin secreto. La seguridad
	// recae en PKCE + redirect_uri exacto.
	if client.IsPublic() {
		return client, true
	}

	// Confidencial: exige secreto y un hash configurado. Sin secret_hash el
	// client está "deshabilitado" hasta que el deploy fije su secreto.
	if client.SecretHash == "" || secret == "" {
		domain.VerifyDecoy(secret)
		return domain.Client{}, false
	}
	ok, err := domain.VerifyPassword(secret, client.SecretHash)
	if err != nil || !ok {
		return domain.Client{}, false
	}
	return client, true
}
