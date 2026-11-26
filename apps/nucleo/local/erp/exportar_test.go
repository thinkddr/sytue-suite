// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
)

func TestExportarCSV(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()
	cli, _ := e.Crear(ctx, "contacto", map[string]any{"nombre": "=HYPERLINK(\"x\")", "tipo": "organizacion"})
	f1, _ := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-03-04", "base": 123456, "total": -5})
	_, _ = e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-03-05", "base": 100})

	rc, nombre, err := e.Exportar(ctx, "factura_venta", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	filas, err := csv.NewReader(rc).ReadAll()
	_ = rc.Close()
	if err != nil || len(filas) != 3 || !strings.HasPrefix(nombre, "factura_venta-") {
		t.Fatalf("csv: %v %d %q", err, len(filas), nombre)
	}
	todo := strings.Join(filas[2], "|")
	if !strings.Contains(todo, "FV-2026-00001") || !strings.Contains(todo, "-0.05") ||
		!strings.Contains(todo, "'=HYPERLINK") {
		t.Fatalf("la fila de la primera factura: %q (cabecera %q)", todo, filas[0])
	}
	rc, _, err = e.Exportar(ctx, "factura_venta", "", "", []string{f1.ID})
	if err != nil {
		t.Fatal(err)
	}
	filas, _ = csv.NewReader(rc).ReadAll()
	_ = rc.Close()
	if len(filas) != 2 {
		t.Fatalf("solo las marcadas: %d", len(filas))
	}
}
