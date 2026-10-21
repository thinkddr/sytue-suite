// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El guard de arranque de AppRole contra un PostgreSQL de verdad: cuando el rol
// de aplicación NO se puede asumir, lo único que sostiene el aislamiento es que
// todas las tablas con RLS la lleven FORZADA, y eso se lee del catálogo.
//
// Lo que se prueba aquí es que ese recuento MIRA las tablas particionadas. No
// mirarlas no era teórico: `analytics_events` —la tabla más caliente de la
// plataforma— es particionada, así que era justo la que el guard no habría visto
// si algún día se creara sin FORCE.
package pg_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

func TestResolveCuentaLasParticionadasSinForce(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()

	// El pool del harness conecta con el rol de aplicación (NOSUPERUSER,
	// NOBYPASSRLS), que es el escenario del guard. El nombre de rol es uno que no
	// existe a propósito: así `puedeAsumirse` dice que no y Resolve pasa por las
	// comprobaciones que interesan.
	rol := pg.NewAppRole("rol_que_no_existe_app")

	if _, err := rol.Resolve(ctx, h.Pool); err != nil {
		t.Fatalf("con el esquema limpio Resolve debería aceptar: %v", err)
	}

	// DDL con la conexión privilegiada: el rol de aplicación no crea tablas.
	priv := h.PoolSinRLS(t)

	// Una tabla PARTICIONADA con RLS habilitada pero SIN forzarla: sus políticas
	// no se aplicarían al dueño, que es el usuario de la conexión del servicio.
	for _, ddl := range []string{
		`CREATE TABLE eventos_por_org (org_id uuid NOT NULL, id bigint NOT NULL)
		     PARTITION BY LIST (org_id)`,
		`ALTER TABLE eventos_por_org ENABLE ROW LEVEL SECURITY`,
	} {
		if _, err := priv.Exec(ctx, ddl); err != nil {
			t.Fatalf("preparar la tabla particionada (%s): %v", ddl, err)
		}
	}

	_, err := rol.Resolve(ctx, h.Pool)
	if err == nil {
		t.Fatal("Resolve dejó arrancar con una tabla particionada con RLS sin FORCE")
	}
	if !strings.Contains(err.Error(), "sin FORCE") {
		t.Fatalf("el error no explica el motivo: %v", err)
	}

	// Y con FORCE vuelve a aceptar: lo que bloquea es la tabla, no el hecho de
	// que exista una partitioned table.
	if _, err := priv.Exec(ctx, `ALTER TABLE eventos_por_org FORCE ROW LEVEL SECURITY`); err != nil {
		t.Fatalf("forzar la RLS: %v", err)
	}
	if _, err := rol.Resolve(ctx, h.Pool); err != nil {
		t.Fatalf("con FORCE debería aceptar: %v", err)
	}
}
