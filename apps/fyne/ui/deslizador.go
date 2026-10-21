// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Deslizador es el `<input type="range">` de la suite. Lo piden el editor de Sytue
// Photos, los ajustes de la cuenta y el ERP.
//
// # El suelo táctil, que es la única medida que NO sale de la web
//
// El carril del navegador mide 4 px de alto y el pulgar 14 en la familia de WebKit, y
// eso es lo que se pinta. Pero la caja que RECIBE el dedo no puede medir 14: WCAG 2.5.8
// pide 24 y la HIG de Apple 44, y en un teléfono —que es un destino de este port y no
// de la web— un pulgar de 14 px no se acierta.
//
// Así que la caja crece SOLO donde hay dedo (`EsTactil()`) y el
// carril se pinta centrado dentro. En escritorio la medida es la de la web, al píxel, y
// no hace falta ninguna excepción a la paridad de `docs/CONVENTIONS.md` §13.6: donde la
// web corre, el port mide lo mismo que ella.
const (
	// AltoCarrilDeslizador es el `::-webkit-slider-runnable-track`.
	AltoCarrilDeslizador float32 = 4
	// LadoPulgar es el `::-webkit-slider-thumb`.
	LadoPulgar float32 = 14
	// SueloTactilDeslizador es el objetivo táctil de la HIG de Apple y de WCAG 2.5.5.
	SueloTactilDeslizador float32 = 44
	// PasoDeslizador es el `step` por defecto: el 0,01 del editor de fotos.
	PasoDeslizador = 0.01
)

// Deslizador es el control.
type Deslizador struct {
	widget.BaseWidget

	Min, Max, Valor float64
	// Paso es el `step`. Con 0, PasoDeslizador. Es lo que mueven las flechas, y es el
	// del control y no una centésima del recorrido: un deslizador de exposición que va
	// de −5 a 5 se mueve de 0,01 en 0,01 igual que uno que va de −1 a 1, porque el paso
	// es una unidad del DOMINIO, no una fracción de la barra.
	Paso float64
	// Rotulo es el nombre accesible (el `aria-label` o el `<label>` de la web).
	Rotulo string
	// TextoValor es el `aria-valuetext`: el valor YA formateado por la app con su
	// `i18n`. Vacío, se lee el número crudo con dos decimales, que es lo que pinta el
	// `<output>` de la web pero sin localizar.
	TextoValor string
	AlCambiar  func(float64)
	// AlDobleClic es el `onDblClick` (el editor de Photos vuelve el ajuste a 0). Se
	// detecta a mano en Tapped y no con fyne.DoubleTappable: con esa interfaz Fyne
	// retrasa cada clic suelto hasta descartar el segundo, y el pulgar llegaría tarde.
	AlDobleClic func()
	// Nativo es el `<input type="range">` SIN estilo de Chromium (el grosor de la cinta
	// de CertiPDF): carril de 8 con borde de 1 y pulgar de 16, con los colores del
	// navegador, que cambian con `color-scheme` (ver [RangoNativo]). Sin él, el de la
	// suite: carril de 4 y pulgar de 14 con los tokens.
	Nativo bool
	// Bipolar: el tramo lleno va del 0 al valor (el editor de Photos). Sin él, va desde el
	// extremo izquierdo (el mínimo).
	Bipolar bool

	enfocado    bool
	ultimoToque time.Time
}

// esperaDobleClic es la ventana entre dos clics para que cuenten como doble: la de
// `DoubleTapDelay` del toolkit.
const esperaDobleClic = 300 * time.Millisecond

var (
	_ fyne.Widget       = (*Deslizador)(nil)
	_ fyne.Focusable    = (*Deslizador)(nil)
	_ fyne.Tappable     = (*Deslizador)(nil)
	_ fyne.Draggable    = (*Deslizador)(nil)
	_ fyne.Accessible   = (*Deslizador)(nil)
	_ desktop.Hoverable = (*Deslizador)(nil)
)

// NuevoDeslizador crea el control con su recorrido.
func NuevoDeslizador(minimo, maximo, valor float64, alCambiar func(float64)) *Deslizador {
	d := &Deslizador{Min: minimo, Max: maximo, Valor: valor, AlCambiar: alCambiar}
	d.ExtendBaseWidget(d)
	return d
}

// Con pone el rótulo accesible y devuelve el deslizador, para encadenarlo.
func (d *Deslizador) Con(rotulo string) *Deslizador {
	d.Rotulo = rotulo
	return d
}

