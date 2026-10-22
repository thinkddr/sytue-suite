// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// El banco de punta a punta contra Postgres: importar dos veces no duplica, casar
// con la factura escribe el cobro y la marca pagada, deshacer lo revierte con un
// asiento anulado, y lo que falta cuenta por mes.
func TestBancoImportarCasarYDeshacer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	factura, fv := facturaDePrueba(ctx, t, st, c)
	if _, err := st.Asentar(ctx, factura, fv.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	dia := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	extracto := store.ExtractoBanco{
		CuentaExterno: "n43:210000010123456789", CuentaNombre: "CaixaBank ··6789",
		Externo: "n43:210000010123456789:20260901-20260930", Hasta: dia, SaldoFinal: 121000,
		Movimientos: []store.MovimientoBanco{
			{Externo: "n43:210000010123456789:a", Fecha: dia, Descripcion: "TRANSF SUMINISTROS ACME", Centimos: 121000},
			{Externo: "n43:210000010123456789:b", Fecha: dia, Descripcion: "COMISION MANTENIMIENTO", Centimos: -500},
		},
	}
	nuevos, err := st.ImportarExtracto(ctx, extracto, "ada")
	if err != nil || len(nuevos) != 2 {
		t.Fatalf("primera importación: %v %v", nuevos, err)
	}
	if otra, err := st.ImportarExtracto(ctx, extracto, "ada"); err != nil || len(otra) != 0 {
		t.Fatalf("la segunda importación del mismo extracto no crea nada: %v %v", otra, err)
	}

	pend, err := st.Pendientes(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil || pend["banco"]["2026-09"] != 2 || pend["facturas"]["2026-09"] != 0 {
		t.Fatalf("pendientes: %v %v", pend, err)
	}

	cobro, comision := nuevos[0], nuevos[1]
	asiento, err := st.Liquidar(ctx, store.Liquidacion{
		LineaID: cobro, FacturaID: fv.ID, ModeloFactura: "factura_venta", Contrapartida: "430000", Concepto: "Cobro " + fv.Ref,
	}, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if asiento.Apuntes[0].Cuenta != "572001" || asiento.Apuntes[0].Debe != 121000 || asiento.Apuntes[1].Haber != 121000 {
		t.Fatalf("el cobro entra en la cuenta de ese banco (572001) contra el cliente: %+v", asiento.Apuntes)
	}
	if reg, _ := st.Obtener(ctx, "factura_venta", fv.ID); reg.Estado != "pagado" {
		t.Fatalf("la factura queda pagada: %s", reg.Estado)
	}
	if _, err := st.Liquidar(ctx, store.Liquidacion{LineaID: cobro, Motivo: "otro", Contrapartida: "555000"}, "ada"); !errors.Is(err, store.ErrYaJustificado) {
		t.Fatalf("un movimiento justificado no se justifica dos veces: %v", err)
	}
	if _, err := st.Liquidar(ctx, store.Liquidacion{LineaID: comision, Motivo: "comision_bancaria", Contrapartida: "626000", Concepto: "Comisión"}, "ada"); err != nil {
		t.Fatal(err)
	}

	hechas, total, err := st.LineasBanco(ctx, "conciliado", 20)
	if err != nil || total != 2 || len(hechas) != 2 {
		t.Fatalf("justificadas: %v %d %v", hechas, total, err)
	}

	if err := st.Reabrir(ctx, cobro); err != nil {
		t.Fatal(err)
	}
	if reg, _ := st.Obtener(ctx, "factura_venta", fv.ID); reg.Estado != "confirmado" {
		t.Fatalf("deshacer devuelve la factura a pendiente de cobro: %s", reg.Estado)
	}
	saldos, err := st.Balance(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range saldos {
		if s.Cuenta == "430000" && s.Saldo != 121000 {
			t.Fatalf("anulado el cobro, el cliente vuelve a deber la factura: %+v", s)
		}
	}
	pendientes, total, err := st.LineasBanco(ctx, "pendiente", 20)
	if err != nil || total != 1 || pendientes[0].ID != cobro {
		t.Fatalf("el movimiento vuelve a ser tarea: %v %d %v", pendientes, total, err)
	}
	// Y se puede volver a casar: el asiento anulado no bloquea el índice.
	if _, err := st.Liquidar(ctx, store.Liquidacion{
		LineaID: cobro, FacturaID: fv.ID, ModeloFactura: "factura_venta", Contrapartida: "430000", Concepto: "Cobro",
	}, "ada"); err != nil {
		t.Fatal(err)
	}
}

// TestElCobroDeStripeNoSePierdeNiSeDuplica (NEG-25): un cobro que falla deja el
// evento sin consumir para el reintento de Stripe; el mismo evento no cobra dos
// veces; ni se cobra una factura ya pagada ni por otro importe.
func TestElCobroDeStripeNoSePierdeNiSeDuplica(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	_, fv := facturaDePrueba(ctx, t, st, cat(t))
	cobrar := func(evento string, centimos int64) error {
		_, err := st.RegistrarCobro(ctx, fv.ID, evento, "payment_intent.succeeded", "572000", "430000", "banco",
			"Cobro · Stripe", centimos, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), "stripe")
		return err
	}
	total, err := domain.Centimos(fv.Data, "total")
	if err != nil {
		t.Fatal(err)
	}
	if err := cobrar("evt_1", total-1); !errors.Is(err, store.ErrImporteNoCasa) {
		t.Fatalf("otro importe: %v, quiero ErrImporteNoCasa", err)
	}
	if err := cobrar("evt_1", total); err != nil {
		t.Fatalf("el reintento del mismo evento, tras fallar, cobra: %v", err)
	}
	if err := cobrar("evt_1", total); !errors.Is(err, store.ErrEventoRepetido) {
		t.Fatalf("el mismo evento otra vez: %v, quiero ErrEventoRepetido", err)
	}
	if err := cobrar("evt_2", total); !errors.Is(err, store.ErrFacturaNoAbierta) {
		t.Fatalf("una factura ya pagada: %v, quiero ErrFacturaNoAbierta", err)
	}
}
