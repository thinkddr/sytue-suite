// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Enlace es el texto que se pulsa: un `<a>` —o un `<button>` con pinta de `<a>`— que se
// subraya al pasar por encima (`hover:underline`) y lleva el anillo de foco con el
// teclado.
//
// # Por qué está en el catálogo
//
// La web no lo tiene en `packages/ui`: cada app lo escribe a mano con sus clases. Y
// en Fyne pasó lo mismo, dos veces con dos subrayados distintos —el Hub con un
// rectángulo bajo la caja de línea, Contactos con el `Underline` de la letra— para la
// MISMA pieza. Lo que cambia entre usos son datos —el escalón, el peso, el token de la
// tinta, el del anillo y la diana extra—, así que eso son los campos, y la pieza es
// una (`docs/CONVENTIONS.md` §13.2).
//
// # El subrayado
//
// Es el de la LETRA (`fyne.TextStyle.Underline`): Fyne lo centra 2 px por debajo de la
// línea base, con 1 px de grueso. La web lo pone con el `post` de Figtree (posición
// −0,1 em, grueso 0,05 em) —1,2 px bajo la línea base en `text-sm`— o, con
// `underline-offset-2` como el de Contactos, a 2 px. Calculado, no medido en un
// navegador: queda a medio píxel en los dos casos. El rectángulo pegado al fondo de la
// caja de línea que llevaba el Hub caía casi 4 px por debajo de la línea base.
//
// ponytail: un solo subrayado para `auto` y para `underline-offset-2`; si un guardián
// de píxel lo pide, un rectángulo colocado desde la línea base con la cuenta de arriba.
//
// # Ancho
//
// Un `<a>` o un `<button>` en línea mide su rótulo aunque la columna que lo contiene
// se estire, así que el anillo de foco abraza el rótulo y no el ancho que le den.
type Enlace struct {
	Pulsable

	Texto   string
	Escalon tema.Escalon
	Peso    Peso
	// Token es el color de la tinta.
	Token string
	// TokenAro es el color del anillo de foco. Vacío: el de la suite. Sobre el cromo
	// oscuro va `chrome-focus`, que es el que ahí se ve.
	TokenAro string
	// Sangria es el `py-…` que agranda la diana sin mover lo de alrededor (`-my-1
	// py-1` en la web): el enlace de «Hoy» del Hub era el ÚNICO objetivo por debajo de
	// los 24 px de WCAG 2.5.8 en las nueve apps.
	Sangria float32
	// Boton lo anuncia como botón y no como enlace: es el `<button>` con pinta de
	// enlace («Reintentar» en el menú de Contactos). Cambia el ROL, no el dibujo.
	Boton bool
	// Subrayado lo subraya SIEMPRE y no solo al pasar por encima: el `underline
	// underline-offset-2` de los «Reintentar» de un aviso de error (el hilo de Rooms).
	Subrayado bool
	// Liso no lo subraya NI al pasar por encima: un `<a>` sin clases, al que el
	// `preflight` de Tailwind deja `text-decoration: inherit` (las descargas del Hub).
	Liso bool

	// fueraDelTab aparta el enlace del tabulador mientras diga que sí: la acción de un
	// aviso PRESTADO a un diálogo (ver `Aviso.prestado`).
	fueraDelTab func() bool
}

// Disabled es `fyne.Disableable`, lo único que el gestor de foco de Fyne mira al
// tabular: un enlace inerte no es parada, y la acción de un aviso prestado tampoco.
func (e *Enlace) Disabled() bool { return e.Inerte || (e.fueraDelTab != nil && e.fueraDelTab()) }

// Enable enciende el enlace.
func (e *Enlace) Enable() { e.Inerte = false; e.Refresh() }

// Disable lo apaga.
func (e *Enlace) Disable() { e.Inerte = true; e.Refresh() }

var (
	_ fyne.Widget      = (*Enlace)(nil)
	_ fyne.Accessible  = (*Enlace)(nil)
	_ fyne.Disableable = (*Enlace)(nil)
)

// NuevoEnlace crea el enlace con su escalón, su peso y el token de su tinta.
func NuevoEnlace(texto string, e tema.Escalon, p Peso, token string, alPulsar func()) *Enlace {
	x := &Enlace{Texto: texto, Escalon: e, Peso: p, Token: token}
	x.Nombre, x.AlPulsar = texto, alPulsar
	x.ExtendBaseWidget(x)
	return x
}

// AccessibilityRole es `link`, que es lo que es un `<a>`; `button` si lo pide `Boton`.
func (e *Enlace) AccessibilityRole() fyne.AccessibleRole {
	if e.Boton {
		return fyne.AccessibleRoleButton
	}
	return fyne.AccessibleRoleLink
}

// MedidaEnlace es lo que ocupa: el rótulo, con la sangría arriba y abajo. Es la única
// cuenta.
func MedidaEnlace(e *Enlace) fyne.Size {
	return fyne.NewSize(Medir(e.Texto, e.Escalon, e.Peso).Width, e.Escalon.Linea+2*e.Sangria)
}

// CreateRenderer compone el anillo y el rótulo. Termina en Refresh (regla 7).
func (e *Enlace) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	token := e.TokenAro
	if token == "" {
		token = "focus-ring"
	}
	aro := NuevoAroCon(token, tema.RadioSM, SinHueco, v).Abierto() // no tiene caja
	r := &enlaceRenderer{e: e, aro: aro, texto: Texto(e.Texto, e.Escalon, e.Peso, tema.Color(e.Token, v))}
	r.objetos = r.aro.Antes(r.texto)
	r.Refresh()
	return r
}

type enlaceRenderer struct {
	e       *Enlace
	aro     *Aro
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *enlaceRenderer) MinSize() fyne.Size           { return MedidaEnlace(r.e) }
func (r *enlaceRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *enlaceRenderer) Destroy()                     {}

// Layout centra el rótulo en su caja, recortado con elipsis si no cabe, y le pone el
// anillo al rótulo, no a la caja: ver «Ancho» en el tipo.
func (r *enlaceRenderer) Layout(t fyne.Size) {
	ancho := Truncar(r.texto, r.e.Texto, r.e.Escalon, r.e.Peso, t.Width)
	r.aro.Colocar(fyne.NewSize(ancho, t.Height))
	ColocarTexto(r.texto, r.e.Escalon, 0, (t.Height-r.e.Escalon.Linea)/2)
}

// Refresh repinta la tinta, el subrayado del `hover` y el anillo del foco.
func (r *enlaceRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.texto.Color = tema.Color(r.e.Token, v)
	r.texto.TextStyle = fyne.TextStyle{Underline: r.e.Subrayado || (r.e.Encima() && !r.e.Liso)}
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.e.Enfocado())
	r.Layout(r.e.Size())
}
