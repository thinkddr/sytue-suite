// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Pasos es el `Stepper` de `Wizard.tsx`: el progreso de un asistente. Cada paso es un
// círculo con su número —un tic si ya está hecho— y su rótulo, unidos por un filete que
// se tiñe al avanzar.
//
// Medido en producción en «Conecta tus dispositivos» (1280 de ventana, 854 de ancho):
// 24,5 de alto; el último paso ocupa lo suyo (113 con «Comprobación») y los otros dos se
// reparten el resto, 363,5 cada uno, con `gap-2`. A 390 se esconden todos los rótulos
// (`hidden sm:inline` y, el del actual, `max-sm:sr-only`): el último mide 28.
type Pasos struct {
	widget.BaseWidget

	Titulos []string
	Actual  int
	// Ventana es el ancho de la VENTANA, que es el que decide el `sm:`. Cero es ancha.
	Ventana float32
	// AlElegir vuelve a un paso ya hecho. Nil: el progreso no es interactivo, que es lo
	// que hace la web con un paso terminal o mientras el asistente trabaja.
	AlElegir func(int)
	// RotuloIr es el `goToStep` que se anuncia en un paso hecho («Volver al paso 1: »).
	RotuloIr func(n int) string

	marcas []*marcaPaso
}

var (
	_ fyne.Widget      = (*Pasos)(nil)
	_ fyne.Disableable = (*marcaPaso)(nil)
)

// Medidas de Wizard.tsx.
const (
	// LadoCirculoPaso es el `h-6 w-6` del círculo; y MargenMarcaX y MargenMarcaY el
	// `px-1 py-0.5` de la marca.
	LadoCirculoPaso float32 = 6 * 0.25 * tema.Rem
	MargenMarcaX    float32 = 1 * 0.25 * tema.Rem
	MargenMarcaY    float32 = 0.5 * 0.25 * tema.Rem
	// HuecoPasos es el `gap-2` de la lista, de cada paso y de la marca.
	HuecoPasos float32 = 2 * 0.25 * tema.Rem
	// MinFilete es el `min-w-4` del filete.
	MinFilete float32 = 4 * 0.25 * tema.Rem
	// TicPaso es el `<Icon name="check" size={14}>` de un paso hecho.
	TicPaso float32 = 14
)

// AltoPasos es el alto de la barra: el círculo con el `py-0.5` de la marca.
var AltoPasos = LadoCirculoPaso + 2*MargenMarcaY

// NuevosPasos crea el progreso.
func NuevosPasos(titulos ...string) *Pasos {
	p := &Pasos{Titulos: titulos}
	p.ExtendBaseWidget(p)
	return p
}

// MinSize: la barra no declara ancho —ocupa el de su columna— y mide 24,5 de alto.
func (p *Pasos) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(0, AltoPasos)
}

// estrecha es el `sm:` de la ventana.
func (p *Pasos) estrecha() bool { return p.Ventana > 0 && p.Ventana < CorteSM }

// conRotulo dice si el paso i enseña su rótulo: todos desde `sm:` y ninguno por debajo.
// El del actual también se esconde (`max-sm:sr-only` en la web): tenía 45,5 de sitio y
// pedía 83–110, y el título del paso ya va debajo. Lo sigue anunciando su nombre.
func (p *Pasos) conRotulo(int) bool { return !p.estrecha() }

// AnchoMarca es lo que ocupa la marca del paso i: el círculo y, si lo lleva, el rótulo,
// con el `px-1` a cada lado.
func (p *Pasos) AnchoMarca(i int) float32 {
	w := 2*MargenMarcaX + LadoCirculoPaso
	if p.conRotulo(i) {
		w += HuecoPasos + Medir(p.Titulos[i], tema.TextoSM, p.pesoRotulo(i)).Width
	}
	return w
}

func (p *Pasos) pesoRotulo(i int) Peso {
	if i == p.Actual {
		return SemiNegrita
	}
	return Normal
}

// Tramos son el inicio y el ancho de cada paso a lo ancho de la barra: el último a su
// medida (`last:flex-none`) y los demás a partes iguales (`flex-1`), con `gap-2`.
func (p *Pasos) Tramos(ancho float32) (x, w []float32) {
	n := len(p.Titulos)
	if n == 0 {
		return nil, nil
	}
	ultimo := p.AnchoMarca(n - 1)
	resto := max(ancho-ultimo-float32(n-1)*HuecoPasos, 0) / float32(max(n-1, 1))
	pos := float32(0)
	for i := range n {
		ancho := resto
		if i == n-1 {
			ancho = ultimo
		}
		x, w = append(x, pos), append(w, ancho)
		pos += ancho + HuecoPasos
	}
	return x, w
}

