// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Aprobaciones con umbral y cuatro ojos (migración 0095), contra PostgreSQL real.
package store_test

import (
	"errors"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestAprobacionPorUmbralYCuatroOjos vigila lo que un verbo no puede decir: que
// a partir de cierto importe hace falta una SEGUNDA persona, y que esa segunda
// persona no puede ser la primera.
func TestAprobacionPorUmbralYCuatroOjos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	factura := modelo(t, c, "factura_venta")
	contacto := modelo(t, c, "contacto")
	cliente := crear(ctx, t, st, contacto, `{"nombre":"Suministros Acme S.L.","tipo":"organizacion"}`)

	nueva := func(base, iva int64) store.Registro {
		t.Helper()
		return crear(ctx, t, st, factura, fmt.Sprintf(
			`{"cliente":%q,"fecha":"2026-09-12","base":%d,"iva":%d,"total":%d}`,
			cliente.ID, base, iva, base+iva))
	}

	// Umbral: 1.000,00 € en facturación, en CÉNTIMOS enteros.
	if err := st.GuardarUmbral(ctx, factura.Modulo, 100000, "EUR", "jefa"); err != nil {
		t.Fatalf("GuardarUmbral: %v", err)
	}

	// Por debajo del umbral: se asienta sin que nadie mire. Cerrar el paso a lo
	// pequeño convertiría la aprobación en un trámite que se firma sin leer.
	pequena := nueva(50000, 10500) // 605,00 €
	if _, err := st.Asentar(ctx, factura, pequena.ID, "ada"); err != nil {
		t.Fatalf("por debajo del umbral tiene que pasar: %v", err)
	}

	// Por encima: no pasa.
	grande := nueva(200000, 42000) // 2.420,00 €
	if _, err := st.Asentar(ctx, factura, grande.ID, "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("por encima del umbral: %v, quiero ErrRequiereAprobacion", err)
	}

	// Se pide la aprobación, con el importe congelado tal como iría al libro.
	sol, err := st.PedirAprobacion(ctx, factura, grande.ID, 242000, "EUR", "ada")
	if err != nil {
		t.Fatalf("PedirAprobacion: %v", err)
	}
	if sol.Importe != 242000 || sol.Moneda != "EUR" || sol.Estado != "pendiente" {
		t.Fatalf("solicitud: %+v", sol)
	}

	// CUATRO OJOS: ni quien la pidió ni quien creó el documento la aprueban.
	// `crear` pone owner_sub = "ada", que es también quien la pidió.
	if _, err := st.ResolverAprobacion(ctx, sol.ID, "aprobada", "", "ada"); !errors.Is(err, store.ErrCuatroOjos) {
		t.Fatalf("aprobarse a uno mismo: %v, quiero ErrCuatroOjos", err)
	}
	// Y sigue sin poder asentarse mientras está pendiente.
	if _, err := st.Asentar(ctx, factura, grande.ID, "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("con la aprobación pendiente: %v, quiero ErrRequiereAprobacion", err)
	}

	// Otra persona sí.
	if _, err := st.ResolverAprobacion(ctx, sol.ID, "aprobada", "revisada", "jefa"); err != nil {
		t.Fatalf("ResolverAprobacion: %v", err)
	}
	if _, err := st.ResolverAprobacion(ctx, sol.ID, "rechazada", "ya no", "jefa"); !errors.Is(err, store.ErrAprobacionResuelta) {
		t.Fatalf("resolver dos veces: %v, quiero ErrAprobacionResuelta", err)
	}
	if _, err := st.Asentar(ctx, factura, grande.ID, "ada"); err != nil {
		t.Fatalf("con la aprobación dada tiene que asentarse: %v", err)
	}

	// UNA APROBACIÓN VALE PARA SU IMPORTE. Aprobar barato y editar después no
	// cuela: la aprobación deja de corresponderse con lo que hay.
	otra := nueva(300000, 63000) // 3.630,00 €
	sol2, err := st.PedirAprobacion(ctx, factura, otra.ID, 363000, "EUR", "ada")
	if err != nil {
		t.Fatalf("PedirAprobacion (2): %v", err)
	}
	if _, err := st.ResolverAprobacion(ctx, sol2.ID, "aprobada", "", "jefa"); err != nil {
		t.Fatalf("ResolverAprobacion (2): %v", err)
	}
	if _, err := st.Actualizar(ctx, factura, otra.ID, map[string]any{
		"cliente": cliente.ID, "fecha": "2026-09-12",
		"base": int64(900000), "iva": int64(189000), "total": int64(1089000),
	}, ""); err != nil {
		t.Fatalf("Actualizar: %v", err)
	}
	if _, err := st.Asentar(ctx, factura, otra.ID, "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("editar después de aprobar: %v, quiero ErrRequiereAprobacion", err)
	}

	// OTRA DIVISA no se convierte y no se deja pasar: no hay fuente de tipos.
	if err := st.GuardarUmbral(ctx, factura.Modulo, 100000, "USD", "jefa"); err != nil {
		t.Fatalf("GuardarUmbral (USD): %v", err)
	}
	minima := nueva(100, 21) // 1,21 €: muy por debajo de 1.000 de cualquier cosa
	if domain.MonedaPorDefecto == "USD" {
		t.Skip("la divisa por defecto coincide con la del umbral; este caso no aplica")
	}
	if _, err := st.Asentar(ctx, factura, minima.ID, "ada"); !errors.Is(err, store.ErrRequiereAprobacion) {
		t.Fatalf("umbral en otra divisa: %v, quiero ErrRequiereAprobacion (no se convierte)", err)
	}
}
