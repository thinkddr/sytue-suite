// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

const baseCafe = `{"id":"b9","name":"Café de la esquina","description":"Proveedores y pedidos\nsegunda línea","owner_sub":"u1","role":"owner","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-04T09:00:00Z"}`

// TestMisBasesSeBuscanConPlegar: el buscador sale con más de una base, filtra por nombre
// y descripción sin mayúsculas ni acentos, y dice cuándo no hay ninguna.
func TestMisBasesSeBuscanConPlegar(t *testing.T) {
	p, _, w := montar(t, map[string]string{"GET /bases": "[" + baseClientes + "," + basePedidos + "," + baseCafe + "]"})
	capturar(t, w, "lista-descripcion")
	// La descripción va en la fila: su primera línea.
	if got := textos(w); !strings.Contains(got, "Café de la esquina, Proveedores y pedidos, Actualizada") {
		t.Fatalf("la fila no lleva la descripción:\n%s", got)
	}
	if p.buscar == nil {
		t.Fatal("no hay buscador")
	}
	tarjetas := func() []string {
		var out []string
		recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
			if x, ok := o.(*tarjetaBase); ok {
				out = append(out, x.Nombre)
			}
		})
		return out
	}
	p.buscar.PonerTexto("cafe")
	if got := tarjetas(); len(got) != 1 || got[0] != "Café de la esquina" {
		t.Fatalf("«cafe» encuentra %q", got)
	}
	p.buscar.PonerTexto("PEDIDOS")
	if got := tarjetas(); len(got) != 2 {
		t.Fatalf("«PEDIDOS» tiene que encontrar el nombre de una y la descripción de otra: %q", got)
	}
	p.buscar.PonerTexto("nada de nada")
	capturar(t, w, "lista-sin-resultados")
	if got := textos(w); len(tarjetas()) != 0 || !strings.Contains(got, "Ninguna base coincide con «nada de nada».") {
		t.Fatalf("sin resultados no lo dice:\n%s", got)
	}
}

// TestConUnaBaseNoHayBuscador: con una sola no hay nada que buscar.
func TestConUnaBaseNoHayBuscador(t *testing.T) {
	p, _, w := montar(t, map[string]string{"GET /bases": "[" + baseClientes + "]"})
	encontrado := false
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) { encontrado = encontrado || o == p.buscar })
	if encontrado {
		t.Error("con una sola base sale el buscador")
	}
}

// entradasAuditoria son n entradas de «editó un registro» de u2, de id `desde` hacia abajo.
func entradasAuditoria(desde, n int, actor string) string {
	xs := make([]string, 0, n)
	for i := range n {
		xs = append(xs, fmt.Sprintf(`{"id":%d,"table_id":"t1","record_id":"r1","actor":%q,"action":"record.update","at":"2026-11-12T10:00:00Z"}`, desde-i, actor))
	}
	return "[" + strings.Join(xs, ",") + "]"
}

