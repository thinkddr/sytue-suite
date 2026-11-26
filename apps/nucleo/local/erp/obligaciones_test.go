// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La cifra del 303 sale del libro, el proceso exige lo pendiente resuelto, los pasos
// en orden y el justificante guardado; el calendario baja como ICS.
func TestCalculoYProcesoDeObligaciones(t *testing.T) {
	ctx := context.Background()
	e, _ := abrir(t, t.TempDir())
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{
		Nombre: "Taller Ruiz", Pais: "ES", Forma: "sl", CP: "28013",
		Condiciones: map[string]bool{"iva": true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Bolt España SL", "tipo": "organizacion"})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-08-05", "base": 10000, "iva": 2100, "total": 12100})
	if err != nil {
		t.Fatal(err)
	}

	// Con la factura en borrador, el 303 del tercer trimestre tiene una pendiente.
	ob, err := e.Obligaciones(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	var vista *api.ERPObligacion
	for i := range ob.Obligaciones {
		if ob.Obligaciones[i].Clave == "es_303" && ob.Obligaciones[i].Periodo == "2026-T3" {
			vista = &ob.Obligaciones[i]
		}
	}
	if vista == nil || vista.Pendientes["facturas"] != 1 || vista.Proceso != nil {
		t.Fatalf("303 T3: %+v", vista)
	}
	if _, err := e.AvanzarProcesoObligacion(ctx, "es_303", "2026-T3", 1, ""); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("con pendientes no se avanza: %v", err)
	}
	if pag, err := e.FacturasPendientesObligacion(ctx, "es_303", "2026-T3", "", 10); err != nil || len(pag.Registros) != 1 || pag.Registros[0].ID != fac.ID {
		t.Fatalf("pendientes: %v %+v", err, pag)
	}

	if _, err := e.Asentar(ctx, "factura_venta", fac.ID); err != nil {
		t.Fatal(err)
	}
	c, err := e.CalculoObligacion(ctx, "es_303", "2026-T3")
	if err != nil || !c.Asentado || c.ResultadoCentimos != 2100 || c.Desde != "2026-07-01" || c.Hasta != "2026-09-30" {
		t.Fatalf("cálculo: %v %+v", err, c)
	}

	for fase := 1; fase <= 3; fase++ {
		if p, err := e.AvanzarProcesoObligacion(ctx, "es_303", "2026-T3", fase, ""); err != nil || p.Fase != fase {
			t.Fatalf("fase %d: %v %+v", fase, err, p)
		}
	}
	// Repetir un paso hecho no cambia nada; saltar, sí falla.
	if p, err := e.AvanzarProcesoObligacion(ctx, "es_303", "2026-T3", 2, ""); err != nil || p.Fase != 3 {
		t.Fatalf("repetir: %v %+v", err, p)
	}
	if _, err := e.AvanzarProcesoObligacion(ctx, "es_303", "2026-T3", 4, "CSV-123"); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("sin justificante: %v", err)
	}
	if _, err := e.SubirAdjunto(ctx, "obligacion", "es_303:2026-T3:justificante", "justificante.pdf", "application/pdf", []byte("%PDF-1.4\n")); err != nil {
		t.Fatal(err)
	}
	if p, err := e.AvanzarProcesoObligacion(ctx, "es_303", "2026-T3", 4, " CSV-123 "); err != nil || p.Fase != 4 || p.Referencia != "CSV-123" {
		t.Fatalf("terminar: %v %+v", err, p)
	}
	if p, err := e.ProcesoObligacion(ctx, "es_303", "2026-T3"); err != nil || p.Fase != 4 {
		t.Fatalf("leer: %v %+v", err, p)
	}

	cuerpo, nombre, err := e.ObligacionesICS(ctx, 2026)
	if ics := leerTodo(t, cuerpo, err); !strings.HasPrefix(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, "es_303-2026-T3") || nombre != "obligaciones-es-2026.ics" {
		t.Fatalf("ICS %q: %.200s", nombre, ics)
	}
}
