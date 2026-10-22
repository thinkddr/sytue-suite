// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # La tarjeta del gráfico: `SheetChart`
//
//	<section class="border-border bg-surface absolute z-10 overflow-hidden rounded-lg
//	                border shadow-[var(--shadow-md)]" style="left:x; top:y; width:w+2">
//	  <div class="border-border flex h-7 items-center gap-1.5 border-b px-2">  cabecera
//	  <svg viewBox="0 0 W H" style="height: H·K">                               escena
//	  <div class="border-t flex flex-wrap gap-x-3 gap-y-0.5 px-2 py-1">       leyenda
//	  <div class="absolute right-0.5 bottom-0.5 h-2.5 w-2.5 border-r-2 border-b-2">  asa
//
// Va DENTRO de la rejilla —sus objetos los pinta el renderer de la rejilla, que recorta
// a su caja— y en coordenadas del CONTENIDO: se desplaza con la hoja y crece con el
// zoom, como la web. Lo que se ve de la escena sale de `verGrafico`, que es la función
// de la web línea a línea; aquí solo se PINTA: los trazos y los círculos, en un raster
// del mismo SVG; los rectángulos y las líneas rectas, con primitivas (el `crispEdges`
// del navegador); y los textos, con `canvas.Text` en su línea base.

// Las medidas de la tarjeta, en px de maqueta (antes del zoom).
var (
	altoCabeceraTarjeta = tema.Espacio(7)   // h-7
	margenTarjeta       = tema.Espacio(2)   // px-2
	huecoCabecera       = tema.Espacio(1.5) // gap-1.5
	ladoBotonTarjeta    = tema.Espacio(6)   // h-6 w-6
	altoVacioGrafico    = tema.Espacio(28)  // h-28
	ladoAsaTamano       = tema.Espacio(2.5) // h-2.5 w-2.5
	margenAsaTamano     = tema.Espacio(0.5) // right-0.5 bottom-0.5
	ladoMuestraLeyenda  = tema.Espacio(2)   // h-2 w-2
	huecoLeyendaX       = tema.Espacio(3)   // gap-x-3
	huecoLeyendaY       = tema.Espacio(0.5) // gap-y-0.5
	huecoMuestraLeyenda = tema.Espacio(1)   // gap-1
	maxNombreLeyenda    = tema.Espacio(24)  // max-w-24
	rellenoLeyendaY     = tema.Espacio(1)   // py-1
)

// Los lados de los iconos del editor (`ICON_SIZE`): sm es 14.
const iconoSM float32 = 14

// RadioMuestraLeyenda es el `rounded-[2px]` de la muestra de la leyenda.
const RadioMuestraLeyenda float32 = 2

// TarjetaGrafico es la tarjeta de UN gráfico de la hoja activa, por su id.
type TarjetaGrafico struct {
	widget.BaseWidget
	lz *Lienzo
	id string

	editando bool
	// El arrastre en curso: mover desde la cabecera o cambiar el tamaño desde el asa.
	modo  int
	desp  fyne.Delta
	medid fyne.Size

	editar, quitar *botonTarjeta
	editor         *editorGrafico
	// tam es la medida a zoom 1 del último pintado, y clave lo que la decidió: se
	// rehace la escena solo si cambia.
	tam   fyne.Size
	clave string
}

// Los gestos del arrastre.
const (
	gestoNada = iota
	gestoMover
	gestoTamano
)

var (
	_ fyne.Widget        = (*TarjetaGrafico)(nil)
	_ fyne.Draggable     = (*TarjetaGrafico)(nil)
	_ desktop.Mouseable  = (*TarjetaGrafico)(nil)
	_ fyne.Tappable      = (*TarjetaGrafico)(nil)
	_ fyne.Accessible    = (*TarjetaGrafico)(nil)
	_ desktop.Cursorable = (*TarjetaGrafico)(nil)
)

func nuevaTarjetaGrafico(lz *Lienzo, id string) *TarjetaGrafico {
	t := &TarjetaGrafico{lz: lz, id: id}
	t.editar = nuevoBotonTarjeta(iconos.EdAjustes, lz.t("chart.edit", nil), func() {
		t.editando = !t.editando
		t.editor = nil
		t.lz.Rejilla.Refresh()
	})
	t.quitar = nuevoBotonTarjeta(iconos.EdCerrar, lz.t("chart.remove", nil), func() {
		lz.quitarGrafico(id)
	})
	t.ExtendBaseWidget(t)
	return t
}

