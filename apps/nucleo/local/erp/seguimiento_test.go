// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestTareasYAdjuntos(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, alm := abrir(t, dir)
	defer func() { _ = alm.Cerrar() }()
	cli, _ := e.Crear(ctx, "contacto", map[string]any{"nombre": "Ana", "tipo": "persona"})
	gasto, err := e.Crear(ctx, "factura_compra", map[string]any{"proveedor": cli.ID, "fecha": "2026-03-01", "base": 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CrearActividad(ctx, "factura_compra", gasto.ID, "subir_documento", "Sube la factura", "2026-03-10"); err != nil {
		t.Fatal(err)
	}
	if err := e.CrearActividad(ctx, "", "", "llamada", "Llamar al gestor", "2026-03-05"); err != nil {
		t.Fatal(err)
	}
	if err := e.CrearActividad(ctx, "", "", "nada", "", "mañana"); estado(err) != 422 {
		t.Fatalf("tarea inválida: %v", err)
	}
	ts, err := e.Actividades(ctx, api.ERPFiltroActividades{Estado: "pendiente"})
	if err != nil || len(ts) != 2 || ts[0].Resumen != "Llamar al gestor" || ts[1].RegistroEtiqueta != gasto.Ref {
		t.Fatalf("tareas: %v %+v", err, ts)
	}
	if ps, err := e.Equipo(ctx); err != nil || len(ps) != 1 || !ps[0].Yo {
		t.Fatalf("equipo: %v %+v", err, ps)
	}

	// Subir la factura ES la tarea: se cierra sola. El fichero queda en la carpeta.
	pdf := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
	n, err := e.SubirAdjunto(ctx, "factura_compra", gasto.ID, "factura", "application/pdf", pdf)
	if err != nil || n != 1 {
		t.Fatalf("subir: %v %d", err, n)
	}
	if _, err := e.SubirAdjunto(ctx, "factura_compra", gasto.ID, "x.html", "text/html", []byte("<html><script>")); estado(err) != 415 {
		t.Fatalf("un HTML no se guarda: %v", err)
	}
	ads, err := e.Adjuntos(ctx, "factura_compra", gasto.ID)
	if err != nil || len(ads) != 1 || ads[0].Nombre != "factura.pdf" || ads[0].Tipo != "application/pdf" {
		t.Fatalf("adjuntos: %v %+v", err, ads)
	}
	en, _ := os.ReadFile(filepath.Join(dir, "adjuntos", ads[0].ID+".pdf"))
	if !bytes.Equal(en, pdf) {
		t.Fatal("el documento está en claro en la carpeta de datos")
	}
	rc, err := e.Adjunto(ctx, ads[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	bajado, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(bajado, pdf) {
		t.Fatalf("bajar: %q", bajado)
	}
	if hechas, _ := e.Actividades(ctx, api.ERPFiltroActividades{Estado: "hecha"}); len(hechas) != 1 {
		t.Fatalf("la tarea de subir queda hecha: %+v", hechas)
	}
	if err := e.CerrarActividad(ctx, ts[0].ID, "cancelar"); err != nil {
		t.Fatal(err)
	}
	if err := e.CerrarActividad(ctx, ts[0].ID, "hecha"); estado(err) != 404 {
		t.Fatalf("cerrar una cancelada: %v", err)
	}
	if err := e.BorrarAdjunto(ctx, ads[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "adjuntos", ads[0].ID+".pdf")); !os.IsNotExist(err) {
		t.Fatalf("borrar quita el fichero: %v", err)
	}
}
