// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// EnsureOrganization sirve GET /org, el arranque de la consola de Cloud.
// Mientras lo resolvía con un `INSERT ... ON CONFLICT DO UPDATE SET tenant =
// EXCLUDED.tenant`, cada carga de página escribía una versión nueva de la fila
// sin cambiar ni un valor: tupla muerta, WAL y un bloqueo sobre la única fila de
// organización del tenant.
//
// La señal es `xmin`, el id de transacción que escribió la versión visible de la
// fila: si Postgres la reescribe, cambia.
package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// xminOrg devuelve el xmin de la fila de organización del tenant.
func xminOrg(t *testing.T, ctx context.Context, h *pgtest.Harness, slug string) string {
	t.Helper()
	var x string
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT xmin::text FROM organizations WHERE tenant = $1`, slug).Scan(&x)
	})
	if err != nil {
		t.Fatalf("leer xmin de la organización: %v", err)
	}
	return x
}

func TestEnsureOrganizationNoEscribeCuandoYaExiste(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	org, err := st.EnsureOrganization(ctx, "Acme", domain.PlanFree, "fr-par")
	if err != nil {
		t.Fatalf("EnsureOrganization (alta): %v", err)
	}
	antes := xminOrg(t, ctx, h, "acme")

	// Diez cargas de la consola.
	for i := range 10 {
		got, err := st.EnsureOrganization(ctx, "Acme", domain.PlanFree, "fr-par")
		if err != nil {
			t.Fatalf("EnsureOrganization #%d: %v", i, err)
		}
		if got.ID != org.ID || got.Slug != "acme" {
			t.Fatalf("EnsureOrganization #%d devolvió otra organización: %+v", i, got)
		}
	}
	if despues := xminOrg(t, ctx, h, "acme"); despues != antes {
		t.Fatalf("xmin %s -> %s: EnsureOrganization reescribe la fila en cada llamada. "+
			"Es una tupla muerta y un bloqueo de fila por CADA carga de la consola", antes, despues)
	}
}
