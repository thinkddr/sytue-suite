// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// TestElBotonFlotanteNoVaEnElDetalle: en el teléfono, con la ficha de un contacto o el
// lector de Mail a la vista (el detalle del armazón), el «+» tapaba el final de lo que se
// lee y no es la acción de esa pantalla (auditoría del 2026-11-24). Con la lista, sí. La
// capa se entera al cambiar el armazón, sin que la app la rehaga.
// VISTO EN ROJO sin `detalleALaVista`: el botón seguía a la vista en el detalle.
func TestElBotonFlotanteNoVaEnElDetalle(t *testing.T) {
	a := NuevoArmazon(canvas.NewRectangle(color.NRGBA{}))
	a.Panel = canvas.NewRectangle(color.NRGBA{})
	fab := NuevoBotonFlotante("Nuevo contacto", nil)
	w := test.NewTempWindow(t, container.NewStack(a, container.New(CapaFlotante{}, fab)))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 700))
	if !fab.Visible() {
		t.Fatal("con la lista a la vista el botón flotante no se ve")
	}
	a.DetalleAbierto = true
	a.Refresh()
	if fab.Visible() {
		t.Error("con el detalle a la vista el botón flotante sigue encima")
	}
	a.DetalleAbierto = false
	a.Refresh()
	if !fab.Visible() {
		t.Error("al volver a la lista el botón flotante no vuelve")
	}
}

// TestLasListasDejanSitioAlBotonFlotante: el final de una lista quedaba bajo el «+» (Mail,
// Contacts, Drive y Cast). El contenido de un desplazamiento gana la reserva (88), y una
// `widget.List` una fila vacía de ese alto al final, que no se pinta ni se elige. En TODOS
// los anchos, que el «+» ya se ve también en escritorio (2026-11-25). VISTO EN ROJO sin la
// reserva, y a 1280 con la de antes, que solo valía por debajo de `Estrecho`.
func TestLasListasDejanSitioAlBotonFlotante(t *testing.T) {
	if ReservaFlotante != 88 {
		t.Errorf("la reserva mide %v y son `h-14` más dos `bottom-4`: 88", ReservaFlotante)
	}
	lleno := canvas.NewRectangle(color.NRGBA{})
	lleno.SetMinSize(fyne.NewSize(100, 300))
	envuelto := ConReservaFlotante(lleno)
	w := test.NewTempWindow(t, container.NewVScroll(envuelto))
	w.Resize(fyne.NewSize(390, 700))
	if got := envuelto.MinSize().Height; got != 300+ReservaFlotante {
		t.Errorf("a 390 el contenido mide %v y con la reserva son %v", got, 300+ReservaFlotante)
	}
	w.Resize(fyne.NewSize(1280, 800))
	if got := envuelto.MinSize().Height; got != 300+ReservaFlotante {
		t.Errorf("a 1280 el contenido mide %v y el «+» también flota ahí: son %v", got, 300+ReservaFlotante)
	}

	// Con sitio para la «Dora» que se añade más abajo.
	filas := make([]string, 0, 4)
	filas = append(filas, "Ana", "Bruno", "Carla")
	l := widget.NewList(func() int { return len(filas) },
		func() fyne.CanvasObject { return widget.NewLabel("plantilla") },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(filas[i]) })
	r := ReservarEnLista(l)
	w2 := test.NewTempWindow(t, l)
	w2.Resize(fyne.NewSize(390, 700))
	r.Ajustar(390)
	if got := l.Length(); got != len(filas)+1 {
		t.Fatalf("a 390 la lista tiene %d filas y con la reserva son %d", got, len(filas)+1)
	}
	hueco := widget.NewLabel("x")
	l.UpdateItem(len(filas), hueco)
	if hueco.Visible() {
		t.Error("la fila de la reserva se pinta")
	}
	l.Select(len(filas))
	l.UpdateItem(0, hueco)
	if !hueco.Visible() || hueco.Text != "Ana" {
		t.Error("la fila reciclada desde la reserva no vuelve a pintarse")
	}
	filas = append(filas, "Dora")
	r.Ajustar(390)
	if got := l.Length(); got != len(filas)+1 {
		t.Errorf("al crecer, la lista tiene %d filas y son %d", got, len(filas)+1)
	}
	r.Ajustar(1280)
	if got := l.Length(); got != len(filas)+1 {
		t.Errorf("a 1280 la lista tiene %d filas y el «+» también flota ahí: son %d", got, len(filas)+1)
	}
	r.Ajustar(0)
	if w2.Canvas().Size().Width > 0 && l.Length() != len(filas)+1 {
		t.Errorf("con el ancho de su lienzo la reserva se apaga: %d filas", l.Length())
	}
}
