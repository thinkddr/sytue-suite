// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// flotante es un panel que flota SIN ser una capa: `menu-surface`, `border`,
// `rounded-lg` y `shadow-md`, como los menús de la web, pero pintado por su dueño entre
// sus propios objetos y FUERA de su caja (Fyne no recorta salvo que se pida).
//
// Es lo que piden el menú de «/», la burbuja de formato y la ficha de un enlace: una
// capa de Fyne se queda con el teclado y con el puntero (ver `ui.Pista`), y los tres
// tienen que dejar escribir —o pasar el ratón— a lo que hay debajo. Es el mismo
// motivo por el que `ui.Pista` es un hermano del control y no una capa.
type flotante struct {
	widget.BaseWidget
	contenido fyne.CanvasObject
	relleno   float32
}

var _ fyne.Tappable = (*flotante)(nil)

func nuevoFlotante(contenido fyne.CanvasObject, relleno float32) *flotante {
	f := &flotante{contenido: contenido, relleno: relleno}
	f.ExtendBaseWidget(f)
	return f
}

// Tapped se traga el clic que cae en el relleno del panel: si no, llegaría a lo de
// debajo (el cuerpo del editor movería el cursor).
func (f *flotante) Tapped(*fyne.PointEvent) {}

func (f *flotante) CreateRenderer() fyne.WidgetRenderer {
	r := &flotanteRenderer{
		f:      f,
		caja:   ui.NuevaCaja(color.NRGBA{A: 0xff}, color.NRGBA{A: 0xff}, tema.RadioLG),
		sombra: ui.NuevaSombra(tema.SombraMD, tema.RadioLG),
	}
	r.Refresh()
	return r
}

type flotanteRenderer struct {
	f      *flotante
	caja   *ui.Caja
	sombra *ui.Sombra
}

func (r *flotanteRenderer) lado() float32 { return 2 * (r.f.relleno + tema.Borde) }

func (r *flotanteRenderer) MinSize() fyne.Size {
	m := r.f.contenido.MinSize()
	return fyne.NewSize(m.Width+r.lado(), m.Height+r.lado())
}

func (r *flotanteRenderer) Objects() []fyne.CanvasObject {
	return append(append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...), r.f.contenido)
}

func (r *flotanteRenderer) Destroy() {}

func (r *flotanteRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Redimensionar(t)
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	d := r.f.relleno + tema.Borde
	r.f.contenido.Move(fyne.NewPos(d, d))
	r.f.contenido.Resize(fyne.NewSize(max(t.Width-r.lado(), 0), max(t.Height-r.lado(), 0)))
}

func (r *flotanteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	r.Layout(r.f.Size())
	r.f.contenido.Refresh()
}

// colocarFlotante pone el panel a su medida en (x, y) del sistema de su dueño, sin
// salirse por la derecha de `ancho` (el `Math.min(…, innerWidth - …)` de la web).
func colocarFlotante(f *flotante, x, y, ancho float32) {
	m := f.MinSize()
	f.Resize(m)
	f.Move(fyne.NewPos(max(0, min(x, ancho-m.Width)), y))
}

// portal es la capa SIN layout que la pantalla pone encima de todo lo suyo (la de
// `ui.Pista.Portal`): ahí van los flotantes para que no los tape lo que se pinta después
// de su dueño —la tarjeta de debajo, en el lienzo—. Como no es una capa de Fyne, no se
// queda con el teclado: el cuerpo del editor sigue recibiendo lo que se escribe. Lo
// comparten varios dueños y cada uno cambia solo lo suyo.
type portal struct {
	c     *fyne.Container
	de    map[any][]fyne.CanvasObject
	orden []any
}

func nuevoPortal() *portal {
	return &portal{c: container.NewWithoutLayout(), de: map[any][]fyne.CanvasObject{}}
}

// poner deja en el portal los objetos de `dueno` (ninguno, para quitarlos).
func (p *portal) poner(dueno any, objs ...fyne.CanvasObject) {
	if p == nil {
		return
	}
	if _, hay := p.de[dueno]; !hay && len(objs) > 0 {
		p.orden = append(p.orden, dueno)
	}
	if len(objs) == 0 {
		delete(p.de, dueno)
	} else {
		p.de[dueno] = objs
	}
	todos := make([]fyne.CanvasObject, 0, len(p.c.Objects)+len(objs))
	quedan := p.orden[:0]
	for _, d := range p.orden {
		if suyos, hay := p.de[d]; hay {
			quedan = append(quedan, d)
			todos = append(todos, suyos...)
		}
	}
	p.orden = quedan
	p.c.Objects = todos
	p.c.Refresh()
}

// desde es lo que hay que sumar a una posición en el sistema de `o` para llevarla al del
// portal.
func (p *portal) desde(o fyne.CanvasObject) fyne.Position {
	d := fyne.CurrentApp().Driver()
	return d.AbsolutePositionForObject(o).Subtract(d.AbsolutePositionForObject(p.c))
}
