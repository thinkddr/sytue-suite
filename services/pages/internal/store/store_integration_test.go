// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// El esquema cumple las dos reglas de la suite, comprobadas por CATÁLOGO: toda tabla
// por organización con RLS forzada, y toda tabla con `tenant` con una política de
// fusión que el preflight de Sytue ID reconozca (también `events_outbox`).
func TestEsquema(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, map[string]string{
		// El despachador de eventos la recorre CROSS-TENANT a propósito: un solo worker
		// publica los eventos de todas las organizaciones (platform/pkg/events).
		"events_outbox": "la lee el despachador de eventos cross-tenant, por diseño",
	})
	pgtest.ExigirPoliticaDeFusion(t, h)
}

// Las tres maneras de leer: la organización ve lo suyo y nada más; la lectura
// pública ve todos los sitios y no escribe; la CLI suspende y reactiva cruzando
// organizaciones, y TenantDeSitio encuentra al dueño.
func TestModosDeLectura(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := New(h.Pool)
	ctx := context.Background()

	var sitio uuid.UUID
	if err := s.enTenant(ctx, "acme", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO sites (tenant, slug, name, draft, created_by)
			VALUES ('acme', 'panaderia-aurora', 'Panadería Aurora', '{}', 'sub-1') RETURNING id`).Scan(&sitio)
	}); err != nil {
		t.Fatalf("crear sitio: %v", err)
	}

	contar := func(tenant string) (n int) {
		t.Helper()
		if err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM sites`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if contar("acme") != 1 || contar("globex") != 0 {
		t.Fatal("la RLS no separa organizaciones")
	}

	if err := s.enPublico(ctx, func(tx pgx.Tx) error {
		var slug string
		if err := tx.QueryRow(ctx, `SELECT slug FROM sites WHERE id = $1`, sitio).Scan(&slug); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE sites SET name = 'x' WHERE id = $1`, sitio)
		if err == nil && tag.RowsAffected() != 0 {
			t.Error("la lectura pública ha podido escribir")
		}
		return nil
	}); err != nil {
		t.Fatalf("lectura pública: %v", err)
	}

	if tenant, err := s.TenantDeSitio(ctx, sitio); err != nil || tenant != "acme" {
		t.Fatalf("TenantDeSitio = %q, %v", tenant, err)
	}
	if _, err := s.TenantDeSitio(ctx, uuid.New()); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("TenantDeSitio de un sitio que no existe = %v", err)
	}

	estado := func() (st, motivo string) {
		t.Helper()
		if err := s.enTenant(ctx, "acme", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT status, suspended_reason FROM sites WHERE id = $1`, sitio).Scan(&st, &motivo)
		}); err != nil {
			t.Fatal(err)
		}
		return st, motivo
	}
	if err := s.Suspender(ctx, "panaderia-aurora", "phishing"); err != nil {
		t.Fatal(err)
	}
	if st, motivo := estado(); st != "suspended" || motivo != "phishing" {
		t.Fatalf("tras suspender: %s, %q", st, motivo)
	}
	if err := s.Reactivar(ctx, "panaderia-aurora"); err != nil {
		t.Fatal(err)
	}
	if st, motivo := estado(); st != "active" || motivo != "" {
		t.Fatalf("tras reactivar: %s, %q", st, motivo)
	}
	if err := s.Suspender(ctx, "no-existe", "x"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("suspender un slug que no existe = %v", err)
	}
}

// El buscador del Hub: solo lo de la organización, sin sitios en la papelera, por nombre
// o slug sin distinguir mayúsculas, con el término tomado al pie de la letra (`%` y `_`
// no comodinean) y lo más reciente primero.
func TestBuscarSitios(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := New(h.Pool)
	ctx := context.Background()

	alta := func(tenant, slug, nombre string, borrado bool) {
		t.Helper()
		if err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO sites (tenant, slug, name, draft, created_by, deleted_at)
				VALUES ($1, $2, $3, '{}', 'sub-1', CASE WHEN $4 THEN now() END)`, tenant, slug, nombre, borrado)
			return err
		}); err != nil {
			t.Fatalf("alta %s: %v", slug, err)
		}
	}
	alta("acme", "panaderia-aurora", "Panadería Aurora", false)
	alta("acme", "tienda-luna", "Tienda Luna", false)
	alta("acme", "vieja", "Panadería vieja", true)
	alta("globex", "panaderia-otra", "Panadería de otra", false)

	buscar := func(q string) []string {
		t.Helper()
		res, err := s.BuscarSitios(ctx, "acme", q, 10)
		if err != nil {
			t.Fatalf("BuscarSitios(%q): %v", q, err)
		}
		slugs := make([]string, 0, len(res))
		for _, r := range res {
			slugs = append(slugs, r.Slug)
		}
		return slugs
	}
	if got := buscar("PANADERÍA"); len(got) != 1 || got[0] != "panaderia-aurora" {
		t.Errorf("por nombre = %v", got)
	}
	if got := buscar("luna"); len(got) != 1 || got[0] != "tienda-luna" {
		t.Errorf("por slug = %v", got)
	}
	if got := buscar("%"); len(got) != 0 {
		t.Errorf("«%%» comodineó: %v", got)
	}
	if got := buscar(""); len(got) != 2 || got[0] != "tienda-luna" {
		t.Errorf("sin término: los recientes, el último primero; = %v", got)
	}
}
