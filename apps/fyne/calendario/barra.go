// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La barra de un evento de VARIOS días o de día completo: `MonthBar` de CalendarGrid.tsx
// y `AllDayBar` de TimeGrid.tsx, que son la misma pieza en dos sitios:
//
//	absolute flex items-center px-1.5 rounded-[3px] hover:brightness-95
//	height: 18px · background: el color del evento · color: barTextColor(relleno)
//	<span class="text-2xs truncate leading-none font-medium">
//
// En los cortes de semana el borde va recto y sin sangría, y el tramo que viene de una
// semana anterior no lleva texto: el título se lee una vez, en el primer tramo.
const (
	// SangriaBarra son los 3 px que se aparta del borde de la celda donde NO hay corte.
	SangriaBarra float32 = 3
	// RadioBarra es el `rounded-[3px]`.
	RadioBarra float32 = 3
	// BrilloEncima es el `hover:brightness-95`.
	BrilloEncima = 0.95
)

// ocurrenciaInstantes son el inicio y el fin de la ocurrencia en la zona de
// visualización. Un evento de día completo es FLOTANTE —el servicio lo guarda a las
// 00:00 UTC— y cae en el mismo día natural en cualquier zona: es el `eventInstant` de la
// web para un valor DATE, que da la medianoche LOCAL de esa fecha.
func ocurrenciaInstantes(o Ocurrencia, loc *time.Location) (inicio, fin time.Time) {
	if !o.Event.AllDay {
		inicio = o.Start.In(loc)
		if !o.End.IsZero() {
			fin = o.End.In(loc)
		}
		return inicio, fin
	}
	flotante := func(t time.Time) time.Time {
		u := t.UTC()
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, loc)
	}
	inicio = flotante(o.Start)
	if !o.End.IsZero() {
		fin = flotante(o.End)
	}
	return inicio, fin
}

// barraEvento es un tramo de barra ya colocado.
type barraEvento struct {
	widget.BaseWidget

	t        Tramo[Ocurrencia]
	rotulo   string // el título, o «(sin título)»
	nombre   string // el `aria-label`, con «(viene de antes)» si toca
	alPulsar func(Ocurrencia, fyne.CanvasObject)

	encima, enfocada bool
}

var (
	_ fyne.Tappable     = (*barraEvento)(nil)
	_ fyne.Focusable    = (*barraEvento)(nil)
	_ desktop.Hoverable = (*barraEvento)(nil)
)

func nuevaBarraEvento(t Tramo[Ocurrencia], rotulo, nombre string, alPulsar func(Ocurrencia, fyne.CanvasObject)) *barraEvento {
	b := &barraEvento{t: t, rotulo: rotulo, nombre: nombre, alPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

// Tapped abre la vista rápida; el clic no baja a la celda (`stopPropagation`).
func (b *barraEvento) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar(b.t.Item, b)
	}
}

func (b *barraEvento) FocusGained()                   { b.enfocada = true; b.Refresh() }
func (b *barraEvento) FocusLost()                     { b.enfocada = false; b.Refresh() }
func (b *barraEvento) TypedRune(rune)                 {}
func (b *barraEvento) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *barraEvento) MouseMoved(*desktop.MouseEvent) {}
func (b *barraEvento) MouseOut()                      { b.encima = false; b.Refresh() }
func (b *barraEvento) TypedKey(e *fyne.KeyEvent)      { pulsarConTecla(b, e, func() { b.Tapped(nil) }) }

func (b *barraEvento) AccessibilityLabel() string             { return b.nombre }
func (b *barraEvento) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (b *barraEvento) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &barraRenderer{
		b:     b,
		fondo: canvas.NewRectangle(b.t.Item.Color),
		texto: ui.Texto(b.rotulo, tema.Texto2XS, ui.Medio, TintaSobre(b.t.Item.Color)),
		aro:   ui.NuevoAro(RadioBarra, 0, v),
	}
	r.texto.Hidden = b.t.SigueIzq
	r.objetos = r.aro.Antes(r.fondo, r.texto)
	r.Refresh()
	return r
}

type barraRenderer struct {
	b       *barraEvento
	fondo   *canvas.Rectangle
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *barraRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoBarra) }
func (r *barraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraRenderer) Destroy()                     {}

// Layout: `px-1.5` y el texto centrado en vertical por su caja de `leading-none`, que
// en la web es el propio tamaño de letra.
func (r *barraRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	ui.Truncar(r.texto, r.b.rotulo, tema.Texto2XS, ui.Medio, max(t.Width-2*espacio1p5, 0))
	caja := tema.Texto2XS.Tam // `leading-none`
	ui.ColocarTexto(r.texto, tema.Escalon{Tam: caja, Linea: caja}, espacio1p5, (t.Height-caja)/2)
	r.aro.Colocar(t)
}

// Refresh pinta el relleno (un 5 % más oscuro con el ratón encima) y los radios: rectos
// en los cortes de semana.
func (r *barraRenderer) Refresh() {
	c := r.b.t.Item.Color
	if r.b.encima {
		c = oscurecer(c, BrilloEncima)
	}
	r.fondo.FillColor = c
	izq, der := RadioBarra, RadioBarra
	if r.b.t.SigueIzq {
		izq = 0
	}
	if r.b.t.SigueDer {
		der = 0
	}
	r.fondo.TopLeftCornerRadius, r.fondo.BottomLeftCornerRadius = izq, izq
	r.fondo.TopRightCornerRadius, r.fondo.BottomRightCornerRadius = der, der
	r.fondo.Refresh()
	r.texto.Color = TintaSobre(r.b.t.Item.Color)
	r.texto.Refresh()
	r.aro.Ver(r.b.enfocada)
}

// oscurecer es `filter: brightness(f)`: cada canal por el factor.
func oscurecer(c color.NRGBA, f float32) color.NRGBA {
	m := func(x uint8) uint8 { return uint8(float32(x)*f + 0.5) }
	return color.NRGBA{R: m(c.R), G: m(c.G), B: m(c.B), A: c.A}
}

// colocarBarra pone un tramo en una fila de ancho `ancho` repartida en `cols` columnas,
// con la sangría de 3 donde no hay corte: `left: calc(col/cols·100% + inL)`.
func colocarBarra(b fyne.CanvasObject, t Tramo[Ocurrencia], x0, y, ancho float32, cols int) {
	izq, der := SangriaBarra, SangriaBarra
	if t.SigueIzq {
		izq = 0
	}
	if t.SigueDer {
		der = 0
	}
	x := x0 + float32(t.Col)/float32(cols)*ancho + izq
	w := float32(t.Ancho)/float32(cols)*ancho - izq - der
	b.Move(fyne.NewPos(x, y))
	b.Resize(fyne.NewSize(max(w, 0), AltoBarra))
}

// pulsarConTecla es Intro y Espacio sobre algo que se pulsa, como un `<button>`; Escape y
// el «atrás» del teléfono suben al diálogo de encima (`ui.SubirEscape`).
func pulsarConTecla(yo fyne.CanvasObject, e *fyne.KeyEvent, f func()) {
	if ui.SubirEscape(yo, e) {
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter || e.Name == fyne.KeySpace {
		f()
	}
}
