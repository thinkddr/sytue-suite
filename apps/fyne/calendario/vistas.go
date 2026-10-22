// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func resumenDe(o Ocurrencia, sinTitulo string) string {
	if o.Event.Summary != "" {
		return o.Event.Summary
	}
	return sinTitulo
}

// conMargen es un `p-*` con un primer hijo de ancho fijo opcional: el `flex gap-3` con
// una columna de `w-14` de la agenda.
type conMargen struct {
	M     ui.Margen
	Fijo  float32
	Hueco float32
}

var _ fyne.Layout = (*conMargen)(nil)

func (c *conMargen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += c.Hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho+c.M.Izq+c.M.Der, alto+c.M.Arr+c.M.Abj)
}

func (c *conMargen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := c.M.Izq
	alto := max(t.Height-c.M.Arr-c.M.Abj, 0)
	restante := max(t.Width-c.M.Izq-c.M.Der, 0)
	for i, o := range objs {
		ancho := restante
		if i == 0 && c.Fijo > 0 && len(objs) > 1 {
			ancho = c.Fijo
		}
		o.Move(fyne.NewPos(x, c.M.Arr))
		o.Resize(fyne.NewSize(ancho, alto))
		x += ancho + c.Hueco
		restante -= ancho + c.Hueco
	}
}

// cortina es el `pointer-events-none opacity-60` de la rejilla mientras llegan los
// eventos de otro rango: la vista se queda en su sitio, atenuada, y no atiende al
// puntero.
//
// Fyne no tiene opacidad de grupo. Pintar la vista al 60 % sobre `bg` —que es lo que
// hay detrás en la web, el fondo del AppShell— da exactamente lo mismo que pintar encima
// `bg` al 40 %: 0,6·vista + 0,4·bg. Y se traga el clic y el paso del ratón, que es la
// otra mitad de la clase.
type cortina struct {
	widget.BaseWidget
	velo *canvas.Rectangle
}

var (
	_ fyne.Tappable     = (*cortina)(nil)
	_ desktop.Hoverable = (*cortina)(nil)
)

// OpacidadRecarga es el `opacity-60` de la rejilla en recarga.
const OpacidadRecarga = 0.6

func nuevaCortina() *cortina {
	c := &cortina{velo: canvas.NewRectangle(color.NRGBA{})}
	c.ExtendBaseWidget(c)
	return c
}

func (c *cortina) Tapped(*fyne.PointEvent)        {}
func (c *cortina) MouseIn(*desktop.MouseEvent)    {}
func (c *cortina) MouseMoved(*desktop.MouseEvent) {}
func (c *cortina) MouseOut()                      {}

func (c *cortina) CreateRenderer() fyne.WidgetRenderer {
	r := &cortinaRenderer{c: c}
	r.Refresh()
	return r
}

type cortinaRenderer struct{ c *cortina }

func (r *cortinaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *cortinaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.c.velo} }
func (r *cortinaRenderer) Destroy()                     {}
func (r *cortinaRenderer) Layout(t fyne.Size)           { r.c.velo.Resize(t) }

func (r *cortinaRenderer) Refresh() {
	fondo := tema.Color("bg", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	opaco := float32(255)
	fondo.A = uint8(opaco*(1-OpacidadRecarga) + 0.5) // 102: el 40 % que deja ver el 60 %
	r.c.velo.FillColor = fondo
	r.c.velo.Refresh()
}

// desplazable es `overflow-y-auto`: un `container.Scroll` vertical SIN las sombras que
// Fyne pinta en el borde cuando hay contenido por encima o por debajo. La web no las
// tiene —el contenido se corta limpio contra la cabecera—, y con ellas la rejilla
// horaria salía con una franja gris bajo la fila de los días.
//
// ponytail: vive aquí porque solo lo pide Calendar; en cuanto otra app lo necesite,
// sube a `ui` con este mismo tema de una línea.
func desplazable(o fyne.CanvasObject) (*container.Scroll, fyne.CanvasObject) {
	s := container.NewVScroll(o)
	return s, container.NewThemeOverride(s, temaSinSombra{})
}

// temaSinSombra es el de la suite con la sombra de Fyne transparente.
type temaSinSombra struct{ tema.Sytue }

func (t temaSinSombra) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameShadow {
		return color.Transparent
	}
	return t.Sytue.Color(n, v)
}
