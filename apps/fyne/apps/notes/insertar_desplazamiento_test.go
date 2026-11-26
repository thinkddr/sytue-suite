// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElMenuInsertarNoVuelveSoloASuSitio: dentro de «+», lo que la persona desplaza la
// lista se queda. Pasar por encima de una fila la activa, y Fyne vuelve a mirar qué fila
// hay bajo el puntero cada vez que algo se repinta (el parpadeo del cursor): cada una de
// esas veces la lista se rehacía con la activa abajo del todo, y volvía sola a su sitio
// cada pocos segundos mientras se desplazaba (lo vio el dueño, 2026-11-26). Las flechas
// siguen llevando la lista hasta la activa, solo lo justo.
func TestElMenuInsertarNoVuelveSoloASuSitio(t *testing.T) {
	ed := editorNotas(t, "<p>Ojo</p>")
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 800))
	w.Canvas().Focus(ed.Zona())
	test.Tap(ed.botones["insertar"])
	lista := func() (*container.Scroll, []*filaSugerencia) {
		var d *container.Scroll
		var filas []*filaSugerencia
		recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *container.Scroll:
				d = x
			case *filaSugerencia:
				filas = append(filas, x)
			}
		})
		return d, filas
	}
	d, filas := lista()
	if d == nil || len(filas) <= filasMenuCmd {
		t.Fatalf("«+» no tiene una lista que desplazar: %v filas", len(filas))
	}
	alto := filas[0].MinSize().Height

	// La persona baja la lista con la rueda, siete filas.
	d.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -7*alto)})
	bajada := d.Offset.Y
	if bajada <= 0 {
		t.Fatalf("la rueda no movió la lista: %v", bajada)
	}
	// El puntero queda sobre otra fila, y Fyne lo vuelve a mirar al repintar: dos veces,
	// como pasa cada pocos segundos.
	for _, i := range []int{8, 10} {
		_, filas = lista()
		filas[i].MouseIn(&desktop.MouseEvent{})
		if d, _ = lista(); d.Offset.Y != bajada {
			t.Fatalf("pasar por la fila %d devolvió la lista de %v a %v", i, bajada, d.Offset.Y)
		}
	}

	// Las flechas sí la mueven, y solo hasta que la activa se ve: con la siguiente a la
	// vista no se mueve nada, y subiendo hasta la primera vuelve arriba.
	var campo *ui.Entrada
	recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Entrada); ok {
			campo = c
		}
	})
	if campo == nil {
		t.Fatal("«+» no tiene su caja de buscar")
	}
	campo.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyDown})
	if d, _ = lista(); d.Offset.Y != bajada {
		t.Fatalf("bajar a una fila que ya se veía movió la lista de %v a %v", bajada, d.Offset.Y)
	}
	for range 11 {
		campo.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyUp})
	}
	if d, _ = lista(); d.Offset.Y != 0 {
		t.Fatalf("subir hasta la primera deja la lista en %v, no arriba", d.Offset.Y)
	}
	ed.insertando.Cerrar()
}
