// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// El constructor reclama la cola de TODAS las organizaciones sin saber de cuál es
// cada build. El arnés entra con un rol sin BYPASSRLS, como entrará producción al
// separar los roles (INF-01): sin la puerta de mantenimiento la RLS forzada de
// `builds` esconde la cola entera y ningún build se reclama, sin error.
func TestClaimNextBuildVeLaColaDeTodasLasOrganizaciones(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)

	acme := tenantCtx("acme")
	b := seedBuild(t, acme, st, seedProject(t, acme, st, "web").ID)

	cb, ok, err := st.ClaimNextBuild(context.Background(), "builder-1")
	if err != nil {
		t.Fatalf("ClaimNextBuild: %v", err)
	}
	if !ok {
		t.Fatal("la build en cola de acme no se reclama: el constructor no ve la cola")
	}
	if cb.Build.ID != b.ID || cb.Build.Status != domain.BuildRunning || cb.Project.Slug != "web" {
		t.Fatalf("reclamada %+v (proyecto %q), quiero la %s en running del proyecto web", cb.Build, cb.Project.Slug, b.ID)
	}

	// Y no se reclama dos veces.
	if _, ok, err := st.ClaimNextBuild(context.Background(), "builder-2"); err != nil || ok {
		t.Fatalf("segunda reclamación: ok=%v err=%v, quiero ninguna", ok, err)
	}
}
