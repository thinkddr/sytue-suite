// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Rate-limit de fallos de autenticación (submission 587/465 e IMAP 993) contra
// un PostgreSQL real. Lo que se verifica aquí es lo que decide si un ataque de
// diccionario se frena: que consultar NO incremente, que incrementar cuente, que
// dos IP no se estorben, y que la purga no se lleve la ventana en curso.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestAuthFailRate(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	// Sin tenant a propósito: auth_fail_rate no lleva RLS, igual que
	// inbound_rate. Se consulta ANTES de saber quién dice ser el cliente.
	ctx := context.Background()

	// now() pertenece a PostgreSQL, no al reloj de Go. El harness tiene su base
	// aislada: un reloj de fixture allí permite probar el cambio de minuto sin
	// carreras con el reloj real (el run 37120448999 cruzó justo ese límite).
	admin, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	if _, err := admin.Exec(ctx, `
		CREATE TABLE reloj_auth_fail_prueba (hora timestamptz NOT NULL);
		INSERT INTO reloj_auth_fail_prueba VALUES ('2026-01-01 12:00:30+00');
		CREATE FUNCTION public.now() RETURNS timestamptz LANGUAGE sql STABLE AS
		$$ SELECT hora FROM public.reloj_auth_fail_prueba $$`); err != nil {
		t.Fatal(err)
	}
	cfg := h.Pool.Config()
	// pg_catalog explícito y después del fixture: de otro modo now() nativo gana.
	cfg.ConnConfig.RuntimeParams["search_path"] = "public, pg_catalog"
	conReloj, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conReloj.Close)
	st := store.New(&pg.Pool{Pool: conReloj})

	const atacante = "203.0.113.7"
	const inocente = "198.51.100.20"

	// Una IP sin fallos empieza en cero.
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 0 {
		t.Fatalf("AuthFailRate inicial = (%d, %v), quiero (0, nil)", n, err)
	}

	// CONSULTAR NO INCREMENTA. Si lo hiciera, el propio control de acceso
	// bloquearía a un cliente que nunca ha fallado.
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 0 {
		t.Fatalf("AuthFailRate tras consultar = (%d, %v), quiero (0, nil)", n, err)
	}

	// Incrementar cuenta, y devuelve el recuento resultante.
	for quiero := 1; quiero <= 3; quiero++ {
		n, err := st.HitAuthFailRate(ctx, atacante)
		if err != nil {
			t.Fatalf("HitAuthFailRate #%d: %v", quiero, err)
		}
		if n != quiero {
			t.Fatalf("HitAuthFailRate #%d = %d, quiero %d", quiero, n, quiero)
		}
	}

	// Y la consulta ve lo mismo que el último incremento.
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 3 {
		t.Fatalf("AuthFailRate tras 3 fallos = (%d, %v), quiero (3, nil)", n, err)
	}

	// AISLAMIENTO POR IP. Sin esto, un atacante bloquearía a todos los demás
	// clientes con solo fallar lo suficiente: una denegación de servicio
	// disfrazada de medida de seguridad.
	if n, err := st.AuthFailRate(ctx, inocente); err != nil || n != 0 {
		t.Fatalf("AuthFailRate de otra IP = (%d, %v), quiero (0, nil)", n, err)
	}

	// La purga NO se lleva la ventana en curso: si lo hiciera, bastaría con
	// esperar a que pasara el recolector para reiniciar el contador.
	if err := st.GCAuthFailRate(ctx, 10*time.Minute); err != nil {
		t.Fatalf("GCAuthFailRate(10m): %v", err)
	}
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 3 {
		t.Fatalf("AuthFailRate tras purgar ventanas viejas = (%d, %v), quiero (3, nil)", n, err)
	}

	// El minuto siguiente reinicia la ventana aunque la anterior siga en la tabla.
	if _, err := admin.Exec(ctx, `UPDATE reloj_auth_fail_prueba SET hora = '2026-01-01 12:01:30+00'`); err != nil {
		t.Fatal(err)
	}
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 0 {
		t.Fatalf("AuthFailRate en el minuto siguiente = (%d, %v), quiero (0, nil)", n, err)
	}
	if n, err := st.HitAuthFailRate(ctx, atacante); err != nil || n != 1 {
		t.Fatalf("HitAuthFailRate en el minuto siguiente = (%d, %v), quiero (1, nil)", n, err)
	}

	// Y con umbral cero sí se lleva todo lo anterior a este instante.
	if err := st.GCAuthFailRate(ctx, 0); err != nil {
		t.Fatalf("GCAuthFailRate(0): %v", err)
	}
	if n, err := st.AuthFailRate(ctx, atacante); err != nil || n != 0 {
		t.Fatalf("AuthFailRate tras purgar todo = (%d, %v), quiero (0, nil)", n, err)
	}
}