// grafico es el ChartSpec de esta tarjeta, o false si ya no existe.
func (t *TarjetaGrafico) grafico() (Grafico, bool) {
	for _, g := range t.lz.Libro.Graficos() {
		if g.ID == t.id {
			return g, true
		}
	}
	return Grafico{}, false
}

// mostrado es el gráfico con el tamaño provisional del asa, si se está arrastrando.
func (t *TarjetaGrafico) mostrado() Grafico {
	g, _ := t.grafico()
	if t.modo == gestoTamano {
		w, h := float64(t.medid.Width), float64(t.medid.Height)
		g.W, g.H = &w, &h
	}
	return g
}

func (t *TarjetaGrafico) vista(g Grafico) vistaGrafico {
	l := t.lz.Libro
	return verGrafico(g, func(c, f int) formula.Value { return l.Valor(c, f) }, t.lz.t("chart.other", nil))
}

// area es el tamaño del área del gráfico en px de maqueta.
func areaDe(g Grafico) (w, h float32) {
	w, h = float32(anchoAreaDefecto), float32(altoAreaDefecto)
	if g.W != nil {
		w = float32(*g.W)
	}
	if g.H != nil {
		h = float32(*g.H)
	}
	return w, h
}

// Posicion es `left`/`top`: 96 y 48 si el gráfico no dice.
func (g Grafico) Posicion() (x, y float32) {
	x, y = 96, 48
	if g.X != nil {
		x = float32(*g.X)
	}
	if g.Y != nil {
		y = float32(*g.Y)
	}
	return x, y
}

// medida es el tamaño de la tarjeta a zoom 1: el área con su borde, la cabecera y la
// leyenda (o el editor, o el cartel de vacío).
func (t *TarjetaGrafico) medida(g Grafico, v vistaGrafico) fyne.Size {
	w, h := areaDe(g)
	ancho := w + 2*tema.Borde
	alto := 2*tema.Borde + altoCabeceraTarjeta
	switch {
	case t.editando:
		alto += t.formulario(g, v).MinSize().Height
	case v.Vacio:
		alto += altoVacioGrafico
	default:
		conLeyenda := leyendaVisible(g, len(v.Leyenda))
		if conLeyenda && g.Legend == "right" {
			alto += max(h, altoLeyendaLado(len(v.Leyenda)))
		} else {
			alto += h
			if conLeyenda {
				alto += altoLeyenda(v.Leyenda, w)
			}
		}
	}
	return fyne.NewSize(ancho, alto)
}

// formulario es el editor del gráfico, que se construye al abrirlo.
func (t *TarjetaGrafico) formulario(g Grafico, v vistaGrafico) *editorGrafico {
	if t.editor == nil {
		t.editor = nuevoEditorGrafico(t.lz, g, v.Series, func(parche map[string]any) {
			t.lz.editarGrafico(t.id, parche)
			t.editando, t.editor = false, nil
			t.lz.Rejilla.Refresh()
		}, func() {
			t.editando, t.editor = false, nil
			t.lz.Rejilla.Refresh()
		})
	}
	return t.editor
}

// filasLeyenda reparte la leyenda de abajo o de arriba en filas: `flex flex-wrap` con
// `gap-x-3` en un ancho de `w − px-2·2`.
func filasLeyenda(items []itemLeyenda, w float32) [][]int {
	cabe := w - 2*margenTarjeta
	var filas [][]int
	var x float32
	for i, it := range items {
		ancho := anchoItemLeyenda(it)
		if len(filas) == 0 || (x > 0 && x+huecoLeyendaX+ancho > cabe) {
			filas = append(filas, nil)
			x = 0
		} else if x > 0 {
			x += huecoLeyendaX
		}
		filas[len(filas)-1] = append(filas[len(filas)-1], i)
		x += ancho
	}
	return filas
}

