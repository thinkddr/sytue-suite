// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del alta ATÓMICA desde plantilla contra un PostgreSQL real.
//
// Lo que se prueba es lo único que no se puede probar con un doble: que las cinco
// escrituras del alta viajan en LA MISMA transacción de verdad. Un fallo a mitad
// tiene que dejar la base de datos exactamente como estaba —sin proyectos
// huérfanos con servicios a medias, que es lo que producía la cadena de cuatro
// POST que esto sustituye— y un alta completa tiene que dejarlo TODO, incluido el
// primer despliegue encolado.
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestInTemplateTxIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// env_vars NO tiene política RLS de mantenimiento (a diferencia de projects,
	// managed_services, builds y deployments, que la reciben en las migraciones
	// 0002/0003), así que una consulta cross-tenant NO ve sus filas: contaría 0
	// pasara lo que pasara —y la aserción de «no queda nada» pasaría en falso—.
	// Sus filas se cuentan por la API del store bajo el contexto del tenant, que
	// además es lo que de verdad importa: que el dueño las vea.
	envs := func(projectID uuid.UUID) []domain.EnvVar {
		t.Helper()
		list, err := st.ListEnvVars(ctx, projectID)
		if err != nil {
			t.Fatalf("ListEnvVars: %v", err)
		}
		return list
	}

	// alta ejecuta los cinco pasos del alta; failAfter aborta tras el paso n.
	// Publica el id del proyecto creado para poder consultarlo después.
	var creado uuid.UUID
	alta := func(slug string, failAfter int) error {
		paso := 0
		boom := errors.New("fallo simulado")
		return st.InTemplateTx(ctx, func(ops store.TemplateOps) error {
			p, err := ops.CreateProject(ctx, store.NewProjectRecord{
				Slug: slug, Name: slug, GitProvider: domain.GitGeneric,
				ProductionRef: "main", Framework: domain.FrameworkGo,
			}, 0)
			if err != nil {
				return err
			}
			creado = p.ID
			if paso++; paso == failAfter {
				return boom
			}
			if _, err := ops.ProvisionService(ctx, store.NewServiceRecord{
				ProjectID: p.ID, Kind: domain.ServicePostgres, Name: "postgres",
				Config: map[string]string{"pvc_size": "2Gi"},
			}, 0); err != nil {
				return err
			}
			if paso++; paso == failAfter {
				return boom
			}
			if _, err := ops.UpsertEnvVar(ctx, store.EnvVarRecord{
				ProjectID: p.ID, Key: "SESSION_SECRET", ValueEnc: []byte("cifrado"), IsSecret: true,
			}); err != nil {
				return err
			}
			if paso++; paso == failAfter {
				return boom
			}
			b, err := ops.EnqueueBuild(ctx, store.NewBuildRecord{ProjectID: p.ID, GitRef: "main"})
			if err != nil {
				return err
			}
			_, err = ops.CreateDeployment(ctx, store.NewDeploymentRecord{
				ProjectID: p.ID, BuildID: &b.ID, Target: domain.TargetProduction,
				GitRef: "main", URL: "https://" + slug + ".apps.sytue.eu",
			})
			return err
		})
	}

	// El tercer paso falla: no puede quedar NADA, ni siquiera el proyecto y el
	// servicio que sí llegaron a insertarse.
	if err := alta("rota", 3); err == nil {
		t.Fatal("se esperaba el error del tercer paso")
	}
	for _, q := range []struct {
		what string
		sql  string
	}{
		{"proyectos", `SELECT count(*) FROM projects WHERE tenant='acme'`},
		{"servicios", `SELECT count(*) FROM managed_services WHERE tenant='acme'`},
		{"builds", `SELECT count(*) FROM builds WHERE tenant='acme'`},
		{"despliegues", `SELECT count(*) FROM deployments WHERE tenant='acme'`},
	} {
		if n := maintCount(t, h.Pool, q.sql); n != 0 {
			t.Fatalf("tras el fallo quedaron %d %s; se esperaban 0", n, q.what)
		}
	}
	if n := len(envs(creado)); n != 0 {
		t.Fatalf("tras el fallo quedaron %d variables; se esperaban 0", n)
	}

	// Alta completa: se persiste todo y el proyecto nace con su primer despliegue
	// en cola (antes no había ninguno).
	if err := alta("entera", 0); err != nil {
		t.Fatalf("alta completa: %v", err)
	}
	for _, q := range []struct {
		what string
		sql  string
	}{
		{"proyectos", `SELECT count(*) FROM projects WHERE tenant='acme' AND slug='entera'`},
		{"servicios", `SELECT count(*) FROM managed_services WHERE tenant='acme' AND status='provisioning'`},
		{"builds", `SELECT count(*) FROM builds WHERE tenant='acme' AND status='queued'`},
		{"despliegues", `SELECT count(*) FROM deployments WHERE tenant='acme' AND target='production' AND status='queued'`},
	} {
		if n := maintCount(t, h.Pool, q.sql); n != 1 {
			t.Fatalf("%s = %d; se esperaba 1", q.what, n)
		}
	}
	vars := envs(creado)
	if len(vars) != 1 {
		t.Fatalf("variables = %d; se esperaba 1", len(vars))
	}
	if vars[0].Key != "SESSION_SECRET" || !vars[0].IsSecret {
		t.Fatalf("variable = %q (secreta: %t); se esperaba SESSION_SECRET secreta", vars[0].Key, vars[0].IsSecret)
	}
}
