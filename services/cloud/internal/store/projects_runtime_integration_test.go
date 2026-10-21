// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración de la ESCALA de una app contra un PostgreSQL real.
//
// Lo que se prueba no es que el UPDATE guarde —eso lo garantiza Postgres— sino la
// consecuencia que hace que el cambio SIRVA: el controlador solo reaplica las
// producciones cuyo updated_at está dentro de su ventana de recencia, así que
// cambiar el tamaño de una app estable tiene que devolver su producción a esa
// ventana. Si no, la configuración quedaría bien guardada y los pods seguirían
// exactamente igual, que es la peor forma de fallar: en silencio.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestUpdateProjectRuntimeIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nuevaProduccionVieja := func(t *testing.T, slug string) (domain.Project, time.Time) {
		t.Helper()
		p := seedProject(t, ctx, st, slug)
		d, err := st.CreateDeployment(ctx, store.NewDeploymentRecord{
			ProjectID: p.ID, Target: domain.TargetProduction, GitRef: "main",
			URL: "https://" + slug + ".apps.sytue.eu",
		})
		if err != nil {
			t.Fatalf("CreateDeployment: %v", err)
		}
		// Producción estable y antigua: desplegada hace un mes, fuera de la ventana.
		//
		// Truncado a MICROSEGUNDOS a propósito: es la precisión de `timestamptz`, y el
		// `time.Now()` de Go trae nanosegundos. Sin truncar, lo que se escribe y lo que
		// se lee de vuelta ya no son el mismo instante, así que el `Equal` de más abajo
		// —«renombrar no tocó la producción»— fallaba prácticamente siempre por 343 ns,
		// acusando a la consulta de un cambio que nunca hizo.
		antes := time.Now().Add(-30 * 24 * time.Hour).Truncate(time.Microsecond)
		maintExec(t, h.Pool, `UPDATE deployments SET status='ready', updated_at=$2 WHERE id=$1`,
			d.ID, antes)
		return p, antes
	}

	// La lectura va dentro de una transacción de MANTENIMIENTO: sin ese flag la RLS
	// no deja ver ni una fila (deny-by-default), que es justo lo que debe pasar.
	updatedAt := func(t *testing.T, projectID uuid.UUID) time.Time {
		t.Helper()
		var at time.Time
		err := h.Pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT updated_at FROM deployments WHERE project_id=$1`, projectID).Scan(&at)
		})
		if err != nil {
			t.Fatalf("leer updated_at: %v", err)
		}
		return at
	}

	t.Run("cambiar la escala devuelve la producción a la ventana", func(t *testing.T) {
		p, antes := nuevaProduccionVieja(t, "web")
		size, count := "pro", 3
		got, err := st.UpdateProject(ctx, p.ID, store.UpdateProjectRecord{
			InstanceSize: &size, InstanceCount: &count,
		})
		if err != nil {
			t.Fatalf("UpdateProject: %v", err)
		}
		if got.InstanceSize != "pro" || got.InstanceCount != 3 {
			t.Errorf("la escala no se guardó: %q x%d", got.InstanceSize, got.InstanceCount)
		}
		if at := updatedAt(t, p.ID); !at.After(antes) {
			t.Error("la producción sigue fuera de la ventana: el controlador no la reaplicaría " +
				"y los pods se quedarían con el tamaño viejo")
		}
	})

	t.Run("el puerto y la sonda también la devuelven", func(t *testing.T) {
		p, antes := nuevaProduccionVieja(t, "api")
		port, path := 3000, "/healthz"
		if _, err := st.UpdateProject(ctx, p.ID, store.UpdateProjectRecord{
			AppPort: &port, HealthPath: &path,
		}); err != nil {
			t.Fatalf("UpdateProject: %v", err)
		}
		if at := updatedAt(t, p.ID); !at.After(antes) {
			t.Error("cambiar el contrato de ejecución también tiene que reaplicar")
		}
	})

	// El contrapunto: renombrar no cambia NADA de lo que corre en el clúster, así
	// que no puede colar una producción antigua en la ventana. Si lo hiciera, cada
	// edición de metadatos provocaría un reapply inútil de una app estable.
	t.Run("renombrar no reaplica nada", func(t *testing.T) {
		p, antes := nuevaProduccionVieja(t, "docs")
		name := "Documentación"
		if _, err := st.UpdateProject(ctx, p.ID, store.UpdateProjectRecord{Name: &name}); err != nil {
			t.Fatalf("UpdateProject: %v", err)
		}
		if at := updatedAt(t, p.ID); !at.Equal(antes) {
			t.Errorf("un cambio de nombre no puede tocar la producción (updated_at %v -> %v)", antes, at)
		}
	})

	// Un tamaño fuera del catálogo no puede llegar a la base de datos: la validación
	// del dominio es la primera red y el CHECK de la migración la última.
	t.Run("un tamaño inventado se rechaza", func(t *testing.T) {
		if err := domain.ValidateInstanceSize("gigante"); err == nil {
			t.Error("un tamaño fuera del catálogo debe rechazarse antes de tocar la BD")
		}
	})
}

// La comprobación de disponibilidad del slug tiene que ver TODA la plataforma, no
// solo el tenant: el slug es el subdominio público y su índice único es global. Si
// mirara solo dentro del tenant diría «libre» de un nombre que otra organización ya
// tiene, y la creación fallaría con el mismo 409 que la sugerencia venía a evitar.
func TestSlugsTakenVeTodaLaPlataformaIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)

	acme := tenantCtx("acme")
	seedProject(t, acme, st, "puerto-sereno")

	// Desde OTRO tenant, que es el caso que importa. Y en UNA sola consulta para
	// todos los candidatos: es como lo llaman la sugerencia (8 nombres) y la
	// comprobación en vivo (el nombre del usuario + sus alternativas).
	otra := tenantCtx("otra-empresa")
	taken, err := st.SlugsTaken(otra, []string{"puerto-sereno", "duna-serena", "ruhiges-tal"})
	if err != nil {
		t.Fatalf("SlugsTaken: %v", err)
	}
	if !taken["puerto-sereno"] {
		t.Error("un slug de otro tenant debe verse ocupado: el índice único es global")
	}
	if taken["duna-serena"] || taken["ruhiges-tal"] {
		t.Error("un slug que nadie tiene debe salir libre")
	}

	vacio, err := st.SlugsTaken(otra, nil)
	if err != nil || len(vacio) != 0 {
		t.Errorf("sin candidatos no hay consulta que hacer: %v, %v", vacio, err)
	}
}
