// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// El historial lo escribe la base: crear, cambiar campos y estado, papelera y
// vuelta, firmado por quien actúa; las notas se añaden y nada de otra
// organización se ve.
func TestHistorialDeUnRegistro(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := authn.Into(tenantCtx("acme"), authn.Claims{Subject: "ada"})
	if err := st.VerUsuario(ctx, "ada", "ada@acme.es", "Ada Lovelace", []string{"owner"}); err != nil {
		t.Fatal(err)
	}
	contacto := modelo(t, c, "contacto")
	reg := crear(ctx, t, st, contacto, `{"nombre":"Acme","tipo":"organizacion","ciudad":"Madrid"}`)

	datos := map[string]any{"nombre": "Acme Ibérica", "tipo": "organizacion", "ciudad": "Madrid", "nif": "B12345674"}
	if _, err := st.Actualizar(ctx, contacto, reg.ID, datos, "Acme Ibérica"); err != nil {
		t.Fatal(err)
	}
	// Guardar sin cambiar nada no deja rastro.
	if _, err := st.Actualizar(ctx, contacto, reg.ID, datos, "Acme Ibérica"); err != nil {
		t.Fatal(err)
	}
	if err := st.Borrar(ctx, "contacto", reg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Restaurar(ctx, "contacto", reg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Anotar(ctx, "contacto", reg.ID, "ada", "  Llamar el lunes  "); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Anotar(ctx, "contacto", reg.ID, "ada", "   "); !errors.Is(err, store.ErrNotaVacia) {
		t.Fatalf("nota vacía: %v", err)
	}
	if _, err := st.Anotar(ctx, "factura_venta", reg.ID, "ada", "otra"); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("una nota en un registro de otro modelo: %v", err)
	}

	entradas, err := st.Historial(ctx, "contacto", reg.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	tipos := make([]string, 0, len(entradas))
	for _, e := range entradas {
		tipos = append(tipos, e.Tipo)
		if e.Sub != "ada" || e.Nombre != "Ada Lovelace" {
			t.Errorf("%s firmado por %q (%q)", e.Tipo, e.Sub, e.Nombre)
		}
	}
	if len(tipos) != 5 || tipos[0] != "nota" || tipos[1] != "restaurado" || tipos[2] != "borrado" || tipos[3] != "cambio" || tipos[4] != "creado" {
		t.Fatalf("historial: %v", tipos)
	}
	cambio := entradas[3].Cambios
	if len(cambio) != 2 || cambio["nombre"].Antes != "Acme" || cambio["nombre"].Despues != "Acme Ibérica" ||
		cambio["nif"].Antes != nil || cambio["nif"].Despues != "B12345674" {
		t.Fatalf("cambios: %+v", cambio)
	}
	if entradas[0].Texto != "Llamar el lunes" {
		t.Fatalf("nota: %q", entradas[0].Texto)
	}

	// El estado va como un campo más.
	factura, fv := facturaDePrueba(ctx, t, st, c)
	if _, err := st.Asentar(ctx, factura, fv.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	entradas, _ = st.Historial(ctx, "factura_venta", fv.ID, 0)
	if len(entradas) != 2 || entradas[0].Cambios["estado"].Antes != "borrador" || entradas[0].Cambios["estado"].Despues != "confirmado" {
		t.Fatalf("estado: %+v", entradas)
	}

	// Otra organización no ve nada.
	if otra, err := st.Historial(tenantCtx("otra"), "contacto", reg.ID, 0); err != nil || len(otra) != 0 {
		t.Fatalf("otra organización: %v %v", otra, err)
	}
}
