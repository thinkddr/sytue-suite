// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que la auditoría estética del 2026-11-24 vio en Photos en el teléfono.

// TestLasPestanasCabenEnElTelefono: las seis pestañas con su rótulo miden unos 600 px, y
// a 390 la fila deslizable cortaba «Compartid…» y escondía las dos últimas. En el
// teléfono la activa lleva su rótulo y las demás solo su icono, con el rótulo como
// nombre accesible; caben todas. Desde `sm`, todas con su rótulo.
//
// VISTO EN ROJO sin `SoloIcono`: a 390 la nav mide 617 para 358 de sitio.
func TestLasPestanasCabenEnElTelefono(t *testing.T) {
	v := vistaDePrueba(t, nil)
	for _, ancho := range []float32{390, 1280} {
		v.Ventana = ancho
		nav := Nav(v, SecCompartidos, nil)
		var pestanas []*PestanaNav
		ancho2 := float32(0)
		recorrerPestanas(nav, func(p *PestanaNav) {
			pestanas = append(pestanas, p)
			ancho2 += MedidaPestanaNav(p).Width
		})
		if len(pestanas) != 6 {
			t.Fatalf("a %v la nav tiene %d pestañas", ancho, len(pestanas))
		}
		ancho2 += 5 * HuecoNav
		for _, p := range pestanas {
			movil := ancho < ui.CorteSM
			if p.SoloIcono != (movil && !p.Activa) || p.AccessibilityLabel() == "" {
				t.Errorf("a %v «%s» (activa %v): solo icono %v", ancho, p.Rotulo, p.Activa, p.SoloIcono)
			}
		}
		// La caja de la galería deja `px-4` a cada lado en el teléfono.
		if sitio := ancho - 2*4*0.25*16; ancho < ui.CorteSM && ancho2 > sitio {
			t.Errorf("a %v la nav mide %v para %v de sitio", ancho, ancho2, sitio)
		}
	}
	// Solo icono, la pestaña es un cuadrado con el icono centrado y sin rótulo pintado.
	p := NuevaPestanaNav("Favoritas", iconos.Corazon, false, nil)
	p.SoloIcono = true
	w := test.NewTempWindow(t, p)
	w.Resize(MedidaPestanaNav(p))
	r := test.WidgetRenderer(p).(*pestanaNavRenderer)
	if m := MedidaPestanaNav(p); m.Width != 2*RellenoPestana+IconoNav || r.texto.Visible() {
		t.Errorf("solo icono la pestaña mide %v y su rótulo se ve: %v", m, r.texto.Visible())
	}
}

func recorrerPestanas(o fyne.CanvasObject, f func(*PestanaNav)) {
	switch x := o.(type) {
	case *PestanaNav:
		f(x)
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrerPestanas(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrerPestanas(h, f)
		}
	}
}

// TestElCorazonSoloEnLasFavoritasSinPuntero: con el dedo, cada miniatura llevaba su
// corazón (el `pointer-coarse:opacity-100` de la web) y todas iguales, huecos, también
// en las favoritas: recargaba la galería y no decía cuáles lo eran. Ahora la favorita
// lleva el corazón LLENO, como la web; la que no lo es, solo al pasar el ratón; y sin
// verse, tocar su esquina abre la foto.
//
// VISTO EN ROJO con el corazón de antes: la favorita lo pinta hueco y, en un teléfono, la
// que no lo es lo enseña y su esquina la marca.
func TestElCorazonSoloEnLasFavoritasSinPuntero(t *testing.T) {
	for _, c := range []struct {
		nombre       string
		favorita     bool
		encima       bool
		hueco, lleno bool
		marca, abre  int
	}{
		{"favorita", true, false, false, true, 1, 0},
		{"sin marcar", false, false, false, false, 0, 1},
		{"sin marcar con el ratón encima", false, true, true, false, 1, 0},
	} {
		m := NuevaMiniatura(fotosDePrueba(1)[0], nil)
		m.Foto.Favorite = c.favorita
		marcas, abiertas := 0, 0
		m.RotuloFavorita, m.AlFavorita, m.AlPulsar = "Favorita", func() { marcas++ }, func() { abiertas++ }
		w := test.NewTempWindow(t, m)
		w.Resize(fyne.NewSize(120, 120))
		if c.encima {
			m.MouseIn(nil)
		}
		r := test.WidgetRenderer(m).(*miniaturaRenderer)
		if r.corazon.Objeto().Visible() != c.hueco || r.corazonLleno.Objeto().Visible() != c.lleno {
			t.Errorf("%s: corazón hueco %v y lleno %v", c.nombre, r.corazon.Objeto().Visible(), r.corazonLleno.Objeto().Visible())
		}
		m.Tapped(&fyne.PointEvent{Position: fyne.NewPos(8, 112)})
		if marcas != c.marca || abiertas != c.abre {
			t.Errorf("%s: tocar la esquina marca %d y abre %d", c.nombre, marcas, abiertas)
		}
	}
}
