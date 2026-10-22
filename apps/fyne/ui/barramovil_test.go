// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestLaBarraMovilEsLaDeAppShell: `h-11` (38,5) con el botón de 28 a `px-4`, centrado
// en lo que deja el filete; la hamburguesa con la lista a la vista y «volver» con el
// detalle, que es cuando sale el título.
func TestLaBarraMovilEsLaDeAppShell(t *testing.T) {
	b := NuevaBarraMovil()
	b.Titulo, b.RotuloAbrir, b.RotuloVolver = "Contactos", "Abrir navegación", "Volver a la lista"
	b.AlMenu, b.ConLista = func() {}, true // hay menú y hay lista, como en Contactos
	if got := MedidaBarraMovil().Height; got != 44 {
		t.Errorf("la barra mide %v y `h-11` son 38,5", got)
	}
	b.Resize(fyne.NewSize(390, 44))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	if p := r.boton.Position(); p != fyne.NewPos(16, (44.0-1-32)/2) {
		t.Errorf("el botón está en %v y va a `px-4`, centrado", p)
	}
	if r.boton.Icono != iconos.Menu || r.boton.Rotulo != "Abrir navegación" || r.titulo.Text != "" {
		t.Errorf("con la lista: icono %v, rótulo %q, título %q", r.boton.Icono, r.boton.Rotulo, r.titulo.Text)
	}
	b.EnDetalle = true
	b.Refresh()
	if r.boton.Icono != iconos.ChevronIzquierda || r.boton.Rotulo != "Volver a la lista" || r.titulo.Text != "Contactos" {
		t.Errorf("con el detalle: icono %v, rótulo %q, título %q", r.boton.Icono, r.boton.Rotulo, r.titulo.Text)
	}
	if x := r.titulo.Position().X; x != 16+32+6 {
		t.Errorf("el título empieza en %v y va tras el botón y el `gap-1.5` (47,25)", x)
	}
}

func TestLaBarraMovilCambiaElDibujoDeMenuAVolver(t *testing.T) {
	b := NuevaBarraMovil()
	b.AlMenu = func() {}
	b.Resize(fyne.NewSize(390, AltoBarraMovil))
	r := test.WidgetRenderer(b).(*barraMovilRenderer)
	rb := test.WidgetRenderer(r.boton).(*botonIconoRenderer)
	menu := rb.ico.Objeto()
	b.EnDetalle = true
	b.Refresh()
	if rb.ico.Objeto() == menu {
		t.Fatal("la acción es volver pero sigue pintado el icono de menú")
	}
	quiero := NuevoBotonIcono(iconos.ChevronIzquierda, "Volver", nil)
	quiero.Lado, quiero.TamIcono = r.boton.Lado, r.boton.TamIcono
	quiero.Trazo, quiero.TrazoEncima = r.boton.Trazo, r.boton.TrazoEncima
	quiero.Resize(r.boton.Size())
	actual := pinta(t, quiero.Lado, quiero.Lado, rb.ico.Objeto())
	esperado := pinta(t, quiero.Lado, quiero.Lado, test.WidgetRenderer(quiero).(*botonIconoRenderer).ico.Objeto())
	for y := range actual.Bounds().Dy() {
		for x := range actual.Bounds().Dx() {
			if px(actual, x, y) != px(esperado, x, y) {
				t.Fatalf("el botón de volver no pinta el galón de la izquierda en (%d,%d)", x, y)
			}
		}
	}
}

// TestElBotonFlotanteSoloEnElTelefono: 49 × 49 (`h-14`) a 14 de la esquina de abajo a
// la derecha por debajo de 768, y escondido desde ahí (`md:hidden`).
func TestElBotonFlotanteSoloEnElTelefono(t *testing.T) {
	fab := NuevoBotonFlotante("Nuevo contacto", iconos.Mas, nil)
	capa := container.New(CapaFlotante{}, fab)
	capa.Resize(fyne.NewSize(390, 844))
	if !fab.Visible() || fab.Position() != fyne.NewPos(390-16-56, 844-16-56) || fab.Size() != fyne.NewSize(56, 56) {
		t.Errorf("en el teléfono: visible %v en %v de %v", fab.Visible(), fab.Position(), fab.Size())
	}
	capa.Resize(fyne.NewSize(1280, 800))
	if fab.Visible() {
		t.Error("en ancho el botón flotante sigue a la vista y es `md:hidden`")
	}
}
