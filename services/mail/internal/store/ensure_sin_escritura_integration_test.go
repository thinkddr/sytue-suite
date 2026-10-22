// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// EnsureAccount es el preámbulo de TODA petición JMAP: 44 handlers lo llaman
// antes de hacer nada. Mientras resolvía la cuenta con un `INSERT ... ON
// CONFLICT DO UPDATE`, cada una de esas peticiones escribía una versión nueva de
// la fila (`updated_at = now()` hace que la actualización sea real): tupla
// muerta, su WAL, y un bloqueo de fila que serializaba a los clientes
// concurrentes del mismo buzón. Leer una cuenta no debe escribir nada.
//
// La señal es `xmin`, el id de transacción que escribió la versión visible de la
// fila: si Postgres la reescribe, cambia. Es exacta y síncrona, a diferencia de
// los contadores de pg_stat_user_tables, que el recolector publica cuando puede.
package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// xminCuenta devuelve el xmin de la fila de la cuenta.
func xminCuenta(t *testing.T, ctx context.Context, h *pgtest.Harness, id uuid.UUID) string {
	t.Helper()
	var x string
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT xmin::text FROM accounts WHERE id = $1`, id).Scan(&x)
	})
	if err != nil {
		t.Fatalf("leer xmin de la cuenta: %v", err)
	}
	return x
}

func TestEnsureAccountNoEscribeCuandoNadaCambia(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	antes := xminCuenta(t, ctx, h, acc.ID)

	// Diez peticiones JMAP con el MISMO token, que es el caso de siempre.
	for i := range 10 {
		got, err := st.EnsureAccount(ctx, "sub-ada@sytue.eu", "ada@sytue.eu", "Test", 1<<30)
		if err != nil {
			t.Fatalf("EnsureAccount #%d: %v", i, err)
		}
		if got.ID != acc.ID || got.Address != "ada@sytue.eu" || got.DisplayName != "Test" {
			t.Fatalf("EnsureAccount #%d devolvió otra cuenta: %+v", i, got)
		}
	}
	if despues := xminCuenta(t, ctx, h, acc.ID); despues != antes {
		t.Fatalf("xmin %s -> %s: EnsureAccount reescribe la fila de la cuenta aunque el token "+
			"no traiga nada nuevo. Son una tupla muerta y un bloqueo de fila por CADA petición JMAP", antes, despues)
	}

	// Y los buzones de sistema siguen siendo seis: el camino rápido se los salta
	// porque nacen en la misma transacción que la cuenta y DeleteMailbox se niega
	// a borrar un rol, no porque se hayan perdido por el camino.
	boxes, err := st.ListMailboxes(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	if len(boxes) != len(domain.DefaultMailboxes) {
		t.Fatalf("buzones = %d, quiero %d", len(boxes), len(domain.DefaultMailboxes))
	}

	// El contrapunto: cuando Sytue ID SÍ manda una dirección nueva, hay que
	// escribir. Un camino rápido que se tragara esto dejaría la cuenta anclada a
	// una dirección que ya no existe.
	movida, err := st.EnsureAccount(ctx, "sub-ada@sytue.eu", "ada@acme.sytue.eu", "Test", 1<<30)
	if err != nil {
		t.Fatalf("EnsureAccount con dirección nueva: %v", err)
	}
	if movida.Address != "ada@acme.sytue.eu" {
		t.Fatalf("Address = %q, quiero la nueva", movida.Address)
	}
	if despues := xminCuenta(t, ctx, h, acc.ID); despues == antes {
		t.Fatal("el camino rápido se tragó un cambio de dirección del token: la fila no se reescribió")
	}
}
