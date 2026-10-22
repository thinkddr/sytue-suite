// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestLasSubidasPendientesOcupanSitio (FILE-08): una URL de subida sin finalizar
// cuenta contra la cuota al pedir la siguiente, hay un tope de pendientes, lo
// finalizado deja de contar, y lo caducado lo recoge el recolector (de todas las
// organizaciones) para borrar su objeto.
func TestLasSubidasPendientesOcupanSitio(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 100); err != nil {
		t.Fatal(err)
	}
	luego := time.Now().Add(time.Hour)
	if err := st.ReservarSubida(ctx, "files/a", 60, luego, true); err != nil {
		t.Fatal(err)
	}
	if err := st.ReservarSubida(ctx, "files/b", 60, luego, true); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("60 pendientes + 60 en una cuota de 100: %v, quiero ErrQuotaExceeded", err)
	}
	if err := st.ReservarSubida(ctx, "externo/b", 60, luego, false); err != nil {
		t.Fatalf("al bucket propio no le aplica la cuota de Sytue: %v", err)
	}
	if err := st.SubidaFinalizada(ctx, "files/a"); err != nil {
		t.Fatal(err)
	}
	if err := st.SubidaFinalizada(ctx, "files/a"); !errors.Is(err, store.ErrSubidaYaFinalizada) {
		t.Fatalf("segunda SubidaFinalizada sobre la misma reserva: %v, quiero ErrSubidaYaFinalizada", err)
	}
	if err := st.ReservarSubida(ctx, "files/c", 60, luego, true); err != nil {
		t.Fatalf("finalizada, la reserva deja de contar (cuenta ya el fichero): %v", err)
	}
	for i := 2; i < store.MaxSubidasPendientes; i++ { // con externo/b y files/c, justo el tope
		if err := st.ReservarSubida(ctx, fmt.Sprintf("externo/%d", i), 1, luego, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.ReservarSubida(ctx, "externo/una-mas", 1, luego, false); !errors.Is(err, store.ErrDemasiadasSubidas) {
		t.Fatalf("pasado el tope: %v, quiero ErrDemasiadasSubidas", err)
	}

	if err := st.ReservarSubida(tenantCtx("globex"), "files/x", 1, time.Now().Add(-time.Minute), false); err != nil {
		t.Fatal(err)
	}
	caducadas, err := st.ReclamarSubidasCaducadas(ctx, time.Now(), 50)
	if err != nil || len(caducadas) != 1 || caducadas[0].Tenant != "globex" || caducadas[0].StorageKey != "files/x" {
		t.Fatalf("caducadas: %+v %v", caducadas, err)
	}
	if otra, _ := st.ReclamarSubidasCaducadas(ctx, time.Now(), 50); len(otra) != 0 {
		t.Fatalf("cada caducada se reclama una vez: %+v", otra)
	}
}
