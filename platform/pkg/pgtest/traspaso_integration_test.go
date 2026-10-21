// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El SQL con el que el piloto separa los roles de PostgreSQL (INF-01,
// deploy/helm/sql/traspasar-propiedad.sql), contra un PostgreSQL de
// verdad y con una base que nace como nacen las de producción: todo creado por un
// superusuario.
package pgtest_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

// esquemaConDeTodo tiene una pieza de cada clase que las migraciones de la suite
// crean (tabla con RLS forzada, partición, secuencia de columna, tipo, función
// SECURITY DEFINER, extensión) y un rol de aplicación al estilo de docs_app.
var esquemaConDeTodo = fstest.MapFS{
	"migrations/0001_init.up.sql": &fstest.MapFile{Data: []byte(`
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TYPE estado AS ENUM ('abierto', 'cerrado');
CREATE TABLE cosas (
    id          bigserial PRIMARY KEY,
    tenant_slug text NOT NULL,
    estado      estado NOT NULL DEFAULT 'abierto'
);
ALTER TABLE cosas ENABLE ROW LEVEL SECURITY;
ALTER TABLE cosas FORCE  ROW LEVEL SECURITY;
CREATE POLICY cosas_tenant_isolation ON cosas
    USING (tenant_slug = current_setting('app.tenant', true))
    WITH CHECK (tenant_slug = current_setting('app.tenant', true));
CREATE TABLE eventos (tenant_slug text NOT NULL, dia date NOT NULL) PARTITION BY RANGE (dia);
CREATE TABLE eventos_2026 PARTITION OF eventos FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
-- Cuenta TODAS las filas: con un dueño superusuario se salta la RLS, que es
-- justo lo que el traspaso tiene que quitarle.
CREATE FUNCTION cuantas_cosas() RETURNS bigint LANGUAGE sql SECURITY DEFINER
    SET search_path = public AS 'SELECT count(*) FROM cosas';
DO $$ BEGIN
    CREATE ROLE traspaso_prueba_app NOLOGIN NOINHERIT NOBYPASSRLS;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
GRANT SELECT ON cosas TO traspaso_prueba_app;
`)},
}

func TestTraspasoDeLaPropiedad(t *testing.T) {
	t.Setenv("PGTEST_TRASPASO_SQL", "../../../deploy/helm/sql/traspasar-propiedad.sql")
	h := pgtest.New(t, esquemaConDeTodo, "migrations")
	ctx := context.Background()

	// Quien entra es el dueño nuevo, y no elude la RLS.
	var rol string
	var elude bool
	if err := h.Pool.QueryRow(ctx,
		`SELECT current_user, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&rol, &elude); err != nil {
		t.Fatalf("leer el rol del pool: %v", err)
	}
	if elude {
		t.Fatalf("el dueño nuevo (%s) elude la RLS", rol)
	}

	// Todo lo que no es de una extensión es suyo: la base, tablas, particiones,
	// secuencias, tipos y funciones.
	var base string
	if err := h.Pool.QueryRow(ctx,
		`SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()`).Scan(&base); err != nil {
		t.Fatal(err)
	}
	if base != rol {
		t.Errorf("la base es de %s, no de %s", base, rol)
	}
	var ajenos int
	if err := h.Pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		         WHERE n.nspname = 'public' AND c.relkind IN ('r','p','S','v','m')
		           AND pg_get_userbyid(c.relowner) <> current_user)
		     + (SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
		         WHERE n.nspname = 'public' AND t.typname = 'estado'
		           AND pg_get_userbyid(t.typowner) <> current_user)
		     + (SELECT count(*) FROM pg_proc p WHERE p.proname = 'cuantas_cosas'
		           AND pg_get_userbyid(p.proowner) <> current_user)`).Scan(&ajenos); err != nil {
		t.Fatal(err)
	}
	if ajenos != 0 {
		t.Errorf("quedan %d objetos que no son del dueño nuevo", ajenos)
	}
	// La extensión se queda con quien la instaló, y se sigue pudiendo usar.
	var deOtro bool
	if err := h.Pool.QueryRow(ctx,
		`SELECT pg_get_userbyid(proowner) <> current_user FROM pg_proc WHERE proname = 'gen_random_bytes' LIMIT 1`).
		Scan(&deOtro); err != nil || !deOtro {
		t.Errorf("la función de pgcrypto cambió de dueño (o no está): deOtro=%v err=%v", deOtro, err)
	}
	if _, err := h.Pool.Exec(ctx, `SELECT gen_random_bytes(4)`); err != nil {
		t.Errorf("el dueño nuevo no puede usar pgcrypto: %v", err)
	}

	// Dos organizaciones, sembradas con el rol privilegiado.
	priv, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	// El rol de aplicación es del servidor, que se comparte: fuera al acabar (antes
	// de que el arnés borre la base, mientras aún se le puede quitar lo concedido).
	t.Cleanup(func() {
		_, _ = priv.Exec(ctx, `DROP OWNED BY traspaso_prueba_app`)
		_, _ = priv.Exec(ctx, `DROP ROLE IF EXISTS traspaso_prueba_app`)
		_ = priv.Close(ctx)
	})
	if _, err := priv.Exec(ctx, `INSERT INTO cosas (tenant_slug) VALUES ('acme'), ('acme'), ('otra')`); err != nil {
		t.Fatal(err)
	}

	// La RLS forzada sujeta al dueño: ve lo de su organización y nada más.
	var vistas, porFuncion int
	err = h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant', 'acme', true)`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM cosas`).Scan(&vistas); err != nil {
			return err
		}
		// Y la SECURITY DEFINER, que ahora es del dueño, tampoco se la salta.
		return tx.QueryRow(ctx, `SELECT cuantas_cosas()`).Scan(&porFuncion)
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistas != 2 {
		t.Errorf("el dueño ve %d filas con app.tenant=acme; quería 2 (las de acme)", vistas)
	}
	if porFuncion != 2 {
		t.Errorf("la función SECURITY DEFINER cuenta %d filas; quería 2: sigue saltándose la RLS", porFuncion)
	}

	// El rol de aplicación se puede asumir: el traspaso le dio la pertenencia.
	err = h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SET LOCAL ROLE traspaso_prueba_app`)
		return err
	})
	if err != nil {
		t.Errorf("el dueño nuevo no puede asumir el rol de aplicación: %v", err)
	}
}
