// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
)

// TestIngressPublishesEveryMountedPrefix: todo prefijo que Rooms monta tiene que estar
// en el Ingress de su chart. Sin esto, `/hooks` estuvo montado, probado y documentado
// como entregado mientras Slack y Teams POSTeaban contra el SPA de Astro.
func TestIngressPublishesEveryMountedPrefix(t *testing.T) {
	routes, ok := NewRouter(Deps{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	// /internal es la API servicio-a-servicio (Hub, Drive): viaja por la red del
	// clúster y NO debe publicarse en el Ingress.
	ingressroutes.AssertCovered(t, routes, "rooms", ingressroutes.Internal)
}
