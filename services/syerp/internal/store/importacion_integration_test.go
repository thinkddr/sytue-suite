// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Importación reversible (migración 0093), contra PostgreSQL real.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestImportacionSeSimulaYSeRevierte vigila las cuatro cosas por las que esto
// existe: que simular NO escribe, que importar deja un lote identificable, que
// revertir deshace tanto lo creado como lo actualizado, y —la que de verdad
// importa— que NO pisa lo que alguien tocó después.
func TestImportacionSeSimulaYSeRevierte(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	m := modelo(t, cat(t), "contacto")

	fila := func(nombre, externo string) store.FilaImportada {
		return store.FilaImportada{
			Linea: 2, Etiqueta: nombre, Externo: externo,
			Data: map[string]any{"nombre": nombre, "tipo": "organizacion", "externo_id": externo},
		}
	}
	buscar := func(etiqueta string) string {
		t.Helper()
		p, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Limite: 50})
		if err != nil {
			t.Fatalf("Listar: %v", err)
		}
		for _, r := range p.Registros {
			if r.Etiqueta == etiqueta {
				return r.ID
			}
		}
		return ""
	}
	cuantos := func() int {
		t.Helper()
		p, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Limite: 50})
		if err != nil {
			t.Fatalf("Listar: %v", err)
		}
		return len(p.Registros)
	}

	// 1. SIMULAR no escribe nada, y aun así cuenta lo que pasaría.
	sim, err := st.SimularImportacion(ctx, m, []store.FilaImportada{fila("Acme SL", "ext-1")}, "ana")
	if err != nil {
		t.Fatalf("SimularImportacion: %v", err)
	}
	if sim.Creados != 1 || sim.ID != "" {
		t.Fatalf("simulación: %+v, quiero 1 creado y sin id de lote", sim)
	}
	if n := cuantos(); n != 0 {
		t.Fatalf("una simulación NO escribe: quedaron %d registros", n)
	}
	if lotes, _ := st.Importaciones(ctx, 10); len(lotes) != 0 {
		t.Fatalf("una simulación no deja lote: %+v", lotes)
	}

	// 2. Importación de verdad: dos contactos nuevos, y un lote con su nombre.
	uno, err := st.Importar(ctx, m,
		[]store.FilaImportada{fila("Acme SL", "ext-1"), fila("Beta SA", "ext-2")}, "clientes.csv", "ana")
	if err != nil {
		t.Fatalf("Importar: %v", err)
	}
	if uno.Creados != 2 || uno.Actualizados != 0 || uno.ID == "" {
		t.Fatalf("primera importación: %+v", uno)
	}
	lotes, err := st.Importaciones(ctx, 10)
	if err != nil || len(lotes) != 1 || lotes[0].Fichero != "clientes.csv" || lotes[0].Creados != 2 {
		t.Fatalf("el lote no se puede encontrar después: %+v %v", lotes, err)
	}

	// 3. Alguien edita a mano uno de los dos.
	beta := buscar("Beta SA")
	if beta == "" {
		t.Fatal("no se encontró el contacto Beta SA")
	}
	if _, err := st.Actualizar(ctx, m, beta, map[string]any{
		"nombre": "Beta SA (a mano)", "tipo": "organizacion", "externo_id": "ext-2",
	}, "Beta SA (a mano)"); err != nil {
		t.Fatalf("Actualizar: %v", err)
	}

	// 4. Revertir: se va lo que trajo la importación y NADIE pisa el trabajo
	// hecho después. Es el caso que justifica `updated_at_tras`.
	rev, err := st.Revertir(ctx, uno.ID, "jefa")
	if err != nil {
		t.Fatalf("Revertir: %v", err)
	}
	if rev.Borrados != 1 || rev.Intactos != 1 || rev.Restaurados != 0 {
		t.Fatalf("revertir: %+v, quiero 1 borrado y 1 intacto (el editado a mano)", rev)
	}
	if n := cuantos(); n != 1 || buscar("Beta SA (a mano)") != beta {
		t.Fatalf("tras revertir tiene que quedar solo el editado a mano (%d registros)", n)
	}
	// Y no se deshace dos veces.
	if _, err := st.Revertir(ctx, uno.ID, "jefa"); !errors.Is(err, store.ErrYaRevertida) {
		t.Fatalf("revertir dos veces: %v, quiero ErrYaRevertida", err)
	}

	// 5. Otra pasada del fichero corregido: ACTUALIZA por `externo_id`, no
	// duplica. Y revertirla devuelve exactamente lo que había antes de ella.
	dos, err := st.Importar(ctx, m, []store.FilaImportada{fila("Beta SAU", "ext-2")}, "clientes-v2.csv", "ana")
	if err != nil {
		t.Fatalf("Importar (2): %v", err)
	}
	if dos.Creados != 0 || dos.Actualizados != 1 {
		t.Fatalf("segunda importación: %+v, quiero 1 actualizado", dos)
	}
	if buscar("Beta SAU") != beta {
		t.Fatal("la segunda pasada tenía que actualizar el mismo registro, no crear otro")
	}
	if rev, err := st.Revertir(ctx, dos.ID, "jefa"); err != nil || rev.Restaurados != 1 {
		t.Fatalf("revertir la actualización: %+v %v", rev, err)
	}
	if buscar("Beta SA (a mano)") != beta {
		t.Fatal("revertir una actualización tiene que devolver los datos de antes")
	}

	// 6. Un lote que no existe no revienta.
	if _, err := st.Revertir(ctx, "00000000-0000-0000-0000-000000000000", "jefa"); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("revertir un lote inexistente: %v, quiero ErrNoEncontrado", err)
	}
}
