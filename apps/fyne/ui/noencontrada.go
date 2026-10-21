// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/PaginaNoEncontrada.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// PaginaNoEncontrada es la 404 de la suite (`PaginaNoEncontrada.tsx`): lo que se enseña
// cuando llega un enlace a algo que ya no está —un fichero borrado, una nota que otro
// quitó— o que la app no reconoce. En nativo no se escriben direcciones, pero los
// enlaces llegan igual: de un aviso viejo, de otra app, de un correo. Antes eso se
// resolvía en silencio, abriendo el inicio como si nada.
//
// La columna de la web, de arriba abajo y centrada en las dos direcciones con
// `px-4 py-12`, sobre `bg`:
//
//	ilustración  `emptySearch` a 160 de ancho                 `mb-6` = 21
//	sobrerrótulo `sytue-overline text-text-subtle`            «Error 404»
//	título       `sytue-h3 text-text mt-2 max-w-md`           7 · 392 de ancho
//	frase        `text-base text-text-muted mt-2 max-w-md`    7 · 392 de ancho
//	botón        primario, talla por defecto, `mt-6`          21
//
// ponytail: el título y la frase se reparten de forma codiciosa, no con el `text-balance`
// y `text-pretty` de la web. En los veintiséis idiomas el título cabe en una línea de 392
// y la frase también, así que hoy no se nota; si alguna se parte, se equilibra aquí.
type PaginaNoEncontrada struct {
	widget.BaseWidget

	Sobrerrotulo, Titulo, Descripcion string
	boton                             *Boton
}

var _ fyne.Widget = (*PaginaNoEncontrada)(nil)

// Medidas de la columna, en píxeles.
const (
	// AnchoIlustracionNoEncontrada es el `size={160}` de la ilustración.
	AnchoIlustracionNoEncontrada float32 = 160
	// AnchoTextoNoEncontrada es `max-w-md`: 28 rem.
	AnchoTextoNoEncontrada = 28 * tema.Rem
)

// El título es `.sytue-h3`: `--text-xl` en semi-negrita (19,25 / 26,25), con la escala y
// el peso de la forma (`EscalonTitulo(3)` y `PesoTitulo(3)`).

// NuevaPaginaNoEncontrada monta la página con las cuatro claves `notFound.*` del
// catálogo común. `alInicio` es lo que hace el botón: volver al inicio de la app.
func NuevaPaginaNoEncontrada(t *i18n.Translator, alInicio func()) *PaginaNoEncontrada {
	p := &PaginaNoEncontrada{
		Sobrerrotulo: t.T("notFound.overline", nil),
		Titulo:       t.T("notFound.title", nil),
		Descripcion:  t.T("notFound.description", nil),
		boton:        NuevoBoton(t.T("notFound.action", nil), alInicio).Con(Primario, TallaMD),
	}
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el título, que es el `h1` de la página.
func (p *PaginaNoEncontrada) AccessibilityLabel() string { return p.Titulo }

// AccessibilityRole: agrupa, y lo pulsable es el botón.
func (p *PaginaNoEncontrada) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// lineasNoEncontrada reparte un texto en el ancho que le toca: `max-w-md` o lo que deje
// la ventana con su `px-4`.
func lineasNoEncontrada(texto string, e tema.Escalon, p Peso, ventana float32) []string {
	return Repartir(texto, e, p, min(AnchoTextoNoEncontrada, ventana-2*tema.Espacio(4)))
}

// AltoColumnaNoEncontrada es lo que ocupa la columna, sin el `py-12`, a ese ancho de
// ventana. Es la única cuenta: el layout recorre lo mismo.
func (p *PaginaNoEncontrada) AltoColumnaNoEncontrada(ventana float32) float32 {
	ilus := iconos.VacioBusqueda.MedidaNatural()
	alto := AnchoIlustracionNoEncontrada * ilus.Height / ilus.Width
	alto += tema.Espacio(6) + tema.TextoXS.Linea
	alto += tema.Espacio(2) + float32(len(lineasNoEncontrada(p.Titulo, EscalonTitulo(3), PesoTitulo(3), ventana)))*EscalonTitulo(3).Linea
	alto += tema.Espacio(2) + float32(len(lineasNoEncontrada(p.Descripcion, tema.TextoBase, Normal, ventana)))*tema.TextoBase.Linea
	return alto + tema.Espacio(6) + p.boton.MinSize().Height
}

// CreateRenderer compone la columna.
func (p *PaginaNoEncontrada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &noEncontradaRenderer{
		p:     p,
		fondo: canvas.NewRectangle(tema.Color("bg", v)),
		ilus:  iconos.Pintar(iconos.VacioBusqueda, v).A(AnchoIlustracionNoEncontrada),
		sobre: NuevoVersal(p.Sobrerrotulo, tema.TextoXS, SemiNegrita, tema.Color("text-subtle", v)),
	}
	r.rehacer(0)
	return r
}

type noEncontradaRenderer struct {
	p              *PaginaNoEncontrada
	fondo          *canvas.Rectangle
	ilus           *iconos.Dibujo
	sobre          *Versal
	titulo, frase  []*canvas.Text
	ancho          float32 // el ancho con el que se repartieron las líneas
	objetos        []fyne.CanvasObject
	colorTitulo    color.NRGBA
	colorFrase     color.NRGBA
	varianteActual tema.Variante
}

// rehacer vuelve a partir título y frase para ese ancho de ventana y rehace la lista.
func (r *noEncontradaRenderer) rehacer(ventana float32) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.ancho, r.varianteActual = ventana, v
	r.colorTitulo, r.colorFrase = tema.Color("text", v), tema.Color("text-muted", v)
	r.titulo = r.titulo[:0]
	for _, l := range lineasNoEncontrada(r.p.Titulo, EscalonTitulo(3), PesoTitulo(3), ventana) {
		r.titulo = append(r.titulo, Texto(l, EscalonTitulo(3), PesoTitulo(3), r.colorTitulo))
	}
	r.frase = r.frase[:0]
	for _, l := range lineasNoEncontrada(r.p.Descripcion, tema.TextoBase, Normal, ventana) {
		r.frase = append(r.frase, Texto(l, tema.TextoBase, Normal, r.colorFrase))
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.ilus.Objeto()}
	r.objetos = append(r.objetos, r.sobre.Objetos()...)
	for _, t := range append(append([]*canvas.Text{}, r.titulo...), r.frase...) {
		r.objetos = append(r.objetos, t)
	}
	r.objetos = append(r.objetos, r.p.boton)
}