// AccessibilityRole: Fyne 2.8.1 tiene cuatro roles y ninguno es `slider`, así que se
// anuncia como botón. Está en los techos del toolkit.
func (d *Deslizador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// AccessibilityLabel es el rótulo y el valor, que es lo que junta un lector de pantalla
// con `aria-label` + `aria-valuetext`.
func (d *Deslizador) AccessibilityLabel() string {
	valor := d.TextoValor
	if valor == "" {
		valor = strconv.FormatFloat(d.Valor, 'f', 2, 64)
	}
	if d.Rotulo == "" {
		return valor
	}
	return d.Rotulo + ": " + valor
}

// FocusGained y FocusLost encienden y apagan el aro.
func (d *Deslizador) FocusGained() { d.enfocado = true; d.Refresh() }

// FocusLost apaga el aro.
func (d *Deslizador) FocusLost() { d.enfocado = false; d.Refresh() }

// TypedRune no hace nada: el deslizador se mueve con las flechas.
func (d *Deslizador) TypedRune(rune) {}

// paso es el `step` efectivo.
func (d *Deslizador) paso() float64 {
	if d.Paso > 0 {
		return d.Paso
	}
	return PasoDeslizador
}

// TypedKey mueve el valor con las flechas, en pasos del `step`, y salta a los extremos
// con Inicio y Fin. Es lo que hace un `<input type="range">` nativo. Las demás teclas no
// significan nada sobre un deslizador.
//
//nolint:exhaustive // el linter quiere las ochenta teclas y aquí hay seis con sentido.
func (d *Deslizador) TypedKey(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyLeft, fyne.KeyDown:
		d.Poner(d.Valor - d.paso())
	case fyne.KeyRight, fyne.KeyUp:
		d.Poner(d.Valor + d.paso())
	case fyne.KeyHome:
		d.Poner(d.Min)
	case fyne.KeyEnd:
		d.Poner(d.Max)
	case fyne.KeyEscape, mobile.KeyBack:
		SubirEscape(d, e)
	}
}

// MouseIn, MouseMoved y MouseOut: el deslizador no cambia de color al pasar por encima
// en la web, pero tiene que aceptar el puntero para poder arrastrarse.
func (d *Deslizador) MouseIn(*desktop.MouseEvent) {}

// MouseMoved no hace nada.
func (d *Deslizador) MouseMoved(*desktop.MouseEvent) {}

// MouseOut no hace nada.
func (d *Deslizador) MouseOut() {}

// Tapped salta al punto pulsado, como hace el navegador al hacer clic en el carril.
func (d *Deslizador) Tapped(e *fyne.PointEvent) {
	d.aX(e.Position.X)
	if d.AlDobleClic == nil {
		return
	}
	if ahora := time.Now(); ahora.Sub(d.ultimoToque) <= esperaDobleClic {
		d.ultimoToque = time.Time{}
		d.AlDobleClic()
	} else {
		d.ultimoToque = ahora
	}
}

// Dragged arrastra el pulgar.
func (d *Deslizador) Dragged(e *fyne.DragEvent) { d.aX(e.Position.X) }

// DragEnd no hace nada: el valor ya se fue avisando durante el arrastre, que es lo que
// hace el `onInput` de la web (y no `onChange`).
func (d *Deslizador) DragEnd() {}

// aX pone el valor que corresponde a una x de la caja. El recorrido útil descuenta el
// pulgar, porque su CENTRO nunca sale del carril.
func (d *Deslizador) aX(x float32) {
	lado := d.ladoPulgar()
	util := d.Size().Width - lado
	if util <= 0 {
		return
	}
	f := float64(min(max((x-lado/2)/util, 0), 1))
	d.Poner(d.Min + f*(d.Max-d.Min))
}

// Poner fija el valor dentro de sus topes y avisa.
func (d *Deslizador) Poner(valor float64) {
	v := min(max(valor, d.Min), d.Max)
	if v == d.Valor {
		return
	}
	d.Valor = v
	if d.AlCambiar != nil {
		d.AlCambiar(v)
	}
	d.Refresh()
}

// Fraccion es dónde está el pulgar, de 0 a 1.
func (d *Deslizador) Fraccion() float64 {
	if d.Max <= d.Min {
		return 0
	}
	return (d.Valor - d.Min) / (d.Max - d.Min)
}

// Las medidas del `<input type="range">` nativo de Chromium, medidas en CertiPDF.
const (
	// AltoCarrilNativo es el carril, con su borde de 1 incluido.
	AltoCarrilNativo float32 = 8
	// LadoPulgarNativo es el pulgar, y también el alto de la caja del control.
	LadoPulgarNativo float32 = 16
)

func (d *Deslizador) ladoPulgar() float32 {
	if d.Nativo {
		return LadoPulgarNativo
	}
	return LadoPulgar
}

func (d *Deslizador) altoCarril() float32 {
	if d.Nativo {
		return AltoCarrilNativo
	}
	return AltoCarrilDeslizador
}

// ColoresRango son los de un `<input type="range">` sin estilo de Chromium.
type ColoresRango struct{ Acento, Carril, BordeCarril color.NRGBA }