// CreateRenderer compone las marcas y los filetes.
func (p *Pasos) CreateRenderer() fyne.WidgetRenderer {
	r := &pasosRenderer{p: p}
	r.Refresh()
	return r
}

type pasosRenderer struct {
	p       *Pasos
	filetes []*canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *pasosRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pasosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pasosRenderer) Destroy()                     {}

func (r *pasosRenderer) Layout(t fyne.Size) {
	p := r.p
	xs, ws := p.Tramos(t.Width)
	for i, m := range p.marcas {
		if i >= len(xs) {
			break
		}
		wm := p.AnchoMarca(i)
		if i < len(xs)-1 {
			// `min-w-0` en la marca y `min-w-4` en el filete: sin sitio, cede la marca
			// y su rótulo se recorta (`truncate`), no el filete.
			wm = max(min(wm, ws[i]-HuecoPasos-MinFilete), 2*MargenMarcaX+LadoCirculoPaso)
		}
		m.Move(fyne.NewPos(xs[i], (t.Height-AltoPasos)/2))
		m.Resize(fyne.NewSize(wm, AltoPasos))
		if i < len(r.filetes) {
			// `h-px flex-1`, centrado en el paso (`items-center`).
			inicio := xs[i] + wm + HuecoPasos
			r.filetes[i].Move(fyne.NewPos(inicio, (t.Height-tema.Borde)/2))
			r.filetes[i].Resize(fyne.NewSize(max(xs[i]+ws[i]-inicio, MinFilete), tema.Borde))
		}
	}
}

// Refresh rehace las marcas si ha cambiado el número de pasos y repinta todo.
func (r *pasosRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(p.marcas) != len(p.Titulos) {
		p.marcas, r.filetes = nil, nil
		for i := range p.Titulos {
			p.marcas = append(p.marcas, nuevaMarcaPaso(p, i))
			if i < len(p.Titulos)-1 {
				r.filetes = append(r.filetes, canvas.NewRectangle(color.NRGBA{}))
			}
		}
		r.objetos = nil
		for _, f := range r.filetes {
			r.objetos = append(r.objetos, f)
		}
		for _, m := range p.marcas {
			r.objetos = append(r.objetos, m)
		}
	}
	for i, f := range r.filetes {
		token := "border"
		if i < p.Actual {
			token = "primary"
		}
		f.FillColor = tema.Color(token, v)
		f.Refresh()
	}
	for _, m := range p.marcas {
		m.Refresh()
	}
	r.Layout(p.Size())
}

// marcaPaso es el círculo con su rótulo. Un paso HECHO es un botón —se puede volver a
// él con el teclado—; los otros dos, texto.
type marcaPaso struct {
	Pulsable
	p *Pasos
	i int
}

func nuevaMarcaPaso(p *Pasos, i int) *marcaPaso {
	m := &marcaPaso{p: p, i: i}
	m.AlPulsar = func() {
		if m.hecho() && p.AlElegir != nil {
			p.AlElegir(i)
		}
	}
	m.ExtendBaseWidget(m)
	return m
}

func (m *marcaPaso) hecho() bool { return m.i < m.p.Actual }

// Disabled saca de la tabulación los pasos que no se pueden pulsar: en la web son un
// `<span>`, no un botón. Enable y Disable no hacen nada porque quien lo decide es el
// asistente, no el paso.
func (m *marcaPaso) Disabled() bool { return !m.hecho() || m.p.AlElegir == nil }

// Enable no hace nada: ver Disabled.
func (m *marcaPaso) Enable() {}

// Disable no hace nada: ver Disabled.
func (m *marcaPaso) Disable() {}

// FocusGained solo enciende el anillo si el paso se puede pulsar: los demás son texto.
func (m *marcaPaso) FocusGained() {
	if !m.Disabled() {
		m.Pulsable.FocusGained()
	}
}

// AccessibilityLabel es lo que dice la web: «Volver al paso 1: Qué usas» en uno hecho y
// el rótulo en los demás.
func (m *marcaPaso) AccessibilityLabel() string {
	titulo := m.p.Titulos[m.i]
	if m.hecho() && m.p.RotuloIr != nil {
		return m.p.RotuloIr(m.i+1) + titulo
	}
	return titulo
}

// AccessibilityRole: botón el que se puede pulsar, texto el resto.
func (m *marcaPaso) AccessibilityRole() fyne.AccessibleRole {
	if !m.Disabled() {
		return fyne.AccessibleRoleButton
	}
	return fyne.AccessibleRoleText
}

