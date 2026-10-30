// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El folio con el dedo. En la web lo pone el navegador del teléfono: tocar coloca el
// cursor, tocar dos veces o mantener pulsado selecciona la palabra, y la selección se
// agranda arrastrando los dos tiradores de sus extremos; arrastrar en otro sitio desplaza.
// Aquí el toque, el doble toque y el desplazamiento ya estaban (`escribir.go`); esto son
// los tiradores y la pulsación larga.

// esMovil es [ui.EsTactil], la única pregunta de «¿teléfono?» de la suite; variable para
// que la prueba lo sea.
var esMovil = ui.EsTactil

// Medidas de un tirador de selección: el círculo de los de Android e iOS, y la zona donde
// lo coge un dedo, que es mayor que lo que se ve (WCAG 2.5.8 pide 24 como poco).
const (
	radioTiradorTexto = 7
	zonaTiradorTexto  = 22
)

// Cuál de los dos tiradores lleva el dedo: el del principio, el del final, o ninguno
// (el arrastre desplaza la página). sinDecidir es antes del primer movimiento.
const (
	sinDecidir = iota
	tiradorInicio
	tiradorFin
	sinTirador
)

// tiradoresTexto son los dos tiradores de la selección, en px de la página: bajo el pie
// de la raya de cada extremo. Solo en un táctil y con algo seleccionado.
func (f *Folio) tiradoresTexto() (ini, fin fyne.Position, ok bool) {
	if !esMovil() || !f.haySeleccion() {
		return ini, fin, false
	}
	a, b := f.orden()
	z := f.escala()
	ox, oy := f.pag.origen()
	en := func(q punto) (fyne.Position, bool) {
		x, y, alto, hay := f.caretEnMaqueta(q)
		return fyne.NewPos(ox+x*z, oy+(y+alto)*z+radioTiradorTexto), hay
	}
	ini, okA := en(a)
	fin, okB := en(b)
	return ini, fin, okA && okB
}

// pintarTiradoresTexto añade los dos círculos en `primary`, encima de todo.
func (f *Folio) pintarTiradoresTexto(objs []fyne.CanvasObject, v tema.Variante) []fyne.CanvasObject {
	ini, fin, ok := f.tiradoresTexto()
	if !ok {
		return objs
	}
	for _, c := range []fyne.Position{ini, fin} {
		t := canvas.NewCircle(tema.Color("primary", v))
		t.StrokeColor, t.StrokeWidth = color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 1.5
		t.Move(fyne.NewPos(c.X-radioTiradorTexto, c.Y-radioTiradorTexto))
		t.Resize(fyne.NewSize(2*radioTiradorTexto, 2*radioTiradorTexto))
		objs = append(objs, t)
	}
	return objs
}

// tiradorTextoEn dice qué tirador hay en pos (px de la página), si alguno.
func (f *Folio) tiradorTextoEn(pos fyne.Position) int {
	ini, fin, ok := f.tiradoresTexto()
	if !ok {
		return sinTirador
	}
	cerca := func(c fyne.Position) bool {
		dx, dy := pos.X-c.X, pos.Y-c.Y
		return dx*dx+dy*dy <= zonaTiradorTexto*zonaTiradorTexto
	}
	switch {
	case cerca(fin): // el del final gana si se tocan: es el que se suele agrandar
		return tiradorFin
	case cerca(ini):
		return tiradorInicio
	}
	return sinTirador
}

// arrastrarTactil es Dragged en un táctil: si el dedo empezó en un tirador, mueve ese
// extremo de la selección —el otro se queda quieto—; si no, desplaza la página.
func (f *Folio) arrastrarTactil(e *fyne.DragEvent) {
	if f.tiradorTexto == sinDecidir {
		f.tiradorTexto = f.tiradorTextoEn(e.Position.Subtract(e.Dragged))
	}
	if f.tiradorTexto == sinTirador {
		gesto := &fyne.ScrollEvent{PointEvent: e.PointEvent, Scrolled: e.Dragged}
		if f.caja != nil && f.caja.AlDesplazar != nil {
			f.caja.AlDesplazar(gesto)
		} else {
			f.scroll.Scrolled(gesto)
		}
		return
	}
	a, b := f.orden()
	quieto := a
	if f.tiradorTexto == tiradorInicio {
		quieto = b
	}
	// El tirador cuelga bajo la línea: lo que se señala es la línea de encima.
	q := f.enPunto(fyne.NewPos(e.Position.X, e.Position.Y-2*radioTiradorTexto))
	f.anc = quieto
	f.mover(q, true)
	f.hayObjetivo = false
	// Si se cruzan, el tirador que se lleva pasa a ser el otro extremo.
	if a2, _ := f.orden(); a2 == q {
		f.tiradorTexto = tiradorInicio
	} else {
		f.tiradorTexto = tiradorFin
	}
}

// TappedSecondary es la pulsación larga del táctil (y el clic derecho del escritorio):
// en un táctil selecciona la palabra, como el navegador del teléfono.
func (p *pagina) TappedSecondary(e *fyne.PointEvent) {
	if esMovil() {
		p.DoubleTapped(e)
	}
}
