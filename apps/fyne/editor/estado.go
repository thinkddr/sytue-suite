// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"math"
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

// LienzoConIdioma es el lienzo cuyo documento guarda su idioma: el folio de Docus, en la
// capa de la página (`data-lang`), como `StatusBar.tsx`.
type LienzoConIdioma interface {
	// IdiomaDocumento es el que guarda el documento ("" sin él) y si se puede cambiar.
	IdiomaDocumento() (idioma string, editable bool)
	// PonerIdiomaDocumento lo guarda en el documento, con su paso de deshacer.
	PonerIdiomaDocumento(idioma string)
}

// BarraEstado es la barra inferior de 28 px (§6.7): la posición, las palabras y el
// idioma a la izquierda; el zoom a la derecha. Las palabras son un botón que abre el
// recuento detallado (Ctrl/Cmd+Mayús+C) y, con selección, dicen «12 de 340». El idioma
// es el DEL DOCUMENTO: un botón con su nombre que abre los 26 de la suite.
//
// Medido: los rótulos en `text-xs` (10,5/14) sobre `--color-text-muted`, los botones de
// zoom de 24×24 y el deslizador de 84×16 con el porcentaje en un hueco de 35.
type BarraEstado struct {
	widget.BaseWidget

	ctx        *Contexto
	menos, mas *BotonCinta
	deslizador *ui.Deslizador
	// porciento es el «100 %» de la barra, que se pulsa para volver al 100 (el
	// `<button>` de StatusBar.tsx; Ctrl/Cmd+0 hace lo mismo).
	porciento *ui.Enlace
	// palabras es la cifra de Docus, que se pulsa para abrir el recuento (el `<button>`
	// de las palabras de StatusBar.tsx).
	palabras *ui.Enlace
	// idioma es el `<select>` del idioma del documento: su nombre, que abre la lista.
	idioma     *ui.Enlace
	menuIdioma *ui.Menu
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
	// El `<input type="range">` de la suite y no `widget.Slider`: el carril de este era la
	// superficie del tema y sobre la barra blanca no se veía.
	// Como el `Step` de `widget.Slider`, solo avisa al cambiar de decena: cada aviso
	// recompone el marco entero, y arrastrar dentro de la misma no cambia nada. El pulgar
	// vuelve a su decena, que es lo que hace el `step={10}` de la web.
	b.deslizador = ui.NuevoDeslizador(ZoomMin, ZoomMax, float64(ctx.Zoom), func(v float64) {
		if z := zoomEnPaso(v); z != ctx.Zoom {
			ctx.PonerZoom(z)
			return
		}
		b.deslizador.Valor = float64(ctx.Zoom)
	}).Con(ctx.Traducir("status.zoom"))
	b.deslizador.Paso = ZoomPaso
	b.deslizador.TextoValor = strconv.Itoa(ctx.Zoom) + " %"
	b.porciento = ui.NuevoEnlace("", tema.TextoXS, ui.Normal, "text-muted", func() { ctx.PonerZoom(100) })
	b.porciento.Boton, b.porciento.Liso = true, true
	b.ponerPorciento()
	b.palabras = ui.NuevoEnlace("", tema.TextoXS, ui.Normal, "text-muted", b.abrirRecuento)
	b.palabras.Boton, b.palabras.Liso = true, true
	b.idioma = ui.NuevoEnlace("", tema.TextoXS, ui.Normal, "text-muted", b.abrirIdiomas)
	b.idioma.Boton, b.idioma.Liso = true, true
	if ctx.Tipo == Documento {
		ctx.AbrirRecuento = b.abrirRecuento
	}
	return b
}

// lienzoConIdioma es el lienzo si guarda el idioma del documento (solo Docus).
func (b *BarraEstado) lienzoConIdioma() (LienzoConIdioma, bool) {
	if b.ctx.Tipo != Documento {
		return nil, false
	}
	l, ok := b.ctx.Lienzo.(LienzoConIdioma)
	return l, ok
}

// idiomaActual es el del documento o, sin él, el de la interfaz, como el `value` del
// `<select>` de la web; y si se puede cambiar.
func (b *BarraEstado) idiomaActual() (i18n.Locale, bool) {
	l, ok := b.lienzoConIdioma()
	if !ok {
		return b.ctx.Loc, false
	}
	idioma, editable := l.IdiomaDocumento()
	if !i18n.IsLocale(idioma) {
		idioma = string(b.ctx.Loc)
	}
	return i18n.Locale(idioma), editable && !b.ctx.SoloLectura
}