func (m *marcaPaso) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &marcaRenderer{
		m:      m,
		aro:    NuevoAro(tema.RadioMD, ConHueco, v).Abierto(),
		fondo:  canvas.NewCircle(color.NRGBA{}),
		dentro: canvas.NewCircle(color.NRGBA{}),
		numero: Texto(fmt.Sprint(m.i+1), tema.TextoXS, SemiNegrita, color.NRGBA{}),
		tic:    iconos.Trazo(iconos.Tic, TicPaso, "on-primary", v),
		rotulo: Texto(m.p.Titulos[m.i], tema.TextoSM, Normal, color.NRGBA{}),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo, r.dentro, r.numero, r.tic.Objeto(), r.rotulo)
	r.Refresh()
	return r
}

type marcaRenderer struct {
	m              *marcaPaso
	aro            *Aro
	fondo, dentro  *canvas.Circle
	numero, rotulo *canvas.Text
	tic            *iconos.Dibujo
	objetos        []fyne.CanvasObject
}

func (r *marcaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.m.p.AnchoMarca(r.m.i), AltoPasos)
}
func (r *marcaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *marcaRenderer) Destroy()                     {}

// grosor es el borde del círculo: `border-2` el actual, `border` el pendiente y ninguno
// el hecho, que es un círculo lleno.
func (r *marcaRenderer) grosor() float32 {
	switch {
	case r.m.i == r.m.p.Actual:
		return 2
	case r.m.hecho():
		return 0
	}
	return tema.Borde
}

func (r *marcaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	x, y := MargenMarcaX, MargenMarcaY
	r.fondo.Move(fyne.NewPos(x, y))
	r.fondo.Resize(fyne.NewSize(LadoCirculoPaso, LadoCirculoPaso))
	g := r.grosor()
	r.dentro.Move(fyne.NewPos(x+g, y+g))
	r.dentro.Resize(fyne.NewSize(LadoCirculoPaso-2*g, LadoCirculoPaso-2*g))
	// `grid place-items-center`: el número y el tic, en el centro del círculo.
	w := Medir(r.numero.Text, tema.TextoXS, SemiNegrita).Width
	ColocarTexto(r.numero, tema.TextoXS, x+(LadoCirculoPaso-w)/2, y+(LadoCirculoPaso-tema.TextoXS.Linea)/2)
	r.tic.Colocar(x+(LadoCirculoPaso-TicPaso)/2, y+(LadoCirculoPaso-TicPaso)/2, TicPaso)
	cabe := t.Width - 2*MargenMarcaX - LadoCirculoPaso - HuecoPasos
	Truncar(r.rotulo, r.m.p.Titulos[r.m.i], tema.TextoSM, r.m.p.pesoRotulo(r.m.i), cabe)
	ColocarTexto(r.rotulo, tema.TextoSM, x+LadoCirculoPaso+HuecoPasos, (t.Height-tema.TextoSM.Linea)/2)
}

// Refresh pinta los tres estados de la web:
//
//	hecho      `bg-primary` con el tic en `on-primary`; rótulo `text-muted`
//	actual     `border-2 border-primary`, número `primary-text`; rótulo `text` 600
//	pendiente  `border border-border-strong`, número `text-subtle`; rótulo `text-subtle`
func (r *marcaRenderer) Refresh() {
	m, p := r.m, r.m.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	superficie := tema.Color("bg", v)
	borde, tinta, rotulo := tema.Color("border-strong", v), tema.Color("text-subtle", v), tema.Color("text-subtle", v)
	switch {
	case m.hecho():
		borde, rotulo = tema.Color("primary", v), tema.Color("text-muted", v)
		superficie = borde
	case m.i == p.Actual:
		borde, tinta, rotulo = tema.Color("primary", v), tema.Color("primary-text", v), tema.Color("text", v)
	}
	if m.hecho() && m.Encima() {
		rotulo = tema.Color("text", v) // `hover:text-text` del botón
	}
	r.fondo.FillColor, r.dentro.FillColor = borde, superficie
	r.fondo.Refresh()
	r.dentro.Refresh()
	r.numero.Color = tinta
	ver(r.numero, !m.hecho())
	r.numero.Refresh()
	r.tic.Ver(m.hecho())
	r.rotulo.Text, r.rotulo.Color = p.Titulos[m.i], rotulo
	r.rotulo.FontSource = p.pesoRotulo(m.i).cara()
	ver(r.rotulo, p.conRotulo(m.i))
	r.rotulo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(m.Enfocado())
	r.Layout(m.Size())
}
