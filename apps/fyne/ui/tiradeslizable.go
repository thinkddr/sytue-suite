// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// TiraDeslizable es una fila de botones cuadrados en `overflow-x-auto` SIN barra
// (`[scrollbar-width:none]`): lo que no cabe se RECORTA y se alcanza desplazando la
// tira —la rueda o el gesto horizontal, el arrastre en una pantalla táctil— o con el
// tabulador, que la desplaza hasta el botón enfocado como hace el navegador. Antes, en
// Mail, el botón que no cabía se escondía y no había forma de llegar a él.
//
// Recorta porque implementa `fyne.Scrollable`: el pintor de Fyne recorta los hijos de
// todo lo que se desplaza (`internal/driver/util.go`). Sin barra porque no es un
// `container.Scroll`, que la enseña al pasar por encima.
//
// La rueda VERTICAL no es suya: sobre una tira de una fila de lista, el navegador sigue
// desplazando la lista. Con AlDesplazarV se la pasa a quien la tenga; sin él, la usa
// para ir de lado, que es lo que hacen las tiras que no están dentro de nada.
//
// Los hijos son cuadrados de `Lado`; con AnchoPropio, cada uno mide el ancho que pide
// (la barra de formato de Mail, con desplegables entre los botones) y `Lado` es el alto
// mínimo. El alto real también respeta el de los hijos, incluidos los botones táctiles.
type TiraDeslizable struct {
	widget.BaseWidget

	Objetos     []fyne.CanvasObject
	Lado, Hueco float32
	// HuecoEntre sustituye a Hueco entre dos hijos visibles: los grupos de
	// formato tienen un hueco interior distinto del que separa las acciones.
	HuecoEntre   func(anterior, actual fyne.CanvasObject) float32
	AnchoPropio  bool
	AlDesplazarV func(*fyne.ScrollEvent)

	desX float32
}

var (
	_ fyne.Widget     = (*TiraDeslizable)(nil)
	_ fyne.Scrollable = (*TiraDeslizable)(nil)
	_ fyne.Draggable  = (*TiraDeslizable)(nil)
)

// NuevaTiraDeslizable crea la tira. Engancha el foco de los botones (de la suite o que
// lleven uno dentro) para enseñar el que lo gana.
func NuevaTiraDeslizable(lado, hueco float32, objs ...fyne.CanvasObject) *TiraDeslizable {
	t := &TiraDeslizable{Objetos: objs, Lado: lado, Hueco: hueco}
	t.ExtendBaseWidget(t)
	for _, o := range objs {
		encadenar := func(antes func()) func() {
			return func() {
				if antes != nil {
					antes()
				}
				t.Mostrar(o)
			}
		}
		switch b := o.(type) {
		case interface{ boton() *Boton }:
			b.boton().AlEnfocar = encadenar(b.boton().AlEnfocar)
		case *BotonIcono:
			b.AlEnfocar = encadenar(b.AlEnfocar)
		}
	}
	return t
}

func (t *TiraDeslizable) hueco(anterior, actual fyne.CanvasObject) float32 {
	if anterior == nil {
		return 0
	}
	if t.HuecoEntre != nil {
		return t.HuecoEntre(anterior, actual)
	}
	return t.Hueco
}

// Desplazamiento es cuánto se ha corrido la tira hacia la izquierda.
func (t *TiraDeslizable) Desplazamiento() float32 { return t.desX }

// ancho es lo que ocupa un hijo en la fila.
func (t *TiraDeslizable) ancho(o fyne.CanvasObject) float32 {
	if t.AnchoPropio {
		return medidaPropiaEnTira(o).Width
	}
	return SueloTactil(t.Lado)
}

// Los controles conservan su ancho propio y el suelo táctil de la tira. Los
// separadores y otras piezas decorativas mantienen sus medidas naturales.
func medidaPropiaEnTira(o fyne.CanvasObject) fyne.Size {
	m := o.MinSize()
	switch o.(type) {
	case interface{ boton() *Boton }, *BotonIcono:
		m.Width, m.Height = SueloTactil(m.Width), SueloTactil(m.Height)
	}
	return m
}

