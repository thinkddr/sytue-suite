// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// filaLeyenda es una leyenda en el desplegable de «Referencia cruzada»: `flex w-full
// flex-col items-start gap-0.5 rounded-sm px-2 py-1 text-left hover:bg-bg-subtle`, con la
// etiqueta en `text-xs font-medium` y el texto, si lo hay, en `sytue-meta-dense`.
type filaLeyenda struct {
	widget.BaseWidget
	l                rico.Leyenda
	alPulsar         func()
	encima, enfocado bool
}

var (
	_ fyne.Tappable     = (*filaLeyenda)(nil)
	_ fyne.Focusable    = (*filaLeyenda)(nil)
	_ desktop.Hoverable = (*filaLeyenda)(nil)
	_ fyne.Accessible   = (*filaLeyenda)(nil)
)

func nuevaFilaLeyenda(l rico.Leyenda, alPulsar func()) *filaLeyenda {
	f := &filaLeyenda{l: l, alPulsar: alPulsar}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaLeyenda) Tapped(*fyne.PointEvent)        { f.alPulsar() }
func (f *filaLeyenda) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaLeyenda) MouseMoved(*desktop.MouseEvent) {}
func (f *filaLeyenda) MouseOut()                      { f.encima = false; f.Refresh() }
func (f *filaLeyenda) FocusGained()                   { f.enfocado = true; f.Refresh() }
func (f *filaLeyenda) FocusLost()                     { f.enfocado = false; f.Refresh() }
func (f *filaLeyenda) TypedRune(r rune) {
	if r == ' ' {
		f.alPulsar()
	}
}

func (f *filaLeyenda) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		f.alPulsar()
	}
}
func (f *filaLeyenda) AccessibilityLabel() string             { return f.l.Etiqueta }
func (f *filaLeyenda) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (f *filaLeyenda) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	alto := tema.TextoXS.Linea + 2*tema.Espacio(1)
	if f.l.Texto != "" {
		alto += tema.Espacio(0.5) + tema.Texto2XS.Linea
	}
	return fyne.NewSize(AnchoReferencias-2*tema.Espacio(1), alto)
}

func (f *filaLeyenda) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaLeyendaRenderer{
		f:        f,
		caja:     ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM),
		aro:      ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		etiqueta: ui.Texto(f.l.Etiqueta, tema.TextoXS, ui.Medio, tema.Color("text", v)),
		texto:    ui.Texto(f.l.Texto, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.etiqueta, r.texto)...)
	return r
}

type filaLeyendaRenderer struct {
	f               *filaLeyenda
	caja            *ui.Caja
	aro             *ui.Aro
	etiqueta, texto *canvas.Text
	objs            []fyne.CanvasObject
}

func (r *filaLeyendaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaLeyendaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaLeyendaRenderer) Destroy()                     {}

func (r *filaLeyendaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x, y := tema.Espacio(2), tema.Espacio(1)
	ui.Truncar(r.etiqueta, r.f.l.Etiqueta, tema.TextoXS, ui.Medio, max(t.Width-2*x, 0))
	ui.ColocarTexto(r.etiqueta, tema.TextoXS, x, y)
	ui.Truncar(r.texto, r.f.l.Texto, tema.Texto2XS, ui.Normal, max(t.Width-2*x, 0))
	ui.ColocarTexto(r.texto, tema.Texto2XS, x, y+tema.TextoXS.Linea+tema.Espacio(0.5))
}

func (r *filaLeyendaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.encima {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.texto.Hidden = r.f.l.Texto == ""
	r.aro.Pintar(v)
	r.aro.Ver(r.f.enfocado)
	if s := r.f.Size(); s.Width > 0 {
		r.Layout(s)
	}
}
