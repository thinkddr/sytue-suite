// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Liso es un relleno plano del color de un token que se resuelve AL PINTARSE, con la
// apariencia en vigor. Es lo que va donde quien compone una pieza ponía
// `canvas.NewRectangle(tema.Color(token, v))`: un fondo, un filete, una banda.
//
// Ese rectángulo se quedaba con el color de la variante con que se compuso. Al cambiar
// el tema en caliente Fyne no vuelve a componer nada: recorre lo que hay y lo refresca
// (`ApplyThemeTo`), y un `canvas.Rectangle` refrescado se repinta con el color que ya
// tenía. Así, al pasar a oscuro, la cinta, la tira de pestañas y el panel de páginas de
// CertiPDF se quedaban claros mientras sus botones —widgets, que resuelven al pintar—
// cambiaban encima (2026-11-25). Liso es un widget: ese mismo refresco le hace resolver
// otra vez, y sigue también al color y a la forma, que cambian los mismos tokens.
type Liso struct {
	widget.BaseWidget
	// Token es el color: `--sytue-<Token>`.
	Token string
	// Tinta, si no es nil, calcula el color en vez de Token: un derivado de tokens (con
	// alfa, aplanado sobre otro) o el color propio de una pieza que depende del tema.
	Tinta func(v tema.Variante) color.NRGBA
	// Radio es el de las esquinas.
	Radio float32
	// Minimo es su medida mínima: el `SetMinSize` del rectángulo al que sustituye (un
	// filete de `h-5 w-px` en una fila). Cero, como el de un `canvas.Rectangle`.
	Minimo fyne.Size
}

var _ fyne.Widget = (*Liso)(nil)

// NuevoLiso es el relleno del color de un token.
func NuevoLiso(token string) *Liso {
	l := &Liso{Token: token}
	l.ExtendBaseWidget(l)
	return l
}

// NuevoLisoCon es el relleno de un color calculado con la variante en vigor.
func NuevoLisoCon(tinta func(v tema.Variante) color.NRGBA) *Liso {
	l := &Liso{Tinta: tinta}
	l.ExtendBaseWidget(l)
	return l
}

// Color es el que pinta con la variante `v`.
func (l *Liso) Color(v tema.Variante) color.NRGBA {
	if l.Tinta != nil {
		return l.Tinta(v)
	}
	return tema.Color(l.Token, v)
}

// CreateRenderer arma el rectángulo. Termina en Refresh, que es la regla 7 del paquete.
func (l *Liso) CreateRenderer() fyne.WidgetRenderer {
	r := &lisoRenderer{l: l, rect: canvas.NewRectangle(color.Transparent)}
	r.Refresh()
	return r
}

type lisoRenderer struct {
	l    *Liso
	rect *canvas.Rectangle
}

// MinSize es Minimo, cero si no se pone, como el de un `canvas.Rectangle`: la medida la
// pone quien lo coloca.
func (r *lisoRenderer) MinSize() fyne.Size           { return r.l.Minimo }
func (r *lisoRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }
func (r *lisoRenderer) Destroy()                     {}
func (r *lisoRenderer) Layout(t fyne.Size)           { r.rect.Resize(t) }

func (r *lisoRenderer) Refresh() {
	r.rect.FillColor = r.l.Color(varianteEnVigor())
	r.rect.CornerRadius = r.l.Radio
	r.rect.Resize(r.l.Size())
	r.rect.Refresh()
}

// RehacerConElTema envuelve `o`, que ocupa todo lo que le den, y llama a `rehacer` cuando
// al colocarlo la variante en vigor ya no es la de la vez anterior.
//
// Es para lo que se compone ENTERO con los colores ya resueltos y se rehace con su
// estado —las tarjetas de los comentarios, los cambios de la revisión—, donde llevar cada
// caja, cada cita y cada tinta atenuada a un widget que resuelva al pintarse sería
// reescribir la lista. Al cambiar de tema en caliente Fyne vuelve a colocar cada
// contenedor (`ApplyThemeTo`), y ahí se rehace con el tema nuevo.
func RehacerConElTema(o fyne.CanvasObject, rehacer func()) *fyne.Container {
	return container.New(&conElTema{v: varianteEnVigor(), rehacer: rehacer}, o)
}

type conElTema struct {
	v       tema.Variante
	rehacer func()
}

func (c *conElTema) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (c *conElTema) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	// La variante se apunta ANTES de rehacer: si lo rehecho vuelve a colocar este
	// contenedor, ya no hay nada que rehacer.
	if v := varianteEnVigor(); v != c.v {
		c.v = v
		c.rehacer()
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}