func anchoItemLeyenda(it itemLeyenda) float32 {
	return ladoMuestraLeyenda + huecoMuestraLeyenda + min(ui.Medir(it.Nombre, tema.Texto2XS, ui.Normal).Width, maxNombreLeyenda)
}

func altoLeyenda(items []itemLeyenda, w float32) float32 {
	n := float32(len(filasLeyenda(items, w)))
	return tema.Borde + 2*rellenoLeyendaY + n*tema.Texto2XS.Linea + (n-1)*huecoLeyendaY
}

func altoLeyendaLado(n int) float32 {
	return 2*rellenoLeyendaY + float32(n)*tema.Texto2XS.Linea + float32(max(n-1, 0))*huecoLeyendaY
}

// ---- gestos ----

// MouseDown y MouseUp se quedan el clic: sin ellos, pulsar en la tarjeta seleccionaría
// la celda de debajo.
func (t *TarjetaGrafico) MouseDown(*desktop.MouseEvent) {}
func (t *TarjetaGrafico) MouseUp(*desktop.MouseEvent)   {}
func (t *TarjetaGrafico) Tapped(*fyne.PointEvent)       {}

// Cursor es el de la flecha: Fyne no trae el de mover.
func (t *TarjetaGrafico) Cursor() desktop.Cursor { return desktop.DefaultCursor }

// Dragged mueve la tarjeta si el arrastre empezó en la cabecera, o cambia su tamaño si
// empezó en el asa de la esquina. Los desplazamientos van en px de la pantalla y se
// guardan en los de la hoja (sin zoom).
func (t *TarjetaGrafico) Dragged(e *fyne.DragEvent) {
	z := t.lz.Rejilla.z()
	if t.modo == gestoNada {
		inicio := fyne.NewPos(e.Position.X-e.Dragged.DX, e.Position.Y-e.Dragged.DY)
		tam := t.Size()
		asa := (ladoAsaTamano + margenAsaTamano + 4) * z
		switch {
		case !t.editando && inicio.X >= tam.Width-asa && inicio.Y >= tam.Height-asa:
			g, _ := t.grafico()
			w, h := areaDe(g)
			t.modo, t.medid = gestoTamano, fyne.NewSize(w, h)
		case inicio.Y < (tema.Borde+altoCabeceraTarjeta)*z:
			t.modo, t.desp = gestoMover, fyne.Delta{}
		default:
			return
		}
	}
	switch t.modo {
	case gestoMover:
		t.desp.DX += e.Dragged.DX / z
		t.desp.DY += e.Dragged.DY / z
	case gestoTamano:
		t.medid.Width = clampF(t.medid.Width+e.Dragged.DX/z, TamanoMinGrafico, TamanoMaxGrafico)
		t.medid.Height = clampF(t.medid.Height+e.Dragged.DY/z, TamanoMinGrafico, TamanoMaxGrafico)
	}
	t.lz.Rejilla.Refresh()
}

// DragEnd guarda la posición o el tamaño, redondeados como la web.
func (t *TarjetaGrafico) DragEnd() {
	modo := t.modo
	t.modo = gestoNada
	g, ok := t.grafico()
	if !ok {
		return
	}
	switch modo {
	case gestoMover:
		if t.desp.DX == 0 && t.desp.DY == 0 {
			return
		}
		x, y := g.Posicion()
		t.lz.moverObjeto(t.id, redondearJS(float64(x+t.desp.DX)), redondearJS(float64(y+t.desp.DY)))
		t.desp = fyne.Delta{}
	case gestoTamano:
		w, h := areaDe(g)
		if t.medid.Width != w || t.medid.Height != h {
			t.lz.editarGrafico(t.id, map[string]any{"w": float64(t.medid.Width), "h": float64(t.medid.Height)})
		}
	}
	t.lz.Rejilla.Refresh()
}

// AccessibilityLabel es `aria-label={t('chart.card.label', { title })}`.
func (t *TarjetaGrafico) AccessibilityLabel() string {
	g, _ := t.grafico()
	return t.lz.t("chart.card.label", i18n.Vars{"title": t.titulo(g)})
}

func (t *TarjetaGrafico) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (t *TarjetaGrafico) titulo(g Grafico) string {
	if g.Title != "" {
		return g.Title
	}
	return t.lz.t("chart.defaultTitle", i18n.Vars{"type": t.lz.t("chart.type."+g.Type, nil), "range": g.Range})
}

