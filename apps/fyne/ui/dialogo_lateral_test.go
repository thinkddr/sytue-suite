// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Modal placement=right: md:p-4, items-stretch, max-h-full. En el teléfono es la hoja de
// todos los diálogos: a todo el ancho, apoyada abajo y con `2.5rem` libres arriba.
// Son las cajas CSS independientes del contenido de los formularios.
func TestElPanelDerechoLlenaElAltoYConservaSuPie(t *testing.T) {
	for _, c := range []struct {
		lienzo fyne.Size
		pos    fyne.Position
		tam    fyne.Size
		// pie es la distancia del pie al borde de abajo: `pb-5` y el filete del panel,
		// que la hoja no lleva.
		pie float32
	}{
		{fyne.NewSize(1280, 800), fyne.NewPos(704, 16), fyne.NewSize(560, 768), 1 + 20},
		{fyne.NewSize(768, 800), fyne.NewPos(192, 16), fyne.NewSize(560, 768), 1 + 20},
		{fyne.NewSize(390, 740), fyne.NewPos(0, 40), fyne.NewSize(390, 700), 20},
	} {
		w := test.NewTempWindow(t, canvas.NewRectangle(tema.Color("bg", tema.Claro)))
		w.Resize(c.lienzo)
		pie := NuevoBoton("Guardar", nil)
		d := NuevoDialogo("Nuevo · Producto", cuerpoDe(3000)).Con(DialogoMD, pie).ALaDerecha()
		d.Mostrar(w.Canvas())
		if d.Position() != c.pos || d.Size() != c.tam {
			t.Errorf("viewport %v: panel %v/%v, CSS %v/%v", c.lienzo, d.Position(), d.Size(), c.pos, c.tam)
		}
		// `rounded-card`: el radio de las tarjetas de la forma, no el 20 de antes.
		if d.radio(c.lienzo) != tema.Forma("radius-card") {
			t.Errorf("viewport %v: radio=%v y el de tarjeta es %v", c.lienzo, d.radio(c.lienzo), tema.Forma("radius-card"))
		}
		quiero := c.tam.Height - c.pie - pie.Size().Height
		if got := pie.Position().Y; got != quiero {
			t.Errorf("viewport %v: pie y=%v, CSS=%v", c.lienzo, got, quiero)
		}
		d.Desplazador().ScrollToBottom()
		if got := pie.Position().Y; got != quiero {
			t.Errorf("el scroll del cuerpo ha movido el pie fijo a%v", got)
		}
		d.Cerrar()
	}
}

func TestLaEntradaLateralSeFundeSinEscalar(t *testing.T) {
	w := test.NewTempWindow(t, canvas.NewRectangle(tema.Color("bg", tema.Claro)))
	w.Resize(fyne.NewSize(1280, 800))
	d := NuevoDialogo("Nuevo", cuerpoDe(30)).ALaDerecha()
	d.Resize(fyne.NewSize(560, 768))
	d.Move(fyne.NewPos(704, 16))
	f := nuevoFundido(w.Canvas(), d, nil, 0, 1)
	f.dx = 16
	f.fotograma(0)
	if f.img.Position() != fyne.NewPos(720, 16) || f.img.Size() != fyne.NewSize(560, 768) {
		t.Errorf("primer fotograma %v/%v; CSS translateX16 sin escala", f.img.Position(), f.img.Size())
	}
	f.fotograma(0.5)
	if f.img.Position() != fyne.NewPos(712, 16) || f.img.Translucency != 0.5 {
		t.Errorf("medio fotograma %v alfa%v; CSS translateX8/opacidad.5", f.img.Position(), f.img.Translucency)
	}
	f.acabar()
	if !d.Visible() || d.Position() != fyne.NewPos(704, 16) {
		t.Error("la entrada no devolvió el panel real a su sitio")
	}
}
