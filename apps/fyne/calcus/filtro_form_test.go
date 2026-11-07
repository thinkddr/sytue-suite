// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestFiltrarPorCondicionDesdeElDesplegable: «Filtrar por condición…» del «▾» abre el
// formulario de `FiltroCondicion.tsx`; «Entre» con dos fechas tecleadas filtra ese mes,
// «Aplicar» solo se enciende con los dos operandos, «Quitar condición» la quita y
// «Volver a los valores» vuelve a la lista.
//
// VISTO EN ROJO (2026-11-05): sin la entrada en `abrirFiltro`, el modelo y el comando
// existían pero no había desde dónde pedirlos.
func TestFiltrarPorCondicionDesdeElDesplegable(t *testing.T) {
	lz := lienzoDePrueba()
	// 46037 = 15/01/2026, 46068 = 15/02/2026, 46040 = 18/01/2026.
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"Fecha","A2":"46037","A3":"46068","A4":"46040"},` +
		`"autofilter":{"row":0,"c1":0,"c2":0}}]}`)
	w := test.NewTempWindow(t, lz)
	w.Resize(fyne.NewSize(560, 300))
	abrir := func() fyne.CanvasObject {
		lz.abrirFiltro(0)
		capas := w.Canvas().Overlays().List()
		if len(capas) != 1 {
			t.Fatalf("hay %d capas abiertas", len(capas))
		}
		return capas[0]
	}
	porCond := func(capa fyne.CanvasObject) *ui.ElementoMenu {
		for _, x := range todos(capa) {
			if e, ok := x.(*ui.ElementoMenu); ok && e.Texto == lz.t("filtro.byCondition", nil) {
				return e
			}
		}
		t.Fatal("el desplegable no ofrece «Filtrar por condición…»")
		return nil
	}
	boton := func(capa fyne.CanvasObject, clave string) *editor.BotonPequeno {
		for _, x := range todos(capa) {
			if b, ok := x.(*editor.BotonPequeno); ok && b.Texto == lz.t(clave, nil) {
				return b
			}
		}
		return nil
	}
	formulario := func() fyne.CanvasObject {
		capas := w.Canvas().Overlays().List()
		if len(capas) != 1 {
			t.Fatalf("el formulario: %d capas", len(capas))
		}
		return capas[0]
	}

	porCond(abrir()).Tapped(nil)
	capa := formulario()
	var op *editor.SelectorCinta
	for _, x := range todos(capa) {
		if s, ok := x.(*editor.SelectorCinta); ok && s.Valor == "gt" {
			op = s
		}
	}
	if op == nil {
		t.Fatal("el formulario no arranca en «Mayor que»")
	}
	op.AlElegir("between")
	capa = formulario()
	var campos []*ui.Entrada
	for _, x := range todos(capa) {
		if e, ok := x.(*ui.Entrada); ok {
			campos = append(campos, e)
		}
	}
	if len(campos) != 2 {
		t.Fatalf("«Entre» pide dos valores y hay %d campos", len(campos))
	}
	aplicar := boton(capa, "action.apply")
	if aplicar == nil || !aplicar.Inerte || boton(capa, "filtro.clear") != nil {
		t.Fatal("sin operandos, «Aplicar» tiene que estar apagado, y sin condición no hay «Quitar»")
	}
	test.Type(campos[0], "01/01/2026")
	if !aplicar.Inerte {
		t.Error("con un solo extremo, «Aplicar» se enciende")
	}
	test.Type(campos[1], "31/01/2026")
	if aplicar.Inerte {
		t.Fatal("con los dos extremos, «Aplicar» sigue apagado")
	}
	aplicar.AlPulsar()
	if oc := lz.Libro.FilasOcultas(); oc[1] || !oc[2] || oc[3] {
		t.Errorf("entre el 1 y el 31 de enero se ocultan %v (solo la fila de febrero)", oc)
	}
	if cond := lz.Libro.CondicionFiltro(0); cond == nil || cond.Op != "between" || cond.A != "01/01/2026" {
		t.Errorf("la condición guardada: %+v", cond)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("aplicar no cierra el formulario")
	}

	// Con condición, el formulario la trae y ofrece quitarla.
	porCond(abrir()).Tapped(nil)
	capa = formulario()
	quitar := boton(capa, "filtro.clear")
	if quitar == nil {
		t.Fatal("con una condición puesta no se ofrece «Quitar condición»")
	}
	quitar.AlPulsar()
	if oc := lz.Libro.FilasOcultas(); lz.Libro.CondicionFiltro(0) != nil || len(oc) > 2 && oc[2] {
		t.Error("«Quitar condición» no la quita")
	}

	// «Volver a los valores» vuelve a la lista de casillas.
	porCond(abrir()).Tapped(nil)
	boton(formulario(), "filtro.back").AlPulsar()
	if n := len(lz.Libro.ValoresFiltro(0)); n == 0 || len(filasDe(formulario())) != n {
		t.Errorf("«Volver a los valores» no vuelve a la lista: %d filas, %d capas", len(filasDe(formulario())), len(w.Canvas().Overlays().List()))
	}
}
