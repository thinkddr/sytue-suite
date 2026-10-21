//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestOrigenCompraCompartidoNoDuplica(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	m := modelo(t, c, "factura_compra")
	proveedor := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Proveedor","tipo":"organizacion","nif":"B12345678"}`)
	datos := func() map[string]any {
		return map[string]any{"proveedor": proveedor.ID, "fecha": "2026-10-03", "base": int64(1000), "iva": int64(210), "total": int64(1210), "externo_id": "B12345678/F-1"}
	}
	inicio := make(chan struct{})
	resultados := make(chan error, 4)
	var wg sync.WaitGroup
	acciones := []func() error{
		func() error { _, err := st.Crear(ctx, m, datos(), "Factura", "ada"); return err },
		func() error {
			_, _, err := st.CrearVarios(ctx, []store.Nuevo{{Modelo: m, Data: datos(), Etiqueta: "Factura"}}, nil, "ada")
			return err
		},
		func() error {
			r, err := st.Importar(ctx, m, []store.FilaImportada{{Linea: 1, Data: datos(), Externo: "B12345678/F-1", Etiqueta: "Factura"}}, "origen.xml", "ada")
			if len(r.Fallos) > 0 {
				t.Errorf("fallos importación: %+v", r.Fallos)
			}
			return err
		},
		func() error {
			_, err := st.UpsertPorExterno(ctx, m, "B12345678/F-1", datos(), "Factura", "", "ada")
			return err
		},
	}
	for _, accion := range acciones {
		wg.Add(1)
		go func() { defer wg.Done(); <-inicio; resultados <- accion() }()
	}
	close(inicio)
	wg.Wait()
	close(resultados)
	for err := range resultados {
		if err != nil && !errors.Is(err, store.ErrRefDuplicada) {
			t.Fatal(err)
		}
	}
	var n int
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM records WHERE model='factura_compra' AND data->>'externo_id'='B12345678/F-1' AND deleted_at IS NULL`).Scan(&n)
	}); err != nil || n != 1 {
		t.Fatalf("compras: %d %v", n, err)
	}
	reg, err := st.BuscarPorExterno(ctx, "factura_compra", "B12345678/F-1")
	if err != nil {
		t.Fatal(err)
	}
	cambios := datos()
	delete(cambios, "externo_id")
	reg, err = st.Actualizar(ctx, m, reg.ID, cambios, "")
	if err != nil || reg.Data["externo_id"] != "B12345678/F-1" {
		t.Fatalf("identidad perdida: %+v %v", reg, err)
	}
	cambios["externo_id"] = "B12345678/F-2"
	if _, err = st.Actualizar(ctx, m, reg.ID, cambios, ""); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("cambio de origen permitido: %v", err)
	}
	otro := tenantCtx("otra")
	p := crear(otro, t, st, modelo(t, c, "contacto"), `{"nombre":"Proveedor","tipo":"organizacion"}`)
	mismos := datos()
	mismos["proveedor"] = p.ID
	if _, err = st.Crear(otro, m, mismos, "Factura", "ada"); err != nil {
		b, _ := json.Marshal(mismos)
		t.Fatalf("origen de otra organización: %s %v", b, err)
	}
}