// abrirIdiomas abre la lista de los 26, cada uno con su nombre en su idioma y el del
// documento marcado. Elegir uno lo guarda en el documento.
func (b *BarraEstado) abrirIdiomas() {
	l, ok := b.lienzoConIdioma()
	actual, editable := b.idiomaActual()
	if !ok || !editable {
		return
	}
	if b.menuIdioma != nil && b.menuIdioma.Abierto() {
		b.menuIdioma.Cerrar()
		return
	}
	elems := make([]fyne.CanvasObject, len(i18n.Locales))
	for i, loc := range i18n.Locales {
		e := ui.NuevoElementoMenu(i18n.Names[loc], iconos.Ninguno, func() {
			if loc != actual {
				l.PonerIdiomaDocumento(string(loc))
			}
			b.Refresh()
		})
		e.Denso, e.Marcada = true, loc == actual
		elems[i] = e
	}
	b.menuIdioma = ui.NuevoMenu(elems...)
	b.menuIdioma.AlInicio, b.menuIdioma.Rotulo = true, b.ctx.Traducir("status.language")
	b.menuIdioma.Mostrar(b.idioma)
}

// zoomEnPaso redondea a la decena, como hacía el `Step` de `widget.Slider`.
func zoomEnPaso(v float64) int {
	return int(math.Round(v/ZoomPaso)) * ZoomPaso
}

// ponerPorciento pone el texto («130 %») y el nombre accesible, que empieza por lo que
// se lee: quien manda por voz dice lo que ve.
func (b *BarraEstado) ponerPorciento() {
	b.porciento.Texto = strconv.Itoa(b.ctx.Zoom) + " %"
	b.porciento.Nombre = b.ctx.Traducir("status.zoom.reset", i18n.Vars{"zoom": b.ctx.Zoom})
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
		b:        b,
		fondo:    canvas.NewRectangle(tema.Color("surface", v)),
		linea:    canvas.NewRectangle(tema.Color("border", v)),
		posicion: ui.Texto("", tema.TextoXS, ui.Normal, tinta),
	}
	r.poner(v)
	return r
}

type estadoRenderer struct {
	b            *BarraEstado
	fondo, linea *canvas.Rectangle
	posicion     *canvas.Text
}

func (r *estadoRenderer) MinSize() fyne.Size { return r.b.MinSize() }

func (r *estadoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		r.fondo, r.linea, r.posicion, r.b.palabras, r.b.idioma,
		r.b.menos, r.b.deslizador, r.b.porciento, r.b.mas,
	}
}

func (r *estadoRenderer) Destroy() {}

func (r *estadoRenderer) poner(v tema.Variante) {
	unidad, total, palabras := r.b.cifras()
	c := r.b.ctx
	r.posicion.Text = c.Traducir(clavePosicion(c.Tipo), i18n.Vars{"page": unidad, "pages": total})
	r.b.palabras.Texto = ""
	if c.Tipo == Documento {
		r.b.palabras.Texto = c.Traducir("status.words", i18n.Vars{"count": palabras})
		if l, ok := c.Lienzo.(LienzoConRecuento); ok {
			if sel, hay := l.PalabrasSeleccionadas(); hay {
				r.b.palabras.Texto = c.Traducir("status.words.selection", i18n.Vars{"selected": sel, "count": palabras})
			}
		}
	}
	r.b.palabras.Nombre = r.b.palabras.Texto
	r.b.idioma.Texto = ""
	if _, ok := r.b.lienzoConIdioma(); ok {
		idioma, editable := r.b.idiomaActual()
		r.b.idioma.Texto = i18n.Names[idioma]
		r.b.idioma.Inerte = !editable
	}
	// El `aria-label` del `<select>`; lo que se lee es el valor, que va en el texto.
	r.b.idioma.Nombre = c.Traducir("status.language")
	r.b.ponerPorciento()
	r.posicion.Color = tema.Color("text-muted", v)
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
	if r.b.palabras.Texto != "" {
		p := ui.MedidaEnlace(r.b.palabras)
		r.b.palabras.Move(fyne.NewPos(x, tema.Borde+(alto-p.Height)/2))
		r.b.palabras.Resize(p)
		r.b.palabras.Show()
		x += p.Width + HuecoEstado
	} else {
		r.b.palabras.Hide()
	}
	// Fuera por debajo de `sm`, como el `max-sm:hidden` del `<select>` de la web: la barra
	// no envuelve y en un teléfono el idioma se montaba sobre el zoom.
	if r.b.idioma.Texto != "" && t.Width >= ui.CorteSM {
		p := ui.MedidaEnlace(r.b.idioma)
		r.b.idioma.Move(fyne.NewPos(x, tema.Borde+(alto-p.Height)/2))
		r.b.idioma.Resize(p)
		r.b.idioma.Show()
	} else {
		r.b.idioma.Hide()
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
	// `w-10 text-center`: el botón mide su rótulo y va centrado en su hueco.
	p := ui.MedidaEnlace(r.b.porciento)
	r.b.porciento.Move(fyne.NewPos(der+(AnchoPorciento-p.Width)/2, tema.Borde+(alto-p.Height)/2))
	r.b.porciento.Resize(p)
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
	r.b.deslizador.Valor = float64(r.b.ctx.Zoom)
	r.b.deslizador.TextoValor = strconv.Itoa(r.b.ctx.Zoom) + " %"
	for _, o := range r.Objects() {
		o.Refresh()
	}
	// Y se coloca de nuevo: los rótulos miden su texto, y del 75 % al 100 % el
	// porcentaje se quedaba con la caja de dos cifras, en «10…».
	r.Layout(r.b.Size())
}
