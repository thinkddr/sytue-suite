// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package peticiones_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// La tabla de un servicio cuyo recurso se repite entre organizaciones (un módulo
// del ERP), con sus columnas y su política, como la crea cada migración.
var migraciones = fstest.MapFS{"m/0001_peticiones.up.sql": {Data: []byte(`
CREATE TABLE peticiones_de_prueba (
    recurso text        NOT NULL,
    tenant  text        NOT NULL,
    sujeto  text        NOT NULL,
    nombre  text        NOT NULL DEFAULT '',
    correo  text        NOT NULL DEFAULT '',
    creada  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, recurso, sujeto)
);
ALTER TABLE peticiones_de_prueba ENABLE ROW LEVEL SECURITY;
ALTER TABLE peticiones_de_prueba FORCE ROW LEVEL SECURITY;
CREATE POLICY peticiones_de_prueba_tenant_isolation ON peticiones_de_prueba
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
`)}}

// TestSinRLSLaPeticionEsDeSuOrganizacion: con un rol que se salta la RLS —el
// dueño, como el despliegue de hoy—, la petición de una organización no se
// lista, no se ve y no se quita desde otra, aunque el recurso se llame igual.
func TestSinRLSLaPeticionEsDeSuOrganizacion(t *testing.T) {
	h := pgtest.New(t, migraciones, "m")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	var super, bypass bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if !super && !bypass {
		t.Fatal("el rol dueño del harness no se salta la RLS: este test no probaría el SQL")
	}

	tabla := peticiones.Tabla{Nombre: "peticiones_de_prueba", Recurso: "recurso", PorTenant: true}
	en := func(org string, fn func(tx pgx.Tx) error) {
		t.Helper()
		if err := pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: org}), fn); err != nil {
			t.Fatalf("en %s: %v", org, err)
		}
	}

	en("acme", func(tx pgx.Tx) error {
		_, err := tabla.Pedir(ctx, tx, "acme", peticiones.Peticion{Recurso: "facturacion", Sujeto: "bea", Nombre: "Bea"})
		return err
	})
	en("globex", func(tx pgx.Tx) error {
		lista, err := tabla.Listar(ctx, tx, "facturacion")
		if err != nil || len(lista) != 0 {
			t.Errorf("globex lista %d peticiones de acme (%v)", len(lista), err)
		}
		if hay, err := tabla.Hay(ctx, tx, "facturacion", "bea"); err != nil || hay {
			t.Errorf("globex ve la petición de acme (hay=%v, %v)", hay, err)
		}
		if quitada, err := tabla.Quitar(ctx, tx, "facturacion", "bea"); err != nil || quitada {
			t.Errorf("globex quita la petición de acme (quitada=%v, %v)", quitada, err)
		}
		return nil
	})
	en("acme", func(tx pgx.Tx) error {
		lista, err := tabla.Listar(ctx, tx, "facturacion")
		if err != nil || len(lista) != 1 || lista[0].Sujeto != "bea" {
			t.Errorf("la petición de acme tenía que seguir ahí: %+v %v", lista, err)
		}
		if hay, err := tabla.Hay(ctx, tx, "facturacion", "bea"); err != nil || !hay {
			t.Errorf("acme no ve su petición (hay=%v, %v)", hay, err)
		}
		return nil
	})
}
