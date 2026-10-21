// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// clavePosicion es la frase de la posición POR TIPO de documento.
//
// Es una frase entera y no «unidad» + números pegados, por lo mismo que la web lo
// escribe así: en alemán y en francés cambian el orden y la preposición.
func clavePosicion(t Tipo) string {
	switch t {
	case Hoja:
		return "status.sheet"
	case Presentacion:
		return "status.slide"
	case Documento:
	}
	return "status.page"
}

// BarraEstado es la barra inferior de 28 px (§6.7): la posición, las palabras y el
// idioma a la izquierda; el zoom a la derecha.
//
// Medido: los rótulos en `text-xs` (10,5/14) sobre `--color-text-muted`, los botones de
// zoom de 24×24 y el deslizador de 84×16 con el porcentaje en un hueco de 35.
type BarraEstado struct {
	widget.BaseWidget

	ctx        *Contexto
	menos, mas *BotonCinta
	deslizador *widget.Slider
}

var _ fyne.Widget = (*BarraEstado)(nil)

// NuevaBarraEstado monta la barra.
func NuevaBarraEstado(ctx *Contexto) *BarraEstado {
	b := &BarraEstado{ctx: ctx}
	b.ExtendBaseWidget(b)
	b.menos = NuevoBotonCinta(iconos.EdMenos, ctx.Traducir("status.zoom.out"), func() {
		ctx.AjustarZoom(-ZoomPaso)
	})
	b.mas = NuevoBotonCinta(iconos.EdMas, ctx.Traducir("status.zoom.in"), func() {
		ctx.AjustarZoom(+ZoomPaso)
	})
	b.deslizador = widget.NewSlider(ZoomMin, ZoomMax)
	b.deslizador.Step = ZoomPaso
	b.deslizador.Value = float64(ctx.Zoom)
	b.deslizador.OnChanged = func(v float64) { ctx.PonerZoom(int(v)) }
	return b
}

// AccessibilityRole la anuncia como contenedor con nombre.
func (b *BarraEstado) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el nombre de la barra.
func (b *BarraEstado) AccessibilityLabel() string { return b.ctx.Traducir("status.label") }

// MinSize es la altura declarada.
func (b *BarraEstado) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(0, AltoEstado)
}

// cifras son las tres de la barra, con el hueco tapado si no hay lienzo.
func (b *BarraEstado) cifras() (unidad, total, palabras int) {
	if b.ctx.Lienzo == nil {
		return 1, 1, 0
	}
	return b.ctx.Lienzo.Estadisticas()
}

// CreateRenderer compone el fondo, la línea y los rótulos.
func (b *BarraEstado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := tema.Color("text-muted", v)
	r := &estadoRenderer{
		b:         b,
		fondo:     canvas.NewRectangle(tema.Color("surface", v)),
		linea:     canvas.NewRectangle(tema.Color("border", v)),
		posicion:  ui.Texto("", tema.TextoXS, ui.Normal, tinta),
		palabras:  ui.Texto("", tema.TextoXS, ui.Normal, tinta),
		porciento: ui.Texto("", tema.TextoXS, ui.Normal, tinta),
	}
	r.poner(v)
	return r
}

type estadoRenderer struct {
	b                             *BarraEstado
	fondo, linea                  *canvas.Rectangle
	posicion, palabras, porciento *canvas.Text
}

func (r *estadoRenderer) MinSize() fyne.Size { return r.b.MinSize() }

func (r *estadoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		r.fondo, r.linea, r.posicion, r.palabras,
		r.b.menos, r.b.deslizador, r.porciento, r.b.mas,
	}
}

func (r *estadoRenderer) Destroy() {}

func (r *estadoRenderer) poner(v tema.Variante) {
	unidad, total, palabras := r.b.cifras()
	c := r.b.ctx
	r.posicion.Text = c.Traducir(clavePosicion(c.Tipo), i18n.Vars{"page": unidad, "pages": total})
	r.palabras.Text = ""
	if c.Tipo == Documento {
		r.palabras.Text = c.Traducir("status.words", i18n.Vars{"count": palabras})
	}
	r.porciento.Text = strconv.Itoa(c.Zoom) + " %"
	for _, t := range []*canvas.Text{r.posicion, r.palabras, r.porciento} {
		t.Color = tema.Color("text-muted", v)
	}
}

func (r *estadoRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.linea.Move(fyne.NewPos(0, 0))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	// El contenido va bajo la línea de arriba: medido, los botones de 24 caen en
	// 774,5 con la barra en 772, que es 772 + 1 + (27−24)/2.
	alto := t.Height - tema.Borde
	centroTexto := tema.Borde + (alto-tema.TextoXS.Linea)/2

	x := MargenEstado
	ui.ColocarTexto(r.posicion, tema.TextoXS, x, centroTexto)
	x += ui.Medir(r.posicion.Text, tema.TextoXS, ui.Normal).Width + HuecoEstado
	if r.palabras.Text != "" {
		ui.ColocarTexto(r.palabras, tema.TextoXS, x, centroTexto)
		r.palabras.Show()
	} else {
		r.palabras.Hide()
	}

	// `ml-auto`: el zoom pegado a la derecha, en el orden −, deslizador, %, +.
	der := t.Width - MargenEstado
	coloca := func(o fyne.CanvasObject, w, h float32) {
		der -= w
		o.Move(fyne.NewPos(der, tema.Borde+(alto-h)/2))
		o.Resize(fyne.NewSize(w, h))
		der -= HuecoCarril // `gap-1`
	}
	coloca(r.b.mas, LadoBotonEstado, LadoBotonEstado)
	der -= AnchoPorciento
	ui.ColocarTexto(r.porciento, tema.TextoXS, der, centroTexto)
	der -= HuecoCarril
	coloca(r.b.deslizador, AnchoDeslizador, AltoDeslizador)
	coloca(r.b.menos, LadoBotonEstado, LadoBotonEstado)
}

func (r *estadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.poner(v)
	r.b.menos.Inerte = r.b.ctx.Zoom <= ZoomMin
	r.b.mas.Inerte = r.b.ctx.Zoom >= ZoomMax
	r.b.deslizador.Value = float64(r.b.ctx.Zoom)
	for _, o := range r.Objects() {
		o.Refresh()
	}
}
