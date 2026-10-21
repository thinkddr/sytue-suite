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
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestElUmbralNoSeSaltaCambiandoElEstado (NEG-11): emitir una factura cambiando
// su estado es lo mismo que asentarla, y pide la misma segunda persona.
func TestElUmbralNoSeSaltaCambiandoElEstado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	factura := modelo(t, c, "factura_venta")
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Suministros Acme S.L.","tipo":"organizacion"}`)
	if err := st.GuardarUmbral(ctx, factura.Modulo, 100000, "EUR", "jefa"); err != nil {
		t.Fatal(err)
	}
	grande := func() store.Registro {
		return crear(ctx, t, st, factura, fmt.Sprintf(
			`{"cliente":%q,"fecha":"2026-09-12","base":200000,"iva":42000,"total":242000}`, cliente.ID))
	}

	uno, dos := grande(), grande()
	if _, err := st.CambiarEstado(ctx, factura, uno.ID, "confirmado", "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("confirmar por /estado por encima del umbral: %v, quiero ErrRequiereAprobacion", err)
	}
	if _, err := st.CambiarEstado(ctx, factura, uno.ID, "pagado", "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("darla por pagada sin pasar por confirmada: %v, quiero ErrRequiereAprobacion", err)
	}
	if _, err := st.LoteEstado(ctx, factura, []string{uno.ID, dos.ID}, "confirmado", "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("confirmar por /lote: %v, quiero ErrRequiereAprobacion", err)
	}
	if reg, _ := st.Obtener(ctx, "factura_venta", uno.ID); reg.Estado != "borrador" {
		t.Fatalf("la factura no se movió: %s", reg.Estado)
	}

	// Con la aprobación de otra persona, sí.
	sol, err := st.PedirAprobacion(ctx, factura, uno.ID, 242000, "EUR", "ada")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResolverAprobacion(ctx, sol.ID, "aprobada", "", "jefa"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, factura, uno.ID, "confirmado", "ada"); err != nil {
		t.Fatalf("confirmar con la aprobación dada: %v", err)
	}
	// Y una vez emitida, pasarla a pagada no vuelve a pedir nada.
	if _, err := st.CambiarEstado(ctx, factura, uno.ID, "pagado", "ada"); err != nil {
		t.Fatalf("de confirmada a pagada: %v", err)
	}
}

// TestLaAprobacionAtaElDocumentoEntero (NEG-13): con el mismo total se cambiaba
// el cliente después de aprobar y la factura se asentaba igual.
func TestLaAprobacionAtaElDocumentoEntero(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	factura := modelo(t, c, "factura_venta")
	contacto := modelo(t, c, "contacto")
	bueno := crear(ctx, t, st, contacto, `{"nombre":"Cliente de verdad","tipo":"organizacion"}`)
	otro := crear(ctx, t, st, contacto, `{"nombre":"Sociedad pantalla","tipo":"organizacion"}`)
	if err := st.GuardarUmbral(ctx, factura.Modulo, 100000, "EUR", "jefa"); err != nil {
		t.Fatal(err)
	}
	fv := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-12","base":200000,"iva":42000,"total":242000}`, bueno.ID))
	sol, err := st.PedirAprobacion(ctx, factura, fv.ID, 242000, "EUR", "ada")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResolverAprobacion(ctx, sol.ID, "aprobada", "", "jefa"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Actualizar(ctx, factura, fv.ID, map[string]any{
		"cliente": otro.ID, "fecha": "2026-09-12",
		"base": int64(200000), "iva": int64(42000), "total": int64(242000),
	}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Asentar(ctx, factura, fv.ID, "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("mismo importe, otro cliente: %v, quiero ErrRequiereAprobacion", err)
	}
}

// TestElBancoRespetaElPeriodoCerrado (NEG-12): justificar un movimiento escribe
// un asiento, y en un mes cerrado no se escribe nada, ni se anula al reabrir.
func TestElBancoRespetaElPeriodoCerrado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	dia := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	nuevos, err := st.ImportarExtracto(ctx, store.ExtractoBanco{
		CuentaExterno: "n43:210000010123456789", CuentaNombre: "CaixaBank ··6789",
		Externo: "n43:210000010123456789:20260901-20260930", Hasta: dia, SaldoFinal: -1000,
		Movimientos: []store.MovimientoBanco{
			{Externo: "n43:210000010123456789:a", Fecha: dia, Descripcion: "COMISION", Centimos: -500},
			{Externo: "n43:210000010123456789:b", Fecha: dia, Descripcion: "COMISION", Centimos: -500},
		},
	}, "ada")
	if err != nil || len(nuevos) != 2 {
		t.Fatalf("importar: %v %v", nuevos, err)
	}
	hecho := store.Liquidacion{LineaID: nuevos[0], Motivo: "comision_bancaria", Contrapartida: "626000", Concepto: "Comisión"}
	if _, err := st.Liquidar(ctx, hecho, "ada"); err != nil {
		t.Fatal(err)
	}
	if err := st.Cerrar(ctx, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), "tercer trimestre", "jefa"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Liquidar(ctx, store.Liquidacion{LineaID: nuevos[1], Motivo: "comision_bancaria", Contrapartida: "626000", Concepto: "Comisión"}, "ada"); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("justificar en un mes cerrado: %v, quiero ErrPeriodoCerrado", err)
	}
	if err := st.Reabrir(ctx, nuevos[0]); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("reabrir en un mes cerrado: %v, quiero ErrPeriodoCerrado", err)
	}
}
