// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de las ALERTAS DE COSTE contra un PostgreSQL real.
//
// Aquí no se prueba lógica de Go: se prueba la GARANTÍA que sostiene la función y
// que solo la base de datos puede dar. El evaluador corre en todas las réplicas de
// cloud-api a la vez, así que lo único que impide enviar el mismo aviso N veces es
// la clave primaria de billing_spend_alerts. Si esa exclusión no fuese atómica, el
// cliente recibiría un webhook por réplica y nadie se daría cuenta hasta producción.
package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
)

func TestSpendAlertDedupeIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := New(h.Pool)
	ctx := context.Background()
	period := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	t.Run("solo el primero avisa; los siguientes se callan", func(t *testing.T) {
		first, err := st.PublishSpendAlert(ctx, "acme", period, 80, 81, 100)
		if err != nil {
			t.Fatalf("PublishSpendAlert: %v", err)
		}
		if !first {
			t.Fatal("el primer aviso del periodo debe enviarse")
		}
		again, err := st.PublishSpendAlert(ctx, "acme", period, 80, 95, 100)
		if err != nil {
			t.Fatalf("PublishSpendAlert (2.ª): %v", err)
		}
		if again {
			t.Error("el mismo umbral no puede avisar dos veces en el mismo periodo")
		}
	})

	t.Run("umbrales distintos avisan por separado", func(t *testing.T) {
		sent, err := st.PublishSpendAlert(ctx, "acme", period, 100, 120, 100)
		if err != nil {
			t.Fatalf("PublishSpendAlert(100): %v", err)
		}
		if !sent {
			t.Error("cruzar el 100 % debe avisar aunque ya se avisara del 80 %")
		}
	})

	t.Run("otro periodo vuelve a avisar (reinicio automático)", func(t *testing.T) {
		next := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
		sent, err := st.PublishSpendAlert(ctx, "acme", next, 80, 81, 100)
		if err != nil {
			t.Fatalf("PublishSpendAlert(mes siguiente): %v", err)
		}
		if !sent {
			t.Error("al cambiar de mes el aviso debe volver a estar disponible")
		}
	})

	t.Run("cada organización tiene su propio contador", func(t *testing.T) {
		sent, err := st.PublishSpendAlert(ctx, "otra", period, 80, 81, 100)
		if err != nil {
			t.Fatalf("PublishSpendAlert(otro tenant): %v", err)
		}
		if !sent {
			t.Error("el aviso de un tenant no puede consumir el de otro")
		}
	})

	// EL CASO QUE IMPORTA: N réplicas evaluando a la vez. Sin la atomicidad del
	// INSERT ... ON CONFLICT, varias verían «no avisado» y enviarían todas.
	t.Run("con 8 réplicas simultáneas solo una envía", func(t *testing.T) {
		const replicas = 8
		var wg sync.WaitGroup
		results := make([]bool, replicas)
		errs := make([]error, replicas)
		start := make(chan struct{})
		for i := range replicas {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start // soltar todas a la vez, para que compitan de verdad
				results[i], errs[i] = st.PublishSpendAlert(ctx, "carrera", period, 80, 81, 100)
			}(i)
		}
		close(start)
		wg.Wait()

		sent := 0
		for i := range replicas {
			if errs[i] != nil {
				t.Fatalf("réplica %d: %v", i, errs[i])
			}
			if results[i] {
				sent++
			}
		}
		if sent != 1 {
			t.Errorf("enviaron %d réplicas de %d; debe enviar exactamente UNA", sent, replicas)
		}
	})

	// La publicación al outbox comparte transacción con la reserva: si no fuera
	// así, quedaría un aviso «reservado y nunca enviado» que se come el único del
	// mes. Se comprueba que por cada reserva hay exactamente un evento en el bus.
	t.Run("cada aviso reservado deja su evento en el outbox", func(t *testing.T) {
		var reservas, eventos int
		// Se cuenta DENTRO de una transacción de mantenimiento a propósito: sin ese
		// flag la RLS de la tabla no deja ver ni una fila (deny-by-default), que es
		// justo lo que debe pasar. Leerlo así comprueba de paso que la política de
		// mantenimiento de la migración 0022 concede exactamente lo que promete.
		if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM billing_spend_alerts`).Scan(&reservas)
		}); err != nil {
			t.Fatalf("contar reservas: %v", err)
		}
		if err := h.Pool.QueryRow(ctx,
			`SELECT count(*) FROM events_outbox WHERE type=$1`, EventSpendThreshold).Scan(&eventos); err != nil {
			t.Fatalf("contar eventos: %v", err)
		}
		if reservas != eventos {
			t.Errorf("%d reservas frente a %d eventos: la reserva y la publicación deben ser atómicas",
				reservas, eventos)
		}
	})
}