// RangoNativo son los colores del deslizador de Chromium en esa variante. Medidos en
// CertiPDF (2026-09-25): en claro, #0075ff sobre un carril #efefef con borde #b2b2b2;
// en oscuro (`color-scheme: dark`), #99c8ff sobre #3b3b3b con borde #858585. El tramo
// recorrido va lleno del acento, sin borde.
func RangoNativo(v tema.Variante) ColoresRango {
	gris := func(n uint8) color.NRGBA { return color.NRGBA{R: n, G: n, B: n, A: 255} }
	if v == tema.Oscuro {
		return ColoresRango{Acento: CasillaNativa(v).Marcada, Carril: gris(0x3b), BordeCarril: gris(0x85)}
	}
	return ColoresRango{Acento: CasillaNativa(v).Marcada, Carril: gris(0xef), BordeCarril: gris(0xb2)}
}

// AltoDeslizador es el alto de la CAJA: el del pulgar en escritorio, y el suelo táctil
// donde se toca con el dedo. Ver el comentario del tipo.
func AltoDeslizador() float32 {
	if EsTactil() {
		return SueloTactilDeslizador
	}
	return LadoPulgar
}

// MedidaDeslizador es lo que ocupa: todo el ancho que le den (el `w-full`), por el alto
// de la caja.
func MedidaDeslizador() fyne.Size { return fyne.NewSize(0, AltoDeslizador()) }

// CreateRenderer arma el deslizador.
func (d *Deslizador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &deslizadorRenderer{d: d}
	if d.Nativo {
		// El carril nativo lleva borde, y la caja lo tiene que saber al crearse.
		c := RangoNativo(v)
		r.carril = NuevaCaja(c.Carril, c.BordeCarril, AltoCarrilNativo/2)
		r.recorrido = NuevaCaja(c.Acento, color.NRGBA{}, AltoCarrilNativo/2)
	} else {
		r.carril = NuevaCaja(tema.Color("border", v), color.NRGBA{}, AltoCarrilDeslizador/2)
		r.recorrido = NuevaCaja(tema.Color("primary", v), color.NRGBA{}, AltoCarrilDeslizador/2)
	}
	r.pulgar = NuevaCaja(tema.Color("primary", v), color.NRGBA{}, tema.RadioPill)
	r.aro = NuevoAro(tema.RadioPill, SinHueco, v)
	r.objetos = append(r.aro.Objetos(), r.carril.Objetos()...)
	r.objetos = append(r.objetos, r.recorrido.Objetos()...)
	r.objetos = append(r.objetos, r.pulgar.Objetos()...)
	r.Refresh()
	return r
}

type deslizadorRenderer struct {
	d                         *Deslizador
	carril, recorrido, pulgar *Caja
	aro                       *Aro
	objetos                   []fyne.CanvasObject
}

func (r *deslizadorRenderer) MinSize() fyne.Size {
	if r.d.Nativo && !EsTactil() {
		return fyne.NewSize(0, LadoPulgarNativo)
	}
	return MedidaDeslizador()
}
func (r *deslizadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *deslizadorRenderer) Destroy()                     {}

func (r *deslizadorRenderer) Layout(t fyne.Size) {
	lado, carril := r.d.ladoPulgar(), r.d.altoCarril()
	y := (t.Height - carril) / 2
	// Chromium mete el carril nativo 1 px por cada lado (medido: 54 de carril en un
	// control de 56); el pulgar sí recorre la caja entera.
	var dentro float32
	if r.d.Nativo {
		dentro = tema.Borde
	}
	MoverCaja(r.carril, fyne.NewPos(dentro, y), fyne.NewSize(t.Width-2*dentro, carril))
	util := max(t.Width-lado, 0)
	x := float32(r.d.Fraccion()) * util
	if r.d.Bipolar && r.d.Max > r.d.Min {
		f0 := float32((0.0 - r.d.Min) / (r.d.Max - r.d.Min))
		x0 := f0*util + lado/2
		xVal := x + lado/2
		desde := min(x0, xVal)
		hasta := max(x0, xVal)
		MoverCaja(r.recorrido, fyne.NewPos(desde, y), fyne.NewSize(hasta-desde, carril))
	} else {
		MoverCaja(r.recorrido, fyne.NewPos(dentro, y), fyne.NewSize(x+lado/2-dentro, carril))
	}
	MoverCaja(r.pulgar, fyne.NewPos(x, (t.Height-lado)/2), fyne.NewSize(lado, lado))
	r.aro.ColocarEn(fyne.NewPos(x, (t.Height-lado)/2), fyne.NewSize(lado, lado))
	r.aro.Ver(r.d.enfocado)
}

func (r *deslizadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.d.Nativo {
		n := RangoNativo(v)
		r.carril.Pintar(n.Carril, n.BordeCarril)
		r.recorrido.Pintar(n.Acento, color.NRGBA{})
		r.pulgar.Pintar(n.Acento, color.NRGBA{})
	} else {
		r.carril.Pintar(tema.Color("border", v), color.NRGBA{})
		r.recorrido.Pintar(tema.Color("primary", v), color.NRGBA{})
		r.pulgar.Pintar(tema.Color("primary", v), color.NRGBA{})
	}
	r.aro.Pintar(v)
	r.Layout(r.d.Size())
}
