// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package casthttp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// El router y el chart tienen que contar la misma historia (CAST-ENCARGO §13.12):
// lo que el servicio monta lo publica el ingress (/api/v1, /ws, /webhooks, /media),
// salvo lo deliberadamente interno y `/public`, que la web usa por dentro del
// clúster y las nativas piden por `/api/v1/public`.
func TestRutasCubiertasPorElIngress(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "..", "..", "deploy", "helm", "charts", "cast", "values.yaml")); err != nil {
		// El chart lo trae la rama de despliegue; hasta entonces no hay contra qué contrastar.
		t.Skip("deploy/helm/charts/cast/values.yaml no está en el árbol: el test espera a la rama de despliegue")
	}
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	router := NewRouter(Deps{Store: st, Servicio: svc})
	ingressroutes.AssertCovered(t, router.(chi.Routes), "cast", append(ingressroutes.Internal, "/public"))
}
