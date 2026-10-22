// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
)

// conTodaLaOrganizacion abre la puerta de mantenimiento dentro de la transacción
// de una persona. Lo que no puede pasar es que se quede abierta: el resto de esa
// transacción vería todos los ámbitos y todas las organizaciones.
func TestConTodaLaOrganizacionDejaLaPuertaComoEstaba(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	puerta := func(tx pgx.Tx) string {
		t.Helper()
		var v string
		if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.maintenance', true), '')`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	fallo := errors.New("fallo de fn")
	recorrer := func(tx pgx.Tx, antes string) error {
		for _, devuelve := range []error{nil, fallo} {
			var dentro string
			err := conTodaLaOrganizacion(ctx, tx, func() error { dentro = puerta(tx); return devuelve })
			if !errors.Is(err, devuelve) {
				t.Errorf("devuelve %v, esperaba %v", err, devuelve)
			}
			if dentro != "on" {
				t.Errorf("dentro la puerta está %q, esperaba abierta", dentro)
			}
			if despues := puerta(tx); despues != antes {
				t.Errorf("la puerta queda %q y estaba %q (fn devolvió %v)", despues, antes, devuelve)
			}
		}
		return nil
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { return recorrer(tx, "") }); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error { return recorrer(tx, "on") }); err != nil {
		t.Fatal(err)
	}
}
