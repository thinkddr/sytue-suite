// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestImportarSimularYDeshacer(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()
	previo, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Viejo", "tipo": "persona", "externo_id": "x-1"})
	if err != nil {
		t.Fatal(err)
	}
	csv := []byte("Nombre;Tipo;externo_id;Fax\nAna López;Persona;;1\nOtro;marciano;;\nNuevo nombre;organizacion;x-1;\n")

	// La vista previa cuenta lo que pasaría y no escribe nada.
	sim, err := e.ImportarCon(ctx, "contacto", csv, false, api.ERPOpcionesImportacion{Simular: true})
	if err != nil || !sim.Simulacion || sim.ID != "" || sim.Creados != 1 || sim.Actualizados != 1 || len(sim.Fallos) != 1 ||
		sim.Fallos[0].Fila != 3 || len(sim.Ignoradas) != 1 || sim.Ignoradas[0] != "Fax" {
		t.Fatalf("simular: %v %+v", err, sim)
	}
	if pag, _ := e.ListarCon(ctx, "contacto", api.ERPFiltro{}); len(pag.Registros) != 1 {
		t.Fatalf("simular no escribe: %d", len(pag.Registros))
	}

	res, err := e.ImportarCon(ctx, "contacto", csv, false, api.ERPOpcionesImportacion{Fichero: "contactos.csv"})
	if err != nil || res.ID == "" || res.Creados != 1 || res.Actualizados != 1 || res.Filas != 3 {
		t.Fatalf("importar: %v %+v", err, res)
	}
	if r, _ := e.Obtener(ctx, "contacto", previo.ID); r.Etiqueta != "Nuevo nombre" || r.Data["tipo"] != "organizacion" {
		t.Fatalf("actualizar por externo_id: %+v", r)
	}
	// Una relación por su NOMBRE.
	if res, err := e.Importar(ctx, "factura_venta", []byte("cliente,fecha,base\nana lopez,04/03/2026,\"1.234,56\"\n"), false); err != nil || res.Creados != 1 {
		t.Fatalf("relación por nombre: %v %+v", err, res)
	}

	rev, err := e.RevertirImportacion(ctx, res.ID)
	if err != nil || rev.Borrados != 1 || rev.Restaurados != 1 || rev.Intactos != 0 {
		t.Fatalf("revertir: %v %+v", err, rev)
	}
	if r, _ := e.Obtener(ctx, "contacto", previo.ID); r.Etiqueta != "Viejo" || r.Data["tipo"] != "persona" {
		t.Fatalf("deshacer devuelve lo que había: %+v", r)
	}
	if _, err := e.RevertirImportacion(ctx, res.ID); codigo(err) != "ya_revertida" {
		t.Fatalf("revertir dos veces: %v", err)
	}
}