func (t *TiraDeslizable) alto() float32 {
	if !t.AnchoPropio {
		return SueloTactil(t.Lado)
	}
	alto := t.Lado
	for _, o := range t.Objetos {
		if o.Visible() {
			alto = max(alto, medidaPropiaEnTira(o).Height)
		}
	}
	return alto
}

// AnchoContenido es lo que miden los botones visibles puestos en fila.
func (t *TiraDeslizable) AnchoContenido() float32 {
	var x float32
	var anterior fyne.CanvasObject
	for _, o := range t.Objetos {
		if o.Visible() {
			x += t.hueco(anterior, o) + t.ancho(o)
			anterior = o
		}
	}
	return x
}

func (t *TiraDeslizable) mover(x float32) {
	x = min(max(x, 0), max(t.AnchoContenido()-t.Size().Width, 0))
	if x != t.desX {
		t.desX = x
		t.Refresh()
	}
}

// Scrolled corre la tira con el gesto horizontal; el vertical se lo pasa a quien lo
// quiera (AlDesplazarV) o, si no hay nadie, también la corre.
func (t *TiraDeslizable) Scrolled(e *fyne.ScrollEvent) {
	d := e.Scrolled.DX
	if d == 0 {
		if t.AlDesplazarV != nil {
			t.AlDesplazarV(e)
			return
		}
		d = e.Scrolled.DY
	}
	t.mover(t.desX - d)
}

// Dragged es el gesto táctil: arrastrar de lado corre la tira; arrastrar en vertical
// es desplazar la lista que la contiene, igual que la rueda.
func (t *TiraDeslizable) Dragged(e *fyne.DragEvent) {
	dx, dy := e.Dragged.DX, e.Dragged.DY
	if t.AlDesplazarV != nil && dy*dy > dx*dx {
		t.AlDesplazarV(&fyne.ScrollEvent{PointEvent: e.PointEvent, Scrolled: e.Dragged})
		return
	}
	t.mover(t.desX - dx)
}

// DragEnd no hace nada: la tira se queda donde se soltó.
func (t *TiraDeslizable) DragEnd() {}

// Mostrar desplaza lo justo para que o se vea entero, que es lo que hace el navegador
// al enfocar un botón recortado.
func (t *TiraDeslizable) Mostrar(o fyne.CanvasObject) {
	if !t.Visible() {
		return
	}
	x := float32(0)
	var anterior fyne.CanvasObject
	for _, h := range t.Objetos {
		if !h.Visible() {
			continue
		}
		x += t.hueco(anterior, h)
		w := t.ancho(h)
		if h == o {
			switch {
			case x < t.desX:
				t.mover(x)
			case x+w > t.desX+t.Size().Width:
				t.mover(x + w - t.Size().Width)
			}
			return
		}
		x += w
		anterior = h
	}
}

// MinSize no declara ancho: la tira se lleva el que le den (`max-w-full`).
func (t *TiraDeslizable) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(0, t.alto())
}

// CreateRenderer coloca los botones en fila, corridos lo desplazado.
func (t *TiraDeslizable) CreateRenderer() fyne.WidgetRenderer { return &tiraDeslizableRenderer{t: t} }

type tiraDeslizableRenderer struct{ t *TiraDeslizable }

func (r *tiraDeslizableRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *tiraDeslizableRenderer) Objects() []fyne.CanvasObject { return r.t.Objetos }
func (r *tiraDeslizableRenderer) Destroy()                     {}
func (r *tiraDeslizableRenderer) Refresh()                     { r.Layout(r.t.Size()) }

func (r *tiraDeslizableRenderer) Layout(tam fyne.Size) {
	t := r.t
	// Si la tira ha crecido o le han quitado botones, lo desplazado puede sobrar.
	t.desX = min(t.desX, max(t.AnchoContenido()-tam.Width, 0))
	x := -t.desX
	alto := t.alto()
	var anterior fyne.CanvasObject
	for _, o := range t.Objetos {
		if !o.Visible() {
			continue
		}
		w, h := t.ancho(o), alto
		if t.AnchoPropio {
			m := medidaPropiaEnTira(o)
			w, h = m.Width, m.Height
		}
		x += t.hueco(anterior, o)
		o.Move(fyne.NewPos(x, (tam.Height-h)/2))
		o.Resize(fyne.NewSize(w, h))
		x += w
		anterior = o
	}
}
