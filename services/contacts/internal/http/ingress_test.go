// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
)

// TestIngressPublishesEveryMountedPrefix: todo prefijo que monta Contactos tiene que estar
// en el Ingress de su chart, o en producción se lo queda el SPA aunque los tests pasen.
// Ver platform/pkg/ingressroutes.
func TestIngressPublishesEveryMountedPrefix(t *testing.T) {
	routes, ok := NewRouter(Deps{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter debería devolver un chi.Routes")
	}
	ingressroutes.AssertCovered(t, routes, "contacts", ingressroutes.Internal)
}
