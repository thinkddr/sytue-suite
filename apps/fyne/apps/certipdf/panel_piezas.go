// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// pestanaPanel es un botón de la tira del panel: `text-2xs font-medium px-2.5 py-2
// border-b-2`, con el contador en `text-text-subtle ml-1` si hay algo que contar.
//
// Es de ESTA app y no del catálogo: `ui.Pestana` es la de `Tabs.tsx` —versalitas,
// `text-sm`, `px-3 py-2.5`— y esta no se le parece en nada salvo en el subrayado.
type pestanaPanel struct {
	ui.Pulsable
	rotulo string
	n      int
	activa bool
}

func nuevaPestanaPanel(rotulo string, n int, activa bool, alPulsar func()) *pestanaPanel {
	p := &pestanaPanel{rotulo: rotulo, n: n, activa: activa}
	p.Nombre, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// contador es el número tal cual se pinta; vacío si no hay nada que contar.
func (p *pestanaPanel) contador() string {
	if p.n <= 0 {
		return ""
	}
	return strconv.Itoa(p.n)
}

// medidaPestanaPanel es la ÚNICA cuenta de la pestaña: rótulo, `ml-1` y contador, con
// el `px-2.5`; y de alto, la caja de línea con `py-2` y el subrayado.
func medidaPestanaPanel(p *pestanaPanel) fyne.Size {
	ancho := ui.Medir(p.rotulo, tema.Texto2XS, ui.Medio).Width
	if c := p.contador(); c != "" {
		ancho += tema.Espacio(1) + ui.Medir(c, tema.Texto2XS, ui.Medio).Width
	}
	return fyne.NewSize(ancho+2*pestanaPX, tema.Texto2XS.Linea+2*pestanaPY+subrayadoPestana)
}

func (p *pestanaPanel) MinSize() fyne.Size { p.ExtendBaseWidget(p); return medidaPestanaPanel(p) }

func (p *pestanaPanel) tinta(v tema.Variante) color.NRGBA {
	if p.activa || p.Encima() {
		return tema.Color("text", v)
	}
	return tema.Color("text-muted", v)
}

func (p *pestanaPanel) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaPanelRenderer{
		p:        p,
		aro:      ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		rotulo:   ui.Texto(p.rotulo, tema.Texto2XS, ui.Medio, p.tinta(v)),
		numero:   ui.Texto(p.contador(), tema.Texto2XS, ui.Medio, tema.Color("text-subtle", v)),
		subrayar: canvas.NewRectangle(color.Transparent),
	}
	r.objetos = r.aro.Antes(r.rotulo, r.numero, r.subrayar)
	r.Refresh()
	return r
}

type pestanaPanelRenderer struct {
	p              *pestanaPanel
	aro            *ui.Aro
	rotulo, numero *canvas.Text
	subrayar       *canvas.Rectangle
	objetos        []fyne.CanvasObject
}

func (r *pestanaPanelRenderer) MinSize() fyne.Size           { return medidaPestanaPanel(r.p) }
func (r *pestanaPanelRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanaPanelRenderer) Destroy()                     {}

func (r *pestanaPanelRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	ui.ColocarTexto(r.rotulo, tema.Texto2XS, pestanaPX, pestanaPY)
	x := pestanaPX + ui.Medir(r.p.rotulo, tema.Texto2XS, ui.Medio).Width + tema.Espacio(1)
	ui.ColocarTexto(r.numero, tema.Texto2XS, x, pestanaPY)
	r.subrayar.Move(fyne.NewPos(0, t.Height-subrayadoPestana))
	r.subrayar.Resize(fyne.NewSize(t.Width, subrayadoPestana))
}

func (r *pestanaPanelRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Color = r.p.tinta(v)
	r.rotulo.Refresh()
	r.numero.Color = tema.Color("text-subtle", v)
	r.numero.Refresh()
	// `border-accent` en la abierta; en las demás, `border-transparent`: el subrayado
	// está siempre, y por eso cambiar de pestaña no mueve el rótulo.
	r.subrayar.FillColor = color.Transparent
	if r.p.activa {
		r.subrayar.FillColor = tema.Color("accent", v)
	}
	r.subrayar.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.p.Enfocado())
	r.Layout(r.p.Size())
}

