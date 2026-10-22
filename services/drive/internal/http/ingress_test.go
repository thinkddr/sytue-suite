// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
)

// TestIngressPublishesEveryMountedPrefix: todo prefijo que Drive monta tiene que estar
// en el Ingress de su chart. Sin esto, los enlaces públicos (`/public/{token}`) —cuya
// URL compone el propio servicio para que el usuario la reparta— y la capa de
// compatibilidad Google/Graph (`/compat`) quedaron sin enrutar en producción.
func TestIngressPublishesEveryMountedPrefix(t *testing.T) {
	routes, ok := NewRouter(Deps{Cfg: config.Config{PublicURL: "https://drive.sytue.eu"}}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	ingressroutes.AssertCovered(t, routes, "drive", ingressroutes.Internal)
}