// ---- pintar ----

func (t *TarjetaGrafico) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaGraficoRenderer{
		t: t, sombra: ui.NuevaSombra(tema.SombraMD, tema.RadioLG),
		caja:      ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		lineaCab:  canvas.NewRectangle(tema.Color("border", v)),
		icono:     iconos.TrazoEdConTinta(iconos.EdGrafico, iconoSM, colorDe(apagado)),
		tituloTxt: ui.Texto("", tema.TextoXS, ui.Medio, colorDe(tintaGrafico)),
		asaDer:    canvas.NewRectangle(colorDe(colorEje)),
		asaAbj:    canvas.NewRectangle(colorDe(colorEje)),
		vacio:     ui.Texto("", tema.TextoXS, ui.Normal, colorDe(apagado)),
	}
	r.Refresh()
	return r
}

func colorDe(hex string) color.NRGBA {
	c, _ := tokens.ParseColor(hex)
	return c
}

type tarjetaGraficoRenderer struct {
	t         *TarjetaGrafico
	sombra    *ui.Sombra
	caja      *ui.Caja
	lineaCab  *canvas.Rectangle
	icono     *iconos.Dibujo
	tituloTxt *canvas.Text
	asaDer    *canvas.Rectangle
	asaAbj    *canvas.Rectangle
	vacio     *canvas.Text

	// Lo de la escena y la leyenda, rehecho en cada Refresh.
	escena    []fyne.CanvasObject
	colocar   []func(o fyne.Position, s float32) // uno por objeto de la escena
	leyenda   []fyne.CanvasObject
	vista     vistaGrafico
	g         Grafico
	objetos   []fyne.CanvasObject
	ultimoTam fyne.Size
}

func (r *tarjetaGraficoRenderer) Destroy()                     {}
func (r *tarjetaGraficoRenderer) Objects() []fyne.CanvasObject { return r.objetos }

// MinSize es la medida con el zoom de la rejilla.
func (r *tarjetaGraficoRenderer) MinSize() fyne.Size {
	z := r.t.lz.Rejilla.z()
	return fyne.NewSize(r.t.tam.Width*z, r.t.tam.Height*z)
}

func (r *tarjetaGraficoRenderer) Refresh() {
	t := r.t
	r.g = t.mostrado()
	r.vista = t.vista(r.g)
	t.tam = t.medida(r.g, r.vista)
	r.construirEscena()
	r.construirLeyenda()
	o := []fyne.CanvasObject{r.sombra.Objeto()}
	o = append(o, r.caja.Objetos()...)
	o = append(o, r.lineaCab, r.icono.Objeto(), r.tituloTxt, t.editar, t.quitar)
	switch {
	case t.editando:
		o = append(o, t.formulario(r.g, r.vista))
	case r.vista.Vacio:
		o = append(o, r.vacio)
	default:
		o = append(o, r.escena...)
		o = append(o, r.leyenda...)
	}
	if !t.editando {
		o = append(o, r.asaDer, r.asaAbj)
	}
	r.objetos = o
	r.Layout(t.Size())
	canvas.Refresh(t)
}

// construirEscena convierte las marcas en objetos: los trazos y círculos seguidos van
// a un mismo raster (sin cambiar el orden de apilado), los rectángulos y las líneas
// rectas son primitivas y los textos, `canvas.Text`.
func (r *tarjetaGraficoRenderer) construirEscena() {
	r.escena, r.colocar = nil, nil
	e := r.vista.Escena
	var svg strings.Builder
	cerrar := func() {
		if svg.Len() == 0 {
			return
		}
		rs := nuevoRasterSVG()
		rs.poner(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + numSVG(e.W) + " " + numSVG(e.H) +
			`" width="` + numSVG(e.W) + `" height="` + numSVG(e.H) + `">` + svg.String() + `</svg>`)
		svg.Reset()
		r.escena = append(r.escena, rs.raster)
		r.colocar = append(r.colocar, func(o fyne.Position, s float32) {
			rs.raster.Move(o)
			rs.raster.Resize(fyne.NewSize(float32(e.W)*s, float32(e.H)*s))
		})
	}
	for _, m := range e.Marcas {
		switch {
		case m.K == "rect":
			cerrar()
			r.primitiva(colorDe(m.Fill), m.X, m.Y, m.W, m.H)
		case m.K == "line" && (m.X1 == m.X2 || m.Y1 == m.Y2):
			cerrar()
			// Una línea de 1 de ancho, centrada en su coordenada: la mitad a cada lado.
			if m.X1 == m.X2 {
				r.primitiva(colorDe(m.Stroke), m.X1-0.5, min(m.Y1, m.Y2), 1, max(m.Y1, m.Y2)-min(m.Y1, m.Y2))
			} else {
				r.primitiva(colorDe(m.Stroke), min(m.X1, m.X2), m.Y1-0.5, max(m.X1, m.X2)-min(m.X1, m.X2), 1)
			}
		case m.K == "text":
			cerrar()
			r.texto(m)
		default:
			svg.WriteString(svgMarca(m))
		}
	}
	cerrar()
}

