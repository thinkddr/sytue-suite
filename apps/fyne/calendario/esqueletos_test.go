// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// sueltos cuenta los `ui.Esqueleto` sueltos que cuelgan de o: cada uno late por su
// cuenta y no anuncia la carga.
func sueltos(o fyne.CanvasObject) int {
	n := 0
	buscarObjeto(o, func(x fyne.CanvasObject) bool {
		if _, es := x.(*ui.Esqueleto); es {
			n++
		}
		return false
	})
	return n
}

// enVentana monta o sola en una ventana sin relleno de `tam` y devuelve dónde cae cada
// bloque, en coordenadas de la ventana.
func enVentana(t *testing.T, o fyne.CanvasObject, tam fyne.Size) func(fyne.CanvasObject) (fyne.Position, fyne.Size) {
	t.Helper()
	w := test.NewTempWindow(t, o)
	w.SetPadded(false)
	w.Resize(tam)
	d := fyne.CurrentApp().Driver()
	return func(b fyne.CanvasObject) (fyne.Position, fyne.Size) { return d.AbsolutePositionForObject(b), b.Size() }
}

// TestLosBloquesDeCargaDeCalendarLatenAUna: la rejilla del mes, los calendarios del
// menú y las salas del formulario cargan con las piezas comunes —un solo latido y un
// solo «Cargando» por grupo—, con la MISMA geometría que tenían con un `ui.Esqueleto`
// por bloque (las cifras salen de volcar los rectángulos antes del cambio).
//
// VISTO EN ROJO con los esqueletos sueltos de antes: 49 en la rejilla, 2 en el menú y
// 1 en las salas.
func TestLosBloquesDeCargaDeCalendarLatenAUna(t *testing.T) {
	p := pantallaCaptura(t)
	p.Ventana = 1280

	rejilla := p.esqueleto()
	b, es := rejilla.(*ui.BloquesEsqueleto)
	if !es || len(b.Bloques()) != 49 || sueltos(rejilla) != 0 {
		t.Fatalf("la rejilla cargando es %T con %d esqueletos sueltos", rejilla, sueltos(rejilla))
	}
	if got, quiero := b.AccessibilityLabel(), p.t("loading.calendar", nil); got != quiero {
		t.Errorf("la rejilla cargando se anuncia «%s» y es «%s»", got, quiero)
	}
	donde := enVentana(t, rejilla, fyne.NewSize(980, 700))
	for _, c := range []struct {
		i        int
		pos, tam fyne.Size
	}{
		{0, fyne.NewSize(10, 10), fyne.NewSize(120, 12)},    // el nombre del lunes
		{7, fyne.NewSize(6, 39), fyne.NewSize(128, 24)},     // la primera celda
		{48, fyne.NewSize(846, 219), fyne.NewSize(128, 24)}, // la última
	} {
		pos, tam := donde(b.Bloques()[c.i])
		if pos != fyne.NewPos(c.pos.Width, c.pos.Height) || tam != c.tam {
			t.Errorf("el bloque %d de la rejilla cae en %v y mide %v; antes, %v y %v", c.i, pos, tam, c.pos, c.tam)
		}
	}

	p.CargandoCalendarios = true
	menu := p.seccionMisCalendarios()
	if len(menu.Filas) != 1 || sueltos(menu) != 0 {
		t.Fatalf("el menú cargando tiene %d piezas y %d esqueletos sueltos", len(menu.Filas), sueltos(menu))
	}
	filas, es := menu.Filas[0].(*ui.FilasEsqueleto)
	if !es || filas.Filas != 2 || filas.Tinte != "sidebar-hover-bg" {
		t.Fatalf("el menú cargando es %T", menu.Filas[0])
	}
	donde = enVentana(t, menu, fyne.NewSize(240, 300))
	for i, y := range []float32{34, 72} {
		pos, tam := donde(test.WidgetRenderer(filas).Objects()[i])
		if pos != fyne.NewPos(0, y) || tam != fyne.NewSize(240, 36) {
			t.Errorf("la fila %d del menú cae en %v y mide %v", i, pos, tam)
		}
	}

	a, w := appConVentana(t, &escrituras{})
	inicio := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.rec.lista = nil // aún no han llegado
	salas := f.bloqueRecursos()
	var grupo *ui.BloquesEsqueleto
	buscarObjeto(salas, func(x fyne.CanvasObject) bool {
		grupo, _ = x.(*ui.BloquesEsqueleto)
		return grupo != nil
	})
	if grupo == nil || len(grupo.Bloques()) != 1 || sueltos(salas) != 0 {
		t.Fatalf("las salas cargando no son un grupo de un bloque (%d sueltos)", sueltos(salas))
	}
	w.SetContent(salas)
	w.Resize(fyne.NewSize(400, 200))
	if pos, tam := fyne.CurrentApp().Driver().AbsolutePositionForObject(grupo.Bloques()[0]), grupo.Bloques()[0].Size(); pos.Y != tema.TextoSM.Linea+tema.Espacio(1.5) || tam.Height != tema.Espacio(9) {
		t.Errorf("el bloque de las salas cae en %v y mide %v", pos, tam)
	}
}