func (r *noEncontradaRenderer) MinSize() fyne.Size {
	ancho := max(MedirVersal(r.p.Sobrerrotulo, tema.TextoXS, SemiNegrita), r.p.boton.MinSize().Width, AnchoIlustracionNoEncontrada)
	return fyne.NewSize(ancho+2*tema.Espacio(4), r.p.AltoColumnaNoEncontrada(0)+2*tema.Espacio(12))
}

func (r *noEncontradaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *noEncontradaRenderer) Destroy()                     {}

// Layout centra la columna en las dos direcciones; con la ventana más baja que la
// columna se apoya en el `py-12`, que es lo que hace `justify-center` sin sitio.
func (r *noEncontradaRenderer) Layout(t fyne.Size) {
	if t.Width != r.ancho {
		r.rehacer(t.Width)
	}
	r.fondo.Resize(t)
	centro := t.Width / 2
	y := max((t.Height-r.p.AltoColumnaNoEncontrada(t.Width))/2, tema.Espacio(12))

	im := r.ilus.Objeto().Size()
	r.ilus.Objeto().Move(fyne.NewPos(centro-im.Width/2, y))
	y += im.Height + tema.Espacio(6)

	r.sobre.Colocar(centro-r.sobre.Ancho()/2, y)
	y += tema.TextoXS.Linea + tema.Espacio(2)

	y = colocarCentradas(r.titulo, EscalonTitulo(3), PesoTitulo(3), centro, y) + tema.Espacio(2)
	y = colocarCentradas(r.frase, tema.TextoBase, Normal, centro, y) + tema.Espacio(6)

	m := r.p.boton.MinSize()
	r.p.boton.Resize(m)
	r.p.boton.Move(fyne.NewPos(centro-m.Width/2, y))
}

// colocarCentradas pone cada línea centrada en `centro` y devuelve dónde acaba.
func colocarCentradas(lineas []*canvas.Text, e tema.Escalon, p Peso, centro, y float32) float32 {
	for _, l := range lineas {
		ColocarTexto(l, e, centro-Medir(l.Text, e, p).Width/2, y)
		y += e.Linea
	}
	return y
}

// Refresh repinta con la variante de ahora: la página es la misma en claro y en oscuro.
func (r *noEncontradaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg", v)
	r.fondo.Refresh()
	r.sobre.Pintar(tema.Color("text-subtle", v))
	if v != r.varianteActual {
		r.ilus = iconos.Pintar(iconos.VacioBusqueda, v).A(AnchoIlustracionNoEncontrada)
		r.rehacer(r.ancho)
		r.Layout(r.p.Size())
	}
	r.p.boton.Refresh()
}
