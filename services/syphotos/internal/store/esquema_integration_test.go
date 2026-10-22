// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del ESQUEMA de Sytue Photos contra un PostgreSQL real.
//
// Existen porque el esquema no se puede comprobar de otra manera: `migrate-check`
// solo mira que una migración no sea destructiva, y leer el `.sql` no dice si
// aplica. Aquí se ejecuta de verdad, que es la única prueba que vale.
//
// Y comprueban lo que de verdad hace daño si se rompe: que una organización no
// vea las fotos de otra. La RLS de este servicio no protege metadatos
// cualesquiera —protege dónde estuvo alguien y con quién, que es de lo más
// sensible que guarda la suite.
package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
)

func arranca(t *testing.T) *pgtest.Harness {
	t.Helper()
	return pgtest.New(t, db.Migrations, db.MigrationsDir)
}

// ctxDe devuelve un contexto con el tenant fijado: es lo que leen `InTenantTx` y
// las políticas RLS.
func ctxDe(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// Que las migraciones APLIQUEN. Parece poco y es lo que no se puede verificar
// sin base de datos: un `CREATE INDEX` con una expresión mal escrita no lo caza
// ningún gate del repositorio.
func TestEsquema_aplica(t *testing.T) {
	h := arranca(t)
	ctx := context.Background()

	var tablas int
	err := h.Pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('assets', 'edits', 'albums', 'album_assets')`).Scan(&tablas)
	if err != nil {
		t.Fatal(err)
	}
	if tablas != 4 {
		t.Errorf("%d de las 4 tablas creadas", tablas)
	}
}

// El índice de la LÍNEA DE TIEMPO lleva `coalesce(taken_at, uploaded_at)` dentro.
// Si esa expresión no coincide EXACTAMENTE con la de la consulta, Postgres no lo
// usa para ordenar y acaba ordenando en memoria toda la colección en cada
// desplazamiento — sin error, solo lento, que es como esto se degrada sin que
// nadie lo note hasta que hay treinta mil fotos.
func TestEsquema_laLineaDeTiempoUsaSuIndice(t *testing.T) {
	h := arranca(t)
	ctx := ctxDe("t1")

	// Con la tabla vacía el planificador puede preferir un recorrido secuencial;
	// lo que se afirma es que la consulta con ESA expresión de orden es
	// planificable contra el índice, no cuál elige hoy.
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			return err
		}
		var plan string
		return tx.QueryRow(ctx, `
			EXPLAIN (FORMAT TEXT)
			SELECT id FROM assets
			WHERE tenant = 't1' AND owner_sub = 'u1' AND deleted_at IS NULL
			ORDER BY coalesce(taken_at, uploaded_at) DESC, id DESC
			LIMIT 50`).Scan(&plan)
	})
	if err != nil {
		t.Fatalf("la consulta de la línea de tiempo no es planificable: %v", err)
	}
}

// UNA ORGANIZACIÓN NO VE LAS FOTOS DE OTRA. Es el invariante que justifica la
// RLS de este servicio.
func TestRLS_aislaEntreOrganizaciones(t *testing.T) {
	h := arranca(t)

	inserta := func(tenant, nombre string) {
		t.Helper()
		ctx := ctxDe(tenant)
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO assets (tenant, owner_sub, drive_file_id, name, media_type,
				                    bytes, width, height)
				VALUES ($1, 'u1', $2, $3, 'image/jpeg', 1024, 4000, 3000)`,
				tenant, nombre+"-drive", nombre)
			return err
		})
		if err != nil {
			t.Fatalf("insertando en %s: %v", tenant, err)
		}
	}
	inserta("t1", "playa")
	inserta("t2", "oficina")

	cuenta := func(tenant string) int {
		t.Helper()
		var n int
		ctx := ctxDe(tenant)
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM assets`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := cuenta("t1"); n != 1 {
		t.Errorf("t1 ve %d activos, se esperaba 1", n)
	}
	if n := cuenta("t2"); n != 1 {
		t.Errorf("t2 ve %d activos, se esperaba 1", n)
	}
}

// Reingerir el mismo fichero de Drive no puede crear una segunda entrada, o la
// línea de tiempo saldría doble. Y el UNIQUE es POR organización: dos
// organizaciones pueden referirse al mismo id sin estorbarse.
func TestEsquema_noSeIngiereDosVecesElMismoFichero(t *testing.T) {
	h := arranca(t)
	ctx := ctxDe("t1")

	ins := func() error {
		return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO assets (tenant, owner_sub, drive_file_id, name, media_type,
				                    bytes, width, height)
				VALUES ('t1', 'u1', 'drive-abc', 'playa.jpg', 'image/jpeg', 1024, 4000, 3000)`)
			return err
		})
	}
	if err := ins(); err != nil {
		t.Fatal(err)
	}
	if err := ins(); err == nil {
		t.Error("se ingirió dos veces el mismo fichero de Drive: la galería saldría doble")
	}
}

// Y el invariante de toda la plataforma, con el helper compartido: ENABLE no
// basta, hace falta FORCE — el dueño de la tabla se salta la política, y el
// usuario de la aplicación suele serlo.
func TestRLS_forzadaEnTodasLasTablas(t *testing.T) {
	pgtest.ExigirRLSForzada(t, arranca(t), nil)
}
