// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package pageshttp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// El router y el chart cuentan la misma historia (PAGES-ENCARGO §16.1): el Ingress
// de pages.<dominio> publica /webhooks y /_borde. /api/v1 NO se publica (A1: solo lo
// llaman el BFF y el Hub por dentro) y /_pages llega por la IngressRoute de las
// webs, por Host, no por este Ingress.
func TestRutasCubiertasPorElIngress(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "..", "..", "deploy", "helm", "charts", "pages", "values.yaml")); err != nil {
		// El chart lo trae la rama de despliegue; hasta entonces no hay contra qué contrastar.
		t.Skip("deploy/helm/charts/pages/values.yaml no está en el árbol: el test espera a la rama de despliegue")
	}
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	router := NewRouter(Deps{Store: store.New(h.Pool)})
	ingressroutes.AssertCovered(t, router.(chi.Routes), "pages", append(ingressroutes.Internal, "/api/v1", "/_pages"))
}