// filaIndice es una entrada del índice en lectura: `rounded px-2 py-1 text-xs`, la
// sangría del nivel a la izquierda, el título recortado y la página a la derecha en
// `text-2xs`. Un marcador SIN destino se enseña igual, en `text-subtle` y sin página, y
// no se puede pulsar: el documento dice que ahí hay un capítulo, y esconderlo dejaría
// un índice con huecos que nadie sabría explicar.
type filaIndice struct {
	ui.Pulsable
	m     Marcador
	ancho float32
}

// radioFila es `rounded` a secas: `var(--radius)`, que la hoja de la suite no redefine
// y Tailwind deja en 0,25rem. 3,5 px, no los 4 de `rounded-sm`.
var radioFila float32 = 0.25 * tema.Rem

// nuevaFilaIndice monta la fila. `sinDestino` es el `title` que la web pone a un
// marcador sin página; aquí va en el nombre accesible, que es donde lo oye quien no ve
// el gris.
func nuevaFilaIndice(m Marcador, ancho float32, sinDestino string) *filaIndice {
	f := &filaIndice{m: m, ancho: ancho}
	f.Nombre = m.Titulo
	if m.Pagina < 0 {
		f.Inerte = true
		f.Nombre = m.Titulo + ". " + sinDestino
	}
	f.ExtendBaseWidget(f)
	return f
}

// AltoFilaIndice es `py-1` sobre la caja de línea de `text-xs`: 21.
func AltoFilaIndice() float32 { return tema.TextoXS.Linea + 2*tema.Espacio(1) }

func (f *filaIndice) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(f.ancho, AltoFilaIndice())
}

// pagina es el número de la derecha; vacío en un marcador sin destino.
func (f *filaIndice) pagina() string {
	if f.m.Pagina < 0 {
		return ""
	}
	return strconv.Itoa(f.m.Pagina + 1)
}

func (f *filaIndice) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaIndiceRenderer{
		f:      f,
		aro:    ui.NuevoAro(radioFila, ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, radioFila),
		titulo: ui.Texto(f.m.Titulo, tema.TextoXS, ui.Normal, tema.Color("text", v)),
		numero: ui.Texto(f.pagina(), tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = r.aro.Antes(append(r.caja.Objetos(), r.titulo, r.numero)...)
	r.Refresh()
	return r
}

type filaIndiceRenderer struct {
	f              *filaIndice
	aro            *ui.Aro
	caja           *ui.Caja
	titulo, numero *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *filaIndiceRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaIndiceRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaIndiceRenderer) Destroy()                     {}

// Layout: la página pegada al `px-2` de la derecha y el título en lo que queda, con el
// `gap-2` entre los dos.
//
// ponytail: los dos textos van centrados en su caja de línea (de 16 px en los dos) y no
// alineados por la línea base (`items-baseline`); con Figtree eso deja el número unas
// tres décimas de píxel más alto que en la web.
func (r *filaIndiceRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	derecha := t.Width - tema.Espacio(2)
	if n := r.f.pagina(); n != "" {
		derecha -= ui.Medir(n, tema.Texto2XS, ui.Normal).Width
		ui.ColocarTexto(r.numero, tema.Texto2XS, derecha, tema.Espacio(1))
		derecha -= tema.Espacio(2)
	}
	x := sangriaIndice(r.f.m.Nivel)
	ui.Truncar(r.titulo, r.f.m.Titulo, tema.TextoXS, ui.Normal, max(derecha-x, 0))
	ui.ColocarTexto(r.titulo, tema.TextoXS, x, tema.Espacio(1))
}

func (r *filaIndiceRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.Encima() && !r.f.Inerte {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.titulo.Color = tema.Color("text", v)
	if r.f.Inerte {
		r.titulo.Color = tema.Color("text-subtle", v)
	}
	r.titulo.Refresh()
	r.numero.Color = tema.Color("text-subtle", v)
	r.numero.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
}
