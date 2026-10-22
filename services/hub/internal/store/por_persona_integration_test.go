// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// El tope por persona en todas las organizaciones aguanta la concurrencia (cerrojo por
// destinatario), y contar no le cambia a la transacción los buzones que tenía fijados.
func TestTopePorPersona(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)

	t.Run("veinte a la vez desde organizaciones distintas no pasan de cinco", func(t *testing.T) {
		svc := notify.NewService(st, notify.NewHub(1)).ConTopePersona(5)
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				_, _ = svc.Create(tenantCtx(fmt.Sprintf("org%d", i)), sampleNotification("hugo", "uno"))
			})
		}
		wg.Wait()
		var n int
		if err := h.Pool.InTx(context.Background(), func(tx pgx.Tx) error {
			if _, e := tx.Exec(context.Background(), "SELECT set_config('app.maintenance', 'on', true)"); e != nil {
				return e
			}
			return tx.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE subject = 'hugo'`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		if n != 5 {
			t.Fatalf("entraron %d avisos a la vez, quiero el tope: 5", n)
		}
	})

	t.Run("contar deja los buzones que la transacción tenía", func(t *testing.T) {
		ctx := tenantCtx("acme")
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, "SELECT set_config('app.destinatarios', 'ana', true)"); e != nil {
				return e
			}
			if _, e := store.DeLaPersona(ctx, tx, "otra", false); e != nil {
				return e
			}
			var v string
			if e := tx.QueryRow(ctx, "SELECT current_setting('app.destinatarios', true)").Scan(&v); e != nil {
				return e
			}
			if v != "ana" {
				t.Errorf("tras contar, app.destinatarios = %q, quiero el de antes: ana", v)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
