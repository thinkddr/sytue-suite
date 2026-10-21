// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/analytics/db"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

// storeSinRLS conecta como el DUEÑO de la base: superusuario, con BYPASSRLS. Es como
// ha llegado a conectar el despliegue, y para ese rol PostgreSQL no evalúa NINGUNA
// política, ni con FORCE ROW LEVEL SECURITY. Lo que aquí separa a una organización de
// otra es solo lo que diga el SQL.
func storeSinRLS(t *testing.T) (*pg.Pool, *store.Store) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pool := h.PoolSinRLS(t)
	return pool, store.New(pool)
}

// agregados deja cifras de un día en las tres tablas que lee el panel.
func agregados(t *testing.T, pool *pg.Pool, tenant, siteID string, dia time.Time, paginas int) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO analytics_daily (tenant, site_id, day, pageviews, visitors, visits) VALUES ($1, $2, $3::date, $4, $4, $4)`,
		`INSERT INTO analytics_hourly (tenant, site_id, hour, pageviews, visitors) VALUES ($1, $2, $3::date, $4, $4)`,
		`INSERT INTO analytics_daily_breakdown (tenant, site_id, day, dim, value, pageviews, visitors) VALUES ($1, $2, $3::date, 'path', '/precios-internos', $4, $4)`,
	} {
		if _, err := pool.Exec(ctx, q, tenant, siteID, dia, paginas); err != nil {
			t.Fatalf("sembrar agregados: %v", err)
		}
	}
}

// COM-09. Con RLS apagada, la API de Analytics no separaba organizaciones: listar
// devolvía los sitios de todas, y leer, cambiar y borrar iban por el id a secas.
func TestSinRLSLaAnaliticaEsDeSuOrganizacion(t *testing.T) {
	pool, st := storeSinRLS(t)
	acme, globex := ctxTenant("acme"), ctxTenant("globex")
	dia := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	deAcme := creaSitio(t, st, "acme", "tienda.acme.example")
	deGlobex := creaSitio(t, st, "globex", "www.globex.example")
	agregados(t, pool, "acme", deAcme.ID, dia, 120)
	agregados(t, pool, "globex", deGlobex.ID, dia, 7)

	t.Run("listar", func(t *testing.T) {
		sitios, err := st.ListSites(globex)
		if err != nil {
			t.Fatal(err)
		}
		hosts := make([]string, 0, len(sitios))
		for _, s := range sitios {
			hosts = append(hosts, s.Host)
		}
		if !slices.Equal(hosts, []string{"www.globex.example"}) {
			t.Fatalf("globex ve los sitios %v, quiero solo el suyo", hosts)
		}
	})

	t.Run("leer las cifras de otra organización", func(t *testing.T) {
		tot, err := st.Summary(globex, deAcme.ID, dia, dia)
		if err != nil || tot.Pageviews != 0 || tot.Days != 0 {
			t.Fatalf("totales de acme vistos desde globex: %+v (%v)", tot, err)
		}
		for _, porHora := range []bool{false, true} {
			if pts, err := st.Timeseries(globex, deAcme.ID, dia, dia, porHora); err != nil || len(pts) != 0 {
				t.Fatalf("serie de acme vista desde globex (hora=%v): %+v (%v)", porHora, pts, err)
			}
		}
		if filas, err := st.Breakdown(globex, deAcme.ID, "path", dia, dia, 10); err != nil || len(filas) != 0 {
			t.Fatalf("desglose de acme visto desde globex: %+v (%v)", filas, err)
		}
		// Y su dueña sí las lee: el filtro no se ha llevado lo legítimo.
		tot, err = st.Summary(acme, deAcme.ID, dia, dia)
		if err != nil || tot.Pageviews != 120 || tot.Days != 1 {
			t.Fatalf("totales de acme: %+v (%v)", tot, err)
		}
		if pts, err := st.Timeseries(acme, deAcme.ID, dia, dia, true); err != nil || len(pts) != 1 {
			t.Fatalf("serie por horas de acme: %+v (%v)", pts, err)
		}
		if filas, err := st.Breakdown(acme, deAcme.ID, "path", dia, dia, 10); err != nil || len(filas) != 1 {
			t.Fatalf("desglose de acme: %+v (%v)", filas, err)
		}
	})

	t.Run("cambiar la lista de eventos de otra", func(t *testing.T) {
		if _, err := st.UpdateAllowlist(globex, deAcme.ID, []string{"espia"}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, quiero ErrNotFound", err)
		}
		sitios, err := st.ListSites(acme)
		if err != nil || len(sitios) != 1 || len(sitios[0].EventAllowlist) != 0 {
			t.Fatalf("la lista de eventos de acme ha cambiado: %+v (%v)", sitios, err)
		}
	})

	t.Run("borrar el sitio de otra", func(t *testing.T) {
		if err := st.DeleteSite(globex, deAcme.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, quiero ErrNotFound", err)
		}
		tot, err := st.Summary(acme, deAcme.ID, dia, dia)
		if err != nil || tot.Pageviews != 120 {
			t.Fatalf("el historial de acme ya no está: %+v (%v)", tot, err)
		}
		if err := st.DeleteSite(acme, deAcme.ID); err != nil {
			t.Fatalf("la dueña no puede borrar su sitio: %v", err)
		}
	})
}

// Un host que ya existe no cambia de organización, lo pida quien lo pida: antes el
// alta interna reescribía `tenant` y con él se iba el sitio entero.
func TestEnsureSiteNoCambiaElHostDeOrganizacion(t *testing.T) {
	_, st := storeSinRLS(t)
	acme, globex := ctxTenant("acme"), ctxTenant("globex")

	primero, err := st.EnsureSite(acme, "acme.sites.sytue.example", []string{"compra"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureSite(globex, "acme.sites.sytue.example", []string{"robo"}); !errors.Is(err, store.ErrHostDeOtro) {
		t.Fatalf("err = %v, quiero ErrHostDeOtro", err)
	}
	if sitios, err := st.ListSites(globex); err != nil || len(sitios) != 0 {
		t.Fatalf("globex se ha quedado con el sitio: %+v (%v)", sitios, err)
	}

	// Repetir el alta desde su dueña es idempotente y solo actualiza la lista.
	otra, err := st.EnsureSite(acme, "acme.sites.sytue.example", []string{"compra", "alta"})
	if err != nil {
		t.Fatal(err)
	}
	if otra.ID != primero.ID || otra.Tenant != "acme" || !slices.Equal(otra.EventAllowlist, []string{"compra", "alta"}) {
		t.Fatalf("sitio tras repetir el alta: %+v, quiero el mismo id %s con la lista nueva", otra, primero.ID)
	}
}