func (r *tarjetaGraficoRenderer) primitiva(c color.NRGBA, x, y, w, h float64) {
	rc := canvas.NewRectangle(c)
	r.escena = append(r.escena, rc)
	r.colocar = append(r.colocar, func(o fyne.Position, s float32) {
		rc.Move(fyne.NewPos(o.X+float32(x)*s, o.Y+float32(y)*s))
		rc.Resize(fyne.NewSize(float32(w)*s, float32(h)*s))
	})
}

// texto pone un `<text>` en su línea base, con su `text-anchor`.
func (r *tarjetaGraficoRenderer) texto(m marca) {
	tx := canvas.NewText(m.Text, colorDe(m.Fill))
	tx.FontSource = tema.Regular
	r.escena = append(r.escena, tx)
	r.colocar = append(r.colocar, func(o fyne.Position, s float32) {
		tx.TextSize = float32(m.Size) * s
		tam, base := fyne.CurrentApp().Driver().RenderedTextSize(m.Text, tx.TextSize, tx.TextStyle, tx.FontSource)
		x := o.X + float32(m.X)*s
		switch m.Anchor {
		case "middle":
			x -= tam.Width / 2
		case "end":
			x -= tam.Width
		}
		tx.Move(fyne.NewPos(x, o.Y+float32(m.Y)*s-base))
		tx.Resize(tam)
	})
}

// svgMarca escribe una marca de trazo o círculo como la pinta `SceneSvg` (sin el
// `<title>`, que aquí no hay dónde enseñar).
func svgMarca(m marca) string {
	atrib := func(nombre string, v float64) string {
		if v == 0 {
			return ""
		}
		return ` ` + nombre + `="` + numSVG(v) + `"`
	}
	trazo := ""
	if m.Stroke != "" {
		trazo = ` stroke="` + m.Stroke + `"` + atrib("stroke-width", m.SW)
	}
	switch m.K {
	case "circle":
		return `<circle cx="` + numSVG(m.CX) + `" cy="` + numSVG(m.CY) + `" r="` + numSVG(m.R) + `" fill="` + m.Fill + `"` +
			atrib("fill-opacity", m.Op) + trazo + `/>`
	case "line": // las diagonales del radar
		return `<path d="M` + numSVG(m.X1) + " " + numSVG(m.Y1) + "L" + numSVG(m.X2) + " " + numSVG(m.Y2) +
			`" fill="none" stroke="` + m.Stroke + `" stroke-width="1"/>`
	}
	extra := ` stroke-linejoin="round"`
	if m.Round {
		extra += ` stroke-linecap="round"`
	}
	return `<path d="` + m.D + `" fill="` + m.Fill + `"` + atrib("fill-opacity", m.Op) + trazo + extra + `/>`
}

