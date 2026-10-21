// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Autocomprobación del harness: verifica LA garantía que el resto de la suite da
// por supuesta —que el pool ejercita la RLS— sin depender de ningún servicio. El
// esquema es un fstest.MapFS mínimo con la política que genera
// pg.TenantRLSPolicySQL, de modo que este test detecta una regresión de pgtest
// (volver a conectar con el rol superusuario, perder un GRANT) aunque nadie
// ejecute los tests de integración de los servicios.
package pgtest_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// tenancyCtx devuelve un contexto con el tenant fijado (lo que leen InTenantTx y
// las políticas RLS).
func tenancyCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// esquemaRLS es una tabla multi-tenant con la RLS deny-by-default de la suite.
var esquemaRLS = fstest.MapFS{
	"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE cosas (
    id          bigserial PRIMARY KEY,
    tenant_slug text NOT NULL,
    nombre      text NOT NULL
);
ALTER TABLE cosas ENABLE ROW LEVEL SECURITY;
ALTER TABLE cosas FORCE  ROW LEVEL SECURITY;
CREATE POLICY cosas_tenant_isolation ON cosas
    USING (tenant_slug = current_setting('app.tenant', true))
    WITH CHECK (tenant_slug = current_setting('app.tenant', true));
`)},
}

func TestHarnessEjercitaLaRLS(t *testing.T) {
	h := pgtest.New(t, esquemaRLS, "migrations")
	ctx := context.Background()

	var user string
	var super, bypass bool
	if err := h.Pool.QueryRow(ctx,
		`SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&user, &super, &bypass); err != nil {
		t.Fatalf("consultar pg_roles: %v", err)
	}
	t.Logf("rol del pool: %s (rolsuper=%v, rolbypassrls=%v)", user, super, bypass)
	if super || bypass {
		t.Fatalf("el pool conecta como %q con rolsuper=%v rolbypassrls=%v: PostgreSQL NO evalúa "+
			"ninguna política RLS para ese rol", user, super, bypass)
	}

	// Sembrado: una fila por tenant, cada una en su transacción con app.tenant.
	for _, slug := range []string{"acme", "globex"} {
		ctxT := tenancyCtx(slug)
		if err := h.Pool.InTenantTx(ctxT, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctxT, `INSERT INTO cosas (tenant_slug, nombre) VALUES ($1, 'x')`, slug)
			return err
		}); err != nil {
			t.Fatalf("sembrar %s: %v", slug, err)
		}
	}

	// La RLS, y nada más, limita el recuento: la consulta no filtra por tenant.
	for _, slug := range []string{"acme", "globex"} {
		ctxT := tenancyCtx(slug)
		var n int
		if err := h.Pool.InTenantTx(ctxT, func(tx pgx.Tx) error {
			return tx.QueryRow(ctxT, `SELECT count(*) FROM cosas`).Scan(&n)
		}); err != nil {
			t.Fatalf("contar en %s: %v", slug, err)
		}
		if n != 1 {
			t.Fatalf("%s ve %d filas, quiero 1 (fuga cross-tenant: la RLS no se aplica)", slug, n)
		}
	}

	// Sin tenant fijado: cero filas y el INSERT rechazado (deny-by-default).
	var n int
	if err := h.Pool.QueryRow(ctx, `SELECT count(*) FROM cosas`).Scan(&n); err != nil {
		t.Fatalf("contar sin app.tenant: %v", err)
	}
	if n != 0 {
		t.Fatalf("sin app.tenant se ven %d filas, quiero 0", n)
	}
	if _, err := h.Pool.Exec(ctx, `INSERT INTO cosas (tenant_slug, nombre) VALUES ('acme', 'y')`); err == nil {
		t.Fatal("INSERT sin app.tenant aceptado: el WITH CHECK de la política no se evalúa")
	}

	// Escribir en OTRO tenant desde el tenant activo: lo para el WITH CHECK.
	ctxT := tenancyCtx("acme")
	if err := h.Pool.InTenantTx(ctxT, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctxT, `INSERT INTO cosas (tenant_slug, nombre) VALUES ('globex', 'z')`)
		return err
	}); err == nil {
		t.Fatal("acme pudo insertar una fila de globex: fuga de escritura cross-tenant")
	}

	// El DSN privilegiado sigue siendo la escotilla documentada: NO ejercita RLS.
	admin := h.PoolSinRLS(t)
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM cosas`).Scan(&n); err != nil {
		t.Fatalf("contar con el rol privilegiado: %v", err)
	}
	if n != 2 {
		t.Fatalf("el rol privilegiado ve %d filas, quiero 2: h.DSN debe seguir saltándose la RLS", n)
	}
}

// La consulta de pg.ConsultaTablasConTenant —la del preflight de la fusión y la de
// ExigirPoliticaDeFusion— distingue la tabla con política de la que no la tiene, y
// no confunde una política de aislamiento con una de fusión.
func TestConsultaTablasConTenant(t *testing.T) {
	h := pgtest.New(t, fstest.MapFS{
		"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE con_politica (tenant text NOT NULL);
CREATE POLICY con_politica_platform_merge ON con_politica
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
CREATE TABLE sin_politica (tenant text NOT NULL);
CREATE POLICY sin_politica_tenant_isolation ON sin_politica
    USING (tenant = current_setting('app.tenant', true));
CREATE TABLE sin_tenant (id int);
`)},
	}, "migrations")
	tablas, sinPolitica, err := pg.ConsultaTablasConTenant(context.Background(), h.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(tablas, ","), "con_politica,sin_politica"; got != want {
		t.Errorf("tablas con tenant: %s, quiero %s", got, want)
	}
	if got, want := strings.Join(sinPolitica, ","), "sin_politica"; got != want {
		t.Errorf("sin política de fusión: %s, quiero %s", got, want)
	}
}
