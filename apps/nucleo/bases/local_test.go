// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// abrirLocal monta el repositorio de Bases sobre el almacén en claro de Sytue Office,
// como `cmd-local/bases`, y le pone delante la fachada de las pantallas.
func abrirLocal(t *testing.T, dir string) (*api.Bases, func()) {
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

// Una base con su tabla, un campo, un registro y una celda escrita sobrevive a cerrar y
// volver a abrir el almacén en claro, sin servidor y sin cifrado.
func TestBasesSobreviveAReabrirElAlmacenEnClaro(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	b, cerrar := abrirLocal(t, dir)
	base, err := b.CreateBase(ctx, "Clientes")
	if err != nil {
		t.Fatal(err)
	}
	tabla, err := b.CreateTable(ctx, base.ID, "Personas")
	if err != nil {
		t.Fatal(err)
	}
	campo, err := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := b.CreateRecord(ctx, tabla.ID, map[string]any{campo.ID: "Ana"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteCell(ctx, reg.ID, campo.ID, api.BasesCell{Value: "Ana Ruiz"}, "op-1"); err != nil {
		t.Fatal(err)
	}
	cerrar()

	b2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	lista, err := b2.ListBases(ctx)
	if err != nil || len(lista) != 1 || lista[0].Name != "Clientes" || lista[0].Role != api.BasesOwner {
		t.Fatal("bases tras reabrir:", lista, err)
	}
	tablas, err := b2.ListTables(ctx, base.ID)
	if err != nil || len(tablas) != 1 || tablas[0].Name != "Personas" {
		t.Fatal("tablas tras reabrir:", tablas, err)
	}
	pagina, err := b2.ListRecords(ctx, tabla.ID, "", "", 100)
	if err != nil || len(pagina.Fields) != 1 || len(pagina.Records) != 1 {
		t.Fatal("registros tras reabrir:", pagina, err)
	}
	if got := pagina.Records[0].Data[campo.ID]; got != "Ana Ruiz" {
		t.Fatalf("la celda escrita no ha vuelto: %v", got)
	}
	// Lo que necesita un servicio falla cerrado, sin red: no se cuela una petición.
	if _, err := b2.ListPermissions(ctx, base.ID); err == nil {
		t.Fatal("los permisos no tienen camino local y deben fallar")
	}
}