// construirLeyenda arma las muestras y los nombres.
func (r *tarjetaGraficoRenderer) construirLeyenda() {
	r.leyenda = nil
	if !leyendaVisible(r.g, len(r.vista.Leyenda)) {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.leyenda = append(r.leyenda, canvas.NewRectangle(tema.Color("border", v))) // border-t, border-b o border-l
	for _, it := range r.vista.Leyenda {
		m := canvas.NewRectangle(colorDe(it.Color))
		m.CornerRadius = RadioMuestraLeyenda
		r.leyenda = append(r.leyenda, m, ui.Texto(it.Nombre, tema.Texto2XS, ui.Normal, colorDe(apagado)))
	}
}

func (r *tarjetaGraficoRenderer) Layout(tam fyne.Size) {
	t := r.t
	z := t.lz.Rejilla.z()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := tema.Borde * z
	r.sombra.Ajustar(fyne.NewPos(0, 0), tam, v)
	r.caja.Redimensionar(tam)
	cab := altoCabeceraTarjeta * z
	// La cabecera: la línea de abajo va DENTRO de sus h-7.
	caja(r.lineaCab, tema.Color("border", v), b, b+cab-b, tam.Width-2*b, b)
	x := b + margenTarjeta*z
	r.icono.Colocar(x, b+(cab-b-iconoSM*z)/2, iconoSM*z)
	x += (iconoSM + huecoCabecera) * z
	lado := ladoBotonTarjeta * z
	der := tam.Width - b - margenTarjeta*z
	for _, bt := range []*botonTarjeta{t.quitar, t.editar} {
		der -= lado
		bt.Move(fyne.NewPos(der, b+(cab-b-lado)/2))
		bt.Resize(fyne.NewSize(lado, lado))
		der -= huecoCabecera * z
	}
	esc := tema.Escalon{Tam: tema.TextoXS.Tam * z, Linea: tema.TextoXS.Linea * z}
	r.tituloTxt.TextSize = esc.Tam
	ui.Truncar(r.tituloTxt, t.titulo(r.g), esc, ui.Medio, max(der-x, 0))
	ui.ColocarTexto(r.tituloTxt, esc, x, b+(cab-b-esc.Linea)/2)

	cuerpo := fyne.NewPos(b, b+cab)
	ancho := tam.Width - 2*b
	switch {
	case t.editando:
		f := t.formulario(r.g, r.vista)
		f.Move(cuerpo)
		f.Resize(fyne.NewSize(ancho, tam.Height-b-cuerpo.Y))
	case r.vista.Vacio:
		esc := tema.Escalon{Tam: tema.TextoXS.Tam * z, Linea: tema.TextoXS.Linea * z}
		r.vacio.Text, r.vacio.TextSize = t.lz.t("chart.empty", nil), esc.Tam
		w := ui.Medir(r.vacio.Text, tema.TextoXS, ui.Normal).Width * z
		ui.ColocarTexto(r.vacio, esc, cuerpo.X+(ancho-w)/2, cuerpo.Y+(altoVacioGrafico*z-esc.Linea)/2)
	default:
		r.colocarEscenaYLeyenda(cuerpo, ancho, z)
	}
	// El asa: `right-0.5 bottom-0.5 h-2.5 w-2.5 border-r-2 border-b-2`.
	a, m, g := ladoAsaTamano*z, margenAsaTamano*z, 2*tema.Borde*z
	caja(r.asaDer, colorDe(colorEje), tam.Width-m-g, tam.Height-m-a, g, a)
	caja(r.asaAbj, colorDe(colorEje), tam.Width-m-a, tam.Height-m-g, a, g)
	r.ultimoTam = tam
}

func (r *tarjetaGraficoRenderer) colocarEscenaYLeyenda(cuerpo fyne.Position, ancho, z float32) {
	g := r.g
	s := float32(escalaGrafico) * z
	origen := cuerpo
	conLeyenda := len(r.leyenda) > 0
	if conLeyenda && g.Legend == "top" {
		origen.Y += r.colocarLeyenda(cuerpo, ancho, z, true)
	}
	for _, c := range r.colocar {
		c(origen, s)
	}
	if !conLeyenda {
		return
	}
	switch g.Legend {
	case "right":
		_, h := areaDe(g)
		x := origen.X + float32(r.vista.Escena.W)*s
		linea, _ := r.leyenda[0].(*canvas.Rectangle)
		colocarLinea(linea, x, origen.Y, tema.Borde*z, max(h, altoLeyendaLado(len(r.vista.Leyenda)))*z)
		r.colocarLeyendaLado(fyne.NewPos(x+tema.Borde*z, origen.Y), z)
	case "top":
	default:
		r.colocarLeyenda(fyne.NewPos(cuerpo.X, origen.Y+float32(r.vista.Escena.H)*s), ancho, z, false)
	}
}

// colocarLeyenda pone la de arriba o la de abajo y devuelve lo que ocupa.
func (r *tarjetaGraficoRenderer) colocarLeyenda(pos fyne.Position, ancho, z float32, arriba bool) float32 {
	items := r.vista.Leyenda
	w, _ := areaDe(r.g)
	filas := filasLeyenda(items, w)
	alto := altoLeyenda(items, w) * z
	linea, _ := r.leyenda[0].(*canvas.Rectangle)
	y := pos.Y + tema.Borde*z
	if arriba {
		colocarLinea(linea, pos.X, pos.Y+alto-tema.Borde*z, ancho, tema.Borde*z)
		y = pos.Y
	} else {
		colocarLinea(linea, pos.X, pos.Y, ancho, tema.Borde*z)
	}
	y += rellenoLeyendaY * z
	for _, fila := range filas {
		x := pos.X + margenTarjeta*z
		for _, i := range fila {
			x += r.colocarItemLeyenda(i, x, y, z) + huecoLeyendaX*z
		}
		y += (tema.Texto2XS.Linea + huecoLeyendaY) * z
	}
	return alto
}

// colocarLeyendaLado pone la de la derecha: `w-24 flex-col gap-0.5 px-2 py-1`.
func (r *tarjetaGraficoRenderer) colocarLeyendaLado(pos fyne.Position, z float32) {
	y := pos.Y + rellenoLeyendaY*z
	for i := range r.vista.Leyenda {
		r.colocarItemLeyenda(i, pos.X+margenTarjeta*z, y, z)
		y += (tema.Texto2XS.Linea + huecoLeyendaY) * z
	}
}

// colocarItemLeyenda pone la muestra y el nombre del i-ésimo; devuelve su ancho.
func (r *tarjetaGraficoRenderer) colocarItemLeyenda(i int, x, y, z float32) float32 {
	muestra, _ := r.leyenda[1+2*i].(*canvas.Rectangle)
	nombre, _ := r.leyenda[2+2*i].(*canvas.Text)
	lado := ladoMuestraLeyenda * z
	linea := tema.Texto2XS.Linea * z
	muestra.Move(fyne.NewPos(x, y+(linea-lado)/2))
	muestra.Resize(fyne.NewSize(lado, lado))
	esc := tema.Escalon{Tam: tema.Texto2XS.Tam * z, Linea: linea}
	nombre.TextSize = esc.Tam
	w := ui.Truncar(nombre, r.vista.Leyenda[i].Nombre, esc, ui.Normal, maxNombreLeyenda*z)
	ui.ColocarTexto(nombre, esc, x+lado+huecoMuestraLeyenda*z, y)
	return lado + huecoMuestraLeyenda*z + min(w, maxNombreLeyenda*z)
}

// ---- el botón de la cabecera ----

// botonTarjeta es el botón de la cabecera de la tarjeta:
//
//	inline-flex h-6 w-6 items-center justify-center rounded-sm hover:bg-bg-subtle
//
// con el icono `sm` (14) en la tinta apagada de la tarjeta.
type botonTarjeta struct {
	ui.Pulsable
	icono  iconos.Ed
	Activo bool
}

func nuevoBotonTarjeta(i iconos.Ed, nombre string, alPulsar func()) *botonTarjeta {
	b := &botonTarjeta{icono: i}
	b.Nombre, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonTarjeta) MinSize() fyne.Size { return fyne.NewSize(ladoBotonTarjeta, ladoBotonTarjeta) }

func (b *botonTarjeta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonTarjetaRenderer{
		b: b, aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		caja:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM),
		glifo: iconos.TrazoEdConTinta(b.icono, iconoSM, colorDe(apagado)),
	}
	r.objetos = r.aro.Antes(append(r.caja.Objetos(), r.glifo.Objeto())...)
	return r
}

