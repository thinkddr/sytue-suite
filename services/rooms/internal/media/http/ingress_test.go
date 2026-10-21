// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
)

// TestIngressPublishesEveryMountedPrefix: todo prefijo que monta Meet tiene que estar
// en el Ingress de su chart, o en producción se lo queda el SPA aunque los tests pasen.
// Ver platform/pkg/ingressroutes.
func TestIngressPublishesEveryMountedPrefix(t *testing.T) {
	routes, ok := NewRouter(Deps{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	// /webhooks/livekit lo llama el SFU, que corre DENTRO del clúster: va por el
	// Service interno, no por el Ingress. Publicarlo sería exponer a Internet un
	// endpoint que solo necesita tráfico este-oeste.
	//
	// El chart de LiveKit ya fija `webhook.urls` apuntando al Service interno de Meet
	// (deploy/helm/charts/livekit): el SFU avisa de los cierres de sala por dentro del
	// clúster, sin pasar por el Ingress.
	internal := append(append([]string{}, ingressroutes.Internal...), "/webhooks")
	ingressroutes.AssertCovered(t, routes, "rooms", internal)
}
