// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
)

// TestIngressPublishesEveryMountedPrefix: todo prefijo que monta el gateway de integraciones tiene que estar
// en el Ingress de su chart, o en producción se lo queda el SPA aunque los tests pasen.
// Ver platform/pkg/ingressroutes.
func TestIngressPublishesEveryMountedPrefix(t *testing.T) {
	routes, ok := NewRouter(Deps{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	ingressroutes.AssertCovered(t, routes, "integrations", ingressroutes.Internal)
}

// TestLasPuertasInternasNoSeAbrenDesdeInternet (NEG-23): /operator y /internal
// contestan 404 a lo que llega por el Ingress (X-Forwarded-For), aun con la
// credencial buena; desde dentro siguen pidiendo su credencial.
func TestLasPuertasInternasNoSeAbrenDesdeInternet(t *testing.T) {
	r := NewRouter(Deps{Cfg: config.Config{OperatorToken: "op", InternalSecret: "in"}})
	pedir := func(ruta, cabecera, valor string, deFuera bool) int {
		req := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
		req.Header.Set(cabecera, valor)
		if deFuera {
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if c := pedir("/operator/templates", "Authorization", "Bearer op", true); c != http.StatusNotFound {
		t.Errorf("/operator desde internet: %d, quiero 404", c)
	}
	if c := pedir("/internal/v1/storage/instances?tenant=acme", "X-Sytue-Internal", "in", true); c != http.StatusNotFound {
		t.Errorf("/internal desde internet: %d, quiero 404", c)
	}
	if c := pedir("/operator/templates", "Authorization", "Bearer otro", false); c != http.StatusUnauthorized {
		t.Errorf("/operator desde dentro sin el token: %d, quiero 401", c)
	}
}