type botonTarjetaRenderer struct {
	b       *botonTarjeta
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *botonTarjetaRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonTarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonTarjetaRenderer) Destroy()                     {}

func (r *botonTarjetaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	lado := min(iconoSM*t.Width/ladoBotonTarjeta, t.Width)
	r.glifo.Colocar((t.Width-lado)/2, (t.Height-lado)/2, lado)
	r.aro.ColocarEn(fyne.NewPos(0, 0), t)
}

func (r *botonTarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.b.Encima() || r.b.Activo {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.Layout(r.b.Size())
}

// ---- en el libro ----

// Graficos son los de la hoja activa.
func (l *Libro) Graficos() []Grafico { return l.h().graficos }

// AnadirGrafico es `chartadd`: un gráfico nuevo con id propio, en cascada para que las
// tarjetas no se apilen exactas.
func (l *Libro) AnadirGrafico(crudo graficoCrudo) bool {
	n := len(l.h().graficos)
	crudo.ID = nuevoID()
	crudo.X, crudo.Y = float64(96+(n%8)*28), float64(48+(n%8)*28)
	g, ok := crudo.sanear()
	if !ok {
		return false
	}
	l.anotar()
	h := l.h()
	h.graficos = append(slices0(h.graficos), g)
	l.version++
	return true
}

// cambiarGrafico sustituye el gráfico de ese id por lo que devuelva `f` (nada = se quita).
func (l *Libro) cambiarGrafico(id string, f func(Grafico) (Grafico, bool)) bool {
	h := l.h()
	for i, g := range h.graficos {
		if g.ID != id {
			continue
		}
		nuevo, sigue := f(g)
		l.anotar()
		lista := slices0(h.graficos)
		if sigue {
			lista[i] = nuevo
		} else {
			lista = append(lista[:i], lista[i+1:]...)
		}
		if len(lista) == 0 {
			lista = nil
		}
		l.h().graficos = lista
		l.version++
		return true
	}
	return false
}

// EditarGrafico es `editChart`: `sanitizeChartSpec({ ...ch, ...patch })`.
func (l *Libro) EditarGrafico(id string, parche map[string]any) bool {
	return l.cambiarGrafico(id, func(g Grafico) (Grafico, bool) {
		b, _ := json.Marshal(g)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		for k, v := range parche {
			m[k] = v
		}
		b, _ = json.Marshal(m)
		var crudo graficoCrudo
		if json.Unmarshal(b, &crudo) != nil {
			return g, true
		}
		nuevo, ok := crudo.sanear()
		if !ok {
			return g, true
		}
		return nuevo, true
	})
}

// slices0 es una copia que se puede modificar sin tocar la foto de deshacer.
func slices0[T any](s []T) []T { return append([]T(nil), s...) }

// ---- el lienzo ----

// quitarGrafico es `removeChart`.
func (c *Lienzo) quitarGrafico(id string) {
	if c.objetosCerrados() {
		return
	}
	if c.Libro.cambiarGrafico(id, func(g Grafico) (Grafico, bool) { return g, false }) {
		c.trasCambio()
	}
}

// editarGrafico es `editChart`.
func (c *Lienzo) editarGrafico(id string, parche map[string]any) {
	if c.objetosCerrados() {
		return
	}
	if c.Libro.EditarGrafico(id, parche) {
		c.trasCambio()
	}
}

// moverObjeto es `moveChart`/`moveSlicer`: la posición nueva, con el tope de la web.
func (c *Lienzo) moverObjeto(id string, x, y float64) {
	if c.objetosCerrados() {
		return
	}
	x, y = min(TopePosicion, max(0, x)), min(TopePosicion, max(0, y))
	if c.Libro.EditarGrafico(id, map[string]any{"x": x, "y": y}) || c.Libro.MoverSegmentacion(id, x, y) {
		c.trasCambio()
	}
}

// objetosCerrados dice si no se puede tocar un objeto: solo lectura, o la hoja protegida
// sin el permiso de objetos.
func (c *Lienzo) objetosCerrados() bool {
	if c.Rejilla.SoloLectura {
		return true
	}
	if p := c.Libro.Proteccion(); p != nil && !p.Objects {
		c.avisarProtegida(c.Rejilla.sel)
		return true
	}
	return false
}

// colocarLinea pone una línea de la leyenda con el color que ya tiene.
func colocarLinea(r *canvas.Rectangle, x, y, w, h float32) {
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(w, h))
	r.Show()
}
