// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La cuota del plan es UNA para toda la suite (CAST-ENCARGO §8, mig 0026): lo que
// Sytue Cast declara por la puerta interna cuenta en used_bytes, en el hueco que
// queda y en la reserva que hace Drive al subir. Contra un PostgreSQL real, con el
// rol de aplicación (la RLS se evalúa de verdad).
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestCuotaSumaLoDeclaradoPorOtrasApps(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	q, err := st.EnsureQuota(ctx, 1000)
	if err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	if q.UsedBytes != 0 || len(q.ExternalUsage) != 0 {
		t.Fatalf("cuota recién creada: %+v", q)
	}

	// Cast declara 300: el total los cuenta y el desglose los atribuye.
	if err := st.SetExternalUsage(ctx, "cast", 300); err != nil {
		t.Fatalf("SetExternalUsage: %v", err)
	}
	q, err = st.GetQuota(ctx)
	if err != nil {
		t.Fatalf("GetQuota: %v", err)
	}
	if q.UsedBytes != 300 || q.ExternalUsage["cast"] != 300 || q.DriveBytes() != 0 {
		t.Fatalf("tras declarar 300: %+v", q)
	}

	// Drive sube 200: el total es la suma de los dos y el desglose sigue separado.
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	seedFile(t, ctx, st, root.ID, "a.txt", 200)
	q, _ = st.GetQuota(ctx)
	if q.UsedBytes != 500 || q.DriveBytes() != 200 || q.ExternalBytes() != 300 {
		t.Fatalf("Drive 200 + Cast 300: %+v", q)
	}

	// Valor ABSOLUTO e idempotente: dos veces 100 son 100, no 200 ni 400.
	for range 2 {
		if err := st.SetExternalUsage(ctx, "cast", 100); err != nil {
			t.Fatalf("SetExternalUsage: %v", err)
		}
	}
	q, _ = st.GetQuota(ctx)
	if q.UsedBytes != 300 || q.ExternalUsage["cast"] != 100 {
		t.Fatalf("tras fijar 100 dos veces: %+v", q)
	}

	// La reserva de Drive respeta lo de Cast: quedan 700 de 1000, así que 800 no
	// caben aunque Drive solo tenga 200.
	_, err = st.CreateFile(ctx, store.NewFileRecord{FolderID: root.ID, Name: "grande.bin", StorageKey: "files/grande", SizeBytes: 800, ContentType: "application/octet-stream"})
	if !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("800 con 700 libres (contando Cast) => %v, quiero ErrQuotaExceeded", err)
	}
	seedFile(t, ctx, st, root.ID, "justo.bin", 700)

	// Los otros caminos que devuelven la cuota componen el mismo total.
	if q, err = st.SetQuotaLimit(ctx, 5000); err != nil || q.LimitBytes != 5000 || q.UsedBytes != 1000 {
		t.Fatalf("SetQuotaLimit: %+v %v", q, err)
	}
	if _, q, err = st.EnsureBase(ctx, 5000, false); err != nil || q.UsedBytes != 1000 || q.ExternalUsage["cast"] != 100 {
		t.Fatalf("EnsureBase: %+v %v", q, err)
	}

	// Otra organización no hereda nada, ni por el store ni por SQL crudo bajo su
	// tenant: la RLS de la tabla nueva aísla la fila de acme.
	beta := tenantCtx("beta")
	if q, err := st.EnsureQuota(beta, 1000); err != nil || q.UsedBytes != 0 || len(q.ExternalUsage) != 0 {
		t.Fatalf("beta ve uso ajeno: %+v %v", q, err)
	}
	var filas int
	err = h.Pool.InTx(context.Background(), func(tx pgx.Tx) error {
		if err := pg.SetTenant(context.Background(), tx, "beta"); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM quota_external_usage`).Scan(&filas)
	})
	if err != nil || filas != 0 {
		t.Fatalf("bajo app.tenant=beta se ven %d filas (%v), quiero 0", filas, err)
	}
}
