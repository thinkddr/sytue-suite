// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// BotonFijar es «Fijar» / «Soltar» de una tesela: `bg-neutral-1000/70 text-neutral-0
// absolute top-2 right-2 rounded-md px-2 py-1 text-xs font-medium`. Es un botón de
// verdad y no un clic sobre la tesela, que no se alcanza con el teclado ni lo anuncia un
// lector. La tesela lo enseña al pasar por encima, al enfocarlo o si ya está fijada
// (entonces tiene que verse para poder soltarla), y siempre en el móvil, que no tiene
// «encima».
type BotonFijar struct {
	widget.BaseWidget
	Rotulo, Nombre string
	AlPulsar       func()
	// AlCambio avisa a la tesela de que el puntero o el foco entraron o salieron: es el
	// `group-hover` de la web.
	AlCambio func()

	encima, enfocado bool
	fondo            *canvas.Rectangle
	texto            *canvas.Text
}

var (
	_ fyne.Tappable     = (*BotonFijar)(nil)
	_ fyne.Focusable    = (*BotonFijar)(nil)
	_ fyne.Accessible   = (*BotonFijar)(nil)
	_ desktop.Hoverable = (*BotonFijar)(nil)
)

// NuevoBotonFijar crea el botón.
func NuevoBotonFijar(rotulo, nombre string, alPulsar func()) *BotonFijar {
	b := &BotonFijar{Rotulo: rotulo, Nombre: nombre, AlPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

// Activo dice si el puntero o el foco están en él.
func (b *BotonFijar) Activo() bool { return b.encima || b.enfocado }

func (b *BotonFijar) Tapped(*fyne.PointEvent) {
	if b.AlPulsar != nil {
		b.AlPulsar()
	}
}

func (b *BotonFijar) cambio() {
	if b.AlCambio != nil {
		b.AlCambio()
	}
}

func (b *BotonFijar) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.cambio() }
func (b *BotonFijar) MouseMoved(*desktop.MouseEvent) {}
func (b *BotonFijar) MouseOut()                      { b.encima = false; b.cambio() }
func (b *BotonFijar) FocusGained()                   { b.enfocado = true; b.cambio() }
func (b *BotonFijar) FocusLost()                     { b.enfocado = false; b.cambio() }
func (b *BotonFijar) TypedRune(rune)                 {}

// TypedKey es Intro y Espacio, como un `<button>`.
func (b *BotonFijar) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter || k.Name == fyne.KeySpace {
		b.Tapped(nil)
		return
	}
	ui.SubirEscape(b, k)
}

// AccessibilityLabel es «Fijar: Ana», el `aria-label` de la web.
func (b *BotonFijar) AccessibilityLabel() string { return b.Rotulo + ": " + b.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (b *BotonFijar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// Medidas: `px-2 py-1 text-xs`.
var (
	MargenFijarX = tema.Espacio(2)
	MargenFijarY = tema.Espacio(1)
)

// MedidaBotonFijar es la caja del botón con ese rótulo.
func MedidaBotonFijar(rotulo string) fyne.Size {
	w := ui.Medir(rotulo, tema.TextoXS, ui.Medio).Width
	return fyne.NewSize(w+2*MargenFijarX, tema.TextoXS.Linea+2*MargenFijarY)
}

func (b *BotonFijar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.fondo = canvas.NewRectangle(conAlfa(tema.Color("neutral-1000", v), 0xb3))
	b.fondo.CornerRadius = tema.RadioMD
	b.texto = ui.Texto(b.Rotulo, tema.TextoXS, ui.Medio, tema.Color("neutral-0", v))
	return &botonFijarRenderer{b: b}
}

type botonFijarRenderer struct{ b *BotonFijar }

func (r *botonFijarRenderer) MinSize() fyne.Size { return MedidaBotonFijar(r.b.Rotulo) }
func (r *botonFijarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.fondo, r.b.texto}
}
func (r *botonFijarRenderer) Destroy() {}
func (r *botonFijarRenderer) Layout(t fyne.Size) {
	r.b.fondo.Resize(t)
	ui.ColocarTexto(r.b.texto, tema.TextoXS, MargenFijarX, MargenFijarY)
}

func (r *botonFijarRenderer) Refresh() {
	r.b.texto.Text = r.b.Rotulo
	r.b.texto.Refresh()
	r.Layout(r.b.Size())
}
