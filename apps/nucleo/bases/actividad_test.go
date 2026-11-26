// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// abrirConActividad es abrirLocal con el registro de actividad local encendido.
func abrirConActividad(t *testing.T, dir string, actividad *bases.Actividad) (*api.Bases, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "bases", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := bases.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	repo.Actividad = actividad
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		repo.Close()
		rec.Cerrar()
		t.Fatal("preparar:", err)
	}
	fachada, err := api.NewBases("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return fachada, func() { repo.Close(); rec.Cerrar() }
}

func acciones(es []api.BasesAuditEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Action
	}
	return out
}

// B-06: con el registro de actividad, cada cambio deja su entrada con las acciones de la
// traza del servicio, de la más reciente a la más antigua, por base, por páginas, y
// sobrevive a cerrar la app. Importar un CSV o reordenar los campos es UNA entrada.
func TestLaActividadLocalAnotaLosCambiosDeCadaBase(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	ruta := filepath.Join(dir, "actividad.jsonl")
	b, cerrar := abrirConActividad(t, dir, bases.NuevaActividad(ruta))
	base, err := b.CreateBase(ctx, "Clientes")
	if err != nil {
		t.Fatal(err)
	}
	otra, err := b.CreateBase(ctx, "Otra")
	if err != nil {
		t.Fatal(err)
	}
	tabla, err := b.CreateTable(ctx, base.ID, "Personas")
	if err != nil {
		t.Fatal(err)
	}
	nombre, err := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	ciudad, err := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Ciudad", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := b.CreateRecord(ctx, tabla.ID, map[string]any{nombre.ID: "Ana"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteCell(ctx, reg.ID, nombre.ID, api.BasesCell{Value: "Ana Ruiz"}, "op-1"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReorderFields(ctx, tabla.ID, []string{ciudad.ID, nombre.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ImportCSV(ctx, base.ID, "Pedidos", []byte("Cosa,Cantidad\nSilla,2\nMesa,1\n")); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteRecord(ctx, reg.ID, false); err != nil {
		t.Fatal(err)
	}
	cerrar()

	b2, cerrar2 := abrirConActividad(t, dir, bases.NuevaActividad(ruta))
	defer cerrar2()
	es, err := b2.ListAudit(ctx, base.ID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"record.delete", "table.import", "field.reorder", "record.update", "record.create",
		"field.create", "field.create", "table.create", "base.create",
	}
	if got := acciones(es); !slices.Equal(got, want) {
		t.Fatalf("actividad = %q, quiero %q", got, want)
	}
	if es[1].TableID == "" || string(es[1].After) != `{"name":"Pedidos"}` {
		t.Errorf("la importación no dice qué tabla: %+v", es[1])
	}
	if es[3].RecordID != reg.ID || es[3].FieldID != nombre.ID || es[3].Actor != "instalacion-1" {
		t.Errorf("la celda escrita no dice qué registro, campo y quién: %+v", es[3])
	}
	// Por páginas: lo anterior a la tercera.
	mas, err := b2.ListAudit(ctx, base.ID, es[2].ID, 2)
	if err != nil || !slices.Equal(acciones(mas), want[3:5]) {
		t.Fatalf("página siguiente = %q, %v", acciones(mas), err)
	}
	// Cada base, la suya.
	if es, err := b2.ListAudit(ctx, otra.ID, 0, 50); err != nil || !slices.Equal(acciones(es), []string{"base.create"}) {
		t.Fatalf("la otra base = %q, %v", acciones(es), err)
	}
}

// Sin registro de actividad (lo de siempre, también en las bases privadas de la
// conectada) no se escribe nada y la actividad falla como antes.
func TestSinActividadLaTrazaFallaComoSiempre(t *testing.T) {
	dir := t.TempDir()
	b, cerrar := abrirLocal(t, dir)
	defer cerrar()
	base, err := b.CreateBase(t.Context(), "Clientes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ListAudit(t.Context(), base.ID, 0, 50); !errors.Is(err, api.ErrPrivateBasesOperation) {
		t.Fatalf("ListAudit sin actividad = %v", err)
	}
}