// TestLaActividadDeLaBase: el propietario abre la actividad desde el menú, ve las
// entradas con el nombre de quien las hizo (nunca su `sub`) y «Cargar más» pide desde
// la última; cuando la página viene corta, se acaba.
func TestLaActividadDeLaBase(t *testing.T) {
	rutas := map[string]string{
		"GET /bases": "[" + baseClientes + "]", "GET /bases/b1": baseClientes, "GET /bases/b1/tables": tablasB1,
		"GET /tables/t1/views": "[]", "GET /tables/t1/records": `{"fields":[],"records":[],"role":"owner"}`,
		"GET /bases/b1/permissions": `[{"id":"p1","principal_kind":"user","principal_id":"u2","principal_label":"Ana Ruiz","role":"editor","created_by":"u1","created_at":"2026-10-01T09:00:00Z"}]`,
		"GET /bases/b1/audit":       entradasAuditoria(500, paginaActividad, "u2"),
	}
	p, d, w := montar(t, rutas)
	p.Yo = func() string { return "u1" }
	p.Abrir("b1")
	pulsarEnMenu(t, w, "Clientes", "Actividad")
	capturar(t, w, "base-actividad")
	got := textos(w)
	for _, quiero := range []string{"Actividad de Clientes", "Ana Ruiz · Editó un registro", "Personas", "[Cargar más]"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, got)
		}
	}
	if strings.Contains(got, "u2") {
		t.Errorf("se enseña el sub de alguien:\n%s", got)
	}
	if !d.vio("GET /bases/b1/audit?limit=50") {
		t.Fatalf("la primera página no se ha pedido así: %q", d.vistas)
	}

	d.pon("GET /bases/b1/audit", entradasAuditoria(7, 1, "u1"))
	test.Tap(boton(w, "Cargar más"))
	if !d.vio("GET /bases/b1/audit?before=451&limit=50") {
		t.Fatalf("«Cargar más» no pide desde la última: %q", d.vistas)
	}
	got = textos(w)
	if !strings.Contains(got, "Tú · Editó un registro") {
		t.Errorf("las entradas propias no son «Tú»:\n%s", got)
	}
	if boton(w, "Cargar más") != nil {
		t.Error("con la última página corta sigue «Cargar más»")
	}
}

// TestLaActividadVaciaLoDice: sin entradas, el texto, y sin «Cargar más».
func TestLaActividadVaciaLoDice(t *testing.T) {
	rutas := map[string]string{
		"GET /bases": "[" + baseClientes + "]", "GET /bases/b1": baseClientes, "GET /bases/b1/tables": `[]`,
		"GET /bases/b1/permissions": `[]`, "GET /bases/b1/audit": `[]`,
	}
	p, _, w := montar(t, rutas)
	p.Abrir("b1")
	pulsarEnMenu(t, w, "Clientes", "Actividad")
	if got := textos(w); !strings.Contains(got, "Todavía no hay nada anotado en esta base.") || strings.Contains(got, "[Cargar más]") {
		t.Errorf("la actividad vacía no lo dice:\n%s", got)
	}
}

// TestEditarLaDescripcion: el diálogo trae la de ahora, no guarda sin cambios, manda el
// nombre de siempre con la nueva recortada y la cabecera la enseña.
func TestEditarLaDescripcion(t *testing.T) {
	rutas := map[string]string{
		"GET /bases": "[" + baseClientes + "]", "GET /bases/b1": baseClientes, "GET /bases/b1/tables": `[]`,
		"PATCH /bases/b1": "",
	}
	p, d, w := montar(t, rutas)
	p.Abrir("b1")
	pulsarEnMenu(t, w, "Clientes", "Editar la descripción")
	var area *ui.AreaTexto
	for _, capa := range w.Canvas().Overlays().List() {
		recorrer(capa, func(o fyne.CanvasObject) {
			if a, ok := o.(*ui.AreaTexto); ok {
				area = a
			}
		})
	}
	if area == nil {
		t.Fatalf("no hay área de texto:\n%s", textos(w))
	}
	if g := boton(w, "Guardar"); g == nil || !g.Inerte {
		t.Fatal("«Guardar» tiene que estar apagado sin cambios")
	}
	area.PonerTexto("  Quién nos compra  ")
	capturar(t, w, "base-descripcion")
	d.pon("GET /bases/b1", strings.Replace(baseClientes, `"description":""`, `"description":"Quién nos compra"`, 1))
	test.Tap(boton(w, "Guardar"))
	if !d.vio(`PATCH /bases/b1 {"name":"Clientes","description":"Quién nos compra"}`) {
		t.Fatalf("no se ha guardado la descripción: %q", d.vistas)
	}
	if got := textos(w); !strings.Contains(got, "Quién nos compra") || strings.Contains(got, "Editar la descripción") {
		t.Errorf("la cabecera no enseña la descripción o el diálogo sigue abierto:\n%s", got)
	}
}
