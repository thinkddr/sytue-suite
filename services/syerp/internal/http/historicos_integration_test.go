// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
)

// Solo la tarea que te puso OTRA persona y sigue en pie, a ti, con la hora en que se creó.
func TestAvisosHistoricosDeTareas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	cuando := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		for _, a := range [][3]string{{"bea", "ana", "pendiente"}, {"ana", "ana", "pendiente"}, {"", "ana", "pendiente"}, {"carla", "ana", "cancelada"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO actividades (tenant, tipo, resumen, vence, responsable_sub, creada_por, creada_en, estado)
				VALUES ('acme', 'tarea', 'Llamar', '2026-09-10', $1, $2, $3, $4)`, a[0], a[1], cuando, a[2]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	avisos, err := AvisosHistoricos(ctx, h.Pool, "https://erp.sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 1 || avisos[0].Subject != "bea" || !avisos[0].Fecha.Equal(cuando) || avisos[0].Type != "erp.task.assigned" {
		t.Fatalf("avisos %+v, quiero solo el de Bea con su fecha", avisos)
	}
}
