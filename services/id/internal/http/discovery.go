// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// handleDiscovery sirve /.well-known/openid-configuration (OIDC Discovery).
func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.JSON(w, r, http.StatusOK, oidc.BuildDiscovery(s.cfg.Issuer))
}

// handleJWKS sirve el conjunto de claves públicas (RFC 7517).
func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	set, err := s.keys.PublicJWKS()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.JSON(w, r, http.StatusOK, set)
}
