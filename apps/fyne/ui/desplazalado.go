// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// DesplazaLado es `overflow-x-auto`: el contenido se desplaza de lado, recortado a la
// caja. NO es `container.NewHScroll`, que convierte la rueda vertical en lateral, pone
// una pista invisible (`scrollBarArea`) encima del contenido y se queda el gesto: encima
// de una tabla ancha la página dejaba de bajar. Aquí manda el eje dominante de cada gesto
// —la rueda, el trackpad (`fyne.Scrollable`) y, en el móvil, el dedo (`fyne.Draggable`,
// que es lo que el driver táctil busca y despacha al arrastrar)—: de lado, el contenido;
// en vertical, `Afuera`, que devuelve el desplazador de la página cuando llega.
//
// Nació en `apps/erp` para las tablas de un informe y subió aquí el 2026-10-05, cuando la
// rejilla de Sytue Bases necesitó lo mismo (`docs/CONVENTIONS.md` §13.2).
type DesplazaLado struct {
	widget.BaseWidget
	Contenido fyne.CanvasObject
	// Ancho es el del contenido. Con 0, el que pida su MinSize en cada momento.
	Ancho float32
	// Afuera devuelve quien se lleva lo vertical: el desplazador de la página
	// (`container.Scroll` lo es). Nil, o si devuelve nil, lo vertical desplaza de lado
	// con la rueda, como `HScroll`.
	Afuera func() fyne.Scrollable
	// AlDesplazar avisa cuando cambia el desplazamiento o el alcance del contenido (por
	// ejemplo, para encender o apagar `MASCARA_SCROLL_X`).
	AlDesplazar func()
	// Medir es el alto del contenido a un ancho, para el que envuelve en filas cuando
	// cabe (la barra de formato de Notepads en el escritorio). Nil: su mínimo.
	Medir func(ancho float32) float32

	x float32
}

var (
	_ fyne.Scrollable = (*DesplazaLado)(nil)
	_ fyne.Draggable  = (*DesplazaLado)(nil)
)

// NuevoDesplazaLado envuelve `o`, de `ancho`, para desplazarlo de lado.
func NuevoDesplazaLado(o fyne.CanvasObject, ancho float32, afuera func() fyne.Scrollable) *DesplazaLado {
	d := &DesplazaLado{Contenido: o, Ancho: ancho, Afuera: afuera}
	d.ExtendBaseWidget(d)
	return d
}

// X es cuánto se ha desplazado el contenido.
func (d *DesplazaLado) X() float32 { return d.x }

// AnchoContenido es lo que mide el contenido que se desplaza.
func (d *DesplazaLado) AnchoContenido() float32 { return d.ancho() }

func (d *DesplazaLado) ancho() float32 {
	if d.Ancho > 0 {
		return d.Ancho
	}
	return d.Contenido.MinSize().Width
}

// mover desplaza el contenido, sin salirse de sus bordes.
func (d *DesplazaLado) mover(dx float32) {
	x := min(max(d.x-dx, 0), max(d.ancho()-d.Size().Width, 0))
	if x != d.x {
		d.x = x
		d.Refresh()
		if d.AlDesplazar != nil {
			d.AlDesplazar()
		}
	}
}

// Mostrar desplaza lo justo para que el tramo que va de `x` a `x+ancho` del contenido
// quede a la vista: la celda a la que se llega con el teclado.
func (d *DesplazaLado) Mostrar(x, ancho float32) {
	switch {
	case x < d.x:
		d.mover(d.x - x)
	case x+ancho > d.x+d.Size().Width:
		d.mover(d.x + d.Size().Width - x - ancho)
	}
}

// Scrolled mueve el contenido si el gesto es de lado, y le pasa a la página el vertical.
func (d *DesplazaLado) Scrolled(e *fyne.ScrollEvent) {
	dx, dy := e.Scrolled.DX, e.Scrolled.DY
	if max(dx, -dx) > max(dy, -dy) {
		d.mover(dx)
		return
	}
	if d.Afuera != nil {
		if f := d.Afuera(); f != nil {
			f.Scrolled(e)
			return
		}
	}
	d.mover(dy)
}

// Dragged es el gesto táctil: en iOS y Android el driver entrega el dedo como arrastre
// sobre el `fyne.Draggable` alcanzado, no como `ScrollEvent`. De lado corre el contenido;
// en vertical se lo cede a `Afuera`, igual que la rueda.
func (d *DesplazaLado) Dragged(e *fyne.DragEvent) {
	dx, dy := e.Dragged.DX, e.Dragged.DY
	if max(dy, -dy) > max(dx, -dx) && d.Afuera != nil {
		if f := d.Afuera(); f != nil {
			f.Scrolled(&fyne.ScrollEvent{PointEvent: e.PointEvent, Scrolled: fyne.NewDelta(0, dy)})
			return
		}
	}
	d.mover(dx)
}

// DragEnd no hace nada: el contenido queda donde se soltó.
func (d *DesplazaLado) DragEnd() {}

// MinSize es el alto del contenido: el ancho lo pone la caja.
// AltoPara es lo que ocupa a ese ancho: lo que diga Medir, o el mínimo del contenido.
func (d *DesplazaLado) AltoPara(ancho float32) float32 {
	if d.Medir != nil {
		return d.Medir(max(ancho, d.ancho()))
	}
	return d.Contenido.MinSize().Height
}

func (d *DesplazaLado) MinSize() fyne.Size {
	d.ExtendBaseWidget(d)
	return fyne.NewSize(0, d.Contenido.MinSize().Height)
}

// CreateRenderer pone el contenido corrido y recortado.
func (d *DesplazaLado) CreateRenderer() fyne.WidgetRenderer { return &desplazaLadoRenderer{d: d} }

type desplazaLadoRenderer struct{ d *DesplazaLado }

// IsClip es el recorte de `overflow-x-auto`.
func (r *desplazaLadoRenderer) IsClip() {}

func (r *desplazaLadoRenderer) MinSize() fyne.Size { return r.d.MinSize() }
func (r *desplazaLadoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.d.Contenido}
}
func (r *desplazaLadoRenderer) Destroy() {}

func (r *desplazaLadoRenderer) Layout(t fyne.Size) {
	ancho := r.d.ancho()
	x := min(r.d.x, max(ancho-t.Width, 0))
	cambio := x != r.d.x
	r.d.x = x
	r.d.Contenido.Resize(fyne.NewSize(max(ancho, t.Width), t.Height))
	r.d.Contenido.Move(fyne.NewPos(-r.d.x, 0))
	if cambio && r.d.AlDesplazar != nil {
		r.d.AlDesplazar()
	}
}

func (r *desplazaLadoRenderer) Refresh() { r.Layout(r.d.Size()); canvas.Refresh(r.d.Contenido) }
