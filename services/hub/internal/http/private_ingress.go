// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"errors"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// mountHubPrivateIngress monta la entrada privada de avisos (ADR 0086): las mismas
// rutas compartidas que Calendar y Mail (/private-storage/ingress/*), con la
// identidad de la sesión, sin delegación.
func mountHubPrivateIngress(api chi.Router, port httpx.PrivateIngressPort) {
	handlers := httpx.NewPrivateIngressHandlers(httpx.PrivateIngressHTTPConfig{
		Product:  "hub",
		Port:     port,
		MapError: hubIngressError,
		// Un aviso cifrado cabe holgado: título, cuerpo y parámetros, y el sobre.
		MaxBodyBytes: 1 << 20,
		Identity: func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
			claims, ok := authn.FromContext(ctx)
			return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
		},
	})
	handlers.Mount(api)
}

func hubIngressError(err error) *httpx.Error {
	if shared := httpx.PrivateIngressError(err); shared != nil {
		return shared
	}
	if errors.Is(err, store.ErrEntradaPrivada) || errors.Is(err, callaccess.ErrUnavailable) {
		return httpx.ErrUnavailable.WithCode("private_ingress_required")
	}
	return hubSourceError(err)
}
