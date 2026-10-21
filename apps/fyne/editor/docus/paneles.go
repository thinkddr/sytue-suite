// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los dos paneles del carril que viven del FOLIO: «Buscar» (`SearchPanel.tsx`) y
// «Esquema» (`OutlinePanel.tsx`). Los que viven del servicio —comentarios— son de la app.

// ---- buscar ----

// coincidencia es un trozo de un bloque que casa con la búsqueda.
type coincidencia struct{ b, desde, hasta int }

// Buscar resalta las coincidencias y devuelve cuántas hay. A diferencia de la web —que
// busca nodo de texto a nodo de texto y no ve «ho<b>la</b>»—, aquí se busca en el texto
// del bloque entero: una palabra con media negrita también se encuentra.
func (f *Folio) Buscar(q string, mayus, palabra bool) int {
	f.resaltes, f.activa = nil, 0
	f.buscado.q, f.buscado.mayus, f.buscado.palabra = q, mayus, palabra
	aguja := []rune(q)
	for i, b := range f.bloques {
		if !f.deTexto(i) {
			continue
		}
		for _, m := range editor.Casar([]rune(b.Texto()), aguja, mayus, palabra) {
			if len(f.resaltes) == editor.MaxCoincidencias {
				break
			}
			f.resaltes = append(f.resaltes, coincidencia{i, m[0], m[1]})
		}
	}
	f.pag.Refresh()
	return len(f.resaltes)
}

var _ editor.ResaltesDeBusqueda = (*Folio)(nil)

// OcultarResaltes deja de pintar las coincidencias al cerrar el panel, como la web.
func (f *Folio) OcultarResaltes() {
	f.ocultos = true
	f.pag.Refresh()
}

// MostrarResaltes las vuelve a pintar al abrirlo: busca otra vez lo tecleado —el
// documento ha podido cambiar— y se queda en la coincidencia que se estaba viendo.
func (f *Folio) MostrarResaltes() {
	f.ocultos = false
	activa := f.activa
	if n := f.Buscar(f.buscado.q, f.buscado.mayus, f.buscado.palabra); n > 0 {
		f.activa = min(activa, n-1)
	}
	f.pag.Refresh()
}

// IrACoincidencia activa la i-ésima (con vuelta) y la lleva a la vista.
func (f *Folio) IrACoincidencia(i int) {
	n := len(f.resaltes)
	if n == 0 {
		return
	}
	f.activa = (i%n + n) % n
	m := f.resaltes[f.activa]
	f.llevarAVista(punto{m.b, m.desde})
	f.pag.Refresh()
}

// llevarAVista desplaza la página para que q quede a la vista, hacia el centro.
func (f *Folio) llevarAVista(q punto) {
	_, y, alto, ok := f.caretEnMaqueta(q)
	if !ok {
		return
	}
	z := f.escala()
	_, oy := f.pag.origen()
	vista := f.scroll.Size().Height
	destino := oy + y*z + alto*z/2 - vista/2 // `block: 'center'`
	tope := max(f.pag.MinSize().Height-vista, 0)
	f.scroll.ScrollToOffset(fyne.NewPos(f.scroll.Offset.X, min(max(destino, 0), tope)))
}

// Reemplazar cambia la coincidencia activa por `por`, y la siguiente pasa a ser la
// activa: la primera que empieza en el cursor o después (el texto recién puesto no se
// vuelve a encontrar aunque contenga lo buscado).
func (f *Folio) Reemplazar(por, q string, mayus, palabra bool) {
	if !f.editable() || len(f.resaltes) == 0 {
		return
	}
	m := f.resaltes[f.activa]
	f.antesDeCambiar(false)
	f.bloques[m.b].Editar().Sustituir(m.desde, m.hasta, por)
	f.ed = nil
	f.cur = punto{m.b, m.desde + len([]rune(por))}
	f.anc = f.cur
	f.cambiado()
	f.Buscar(q, mayus, palabra)
	for i, r := range f.resaltes {
		if !(punto{r.b, r.desde}).antes(f.cur) {
			f.IrACoincidencia(i)
			return
		}
	}
	f.IrACoincidencia(0)
}

// ReemplazarTodo cambia todas, de la última a la primera para que las posiciones de las
// de delante sigan valiendo. Es UN paso de deshacer.
func (f *Folio) ReemplazarTodo(por, q string, mayus, palabra bool) {
	if !f.editable() || len(f.resaltes) == 0 {
		return
	}
	f.antesDeCambiar(false)
	for i := len(f.resaltes) - 1; i >= 0; i-- {
		m := f.resaltes[i]
		f.bloques[m.b].Editar().Sustituir(m.desde, m.hasta, por)
	}
	f.ed = nil
	f.cambiado()
	f.Buscar(q, mayus, palabra)
}

// Activa es la coincidencia que se está viendo, y Coincidencias cuántas hay.
func (f *Folio) Activa() int { return f.activa }

// Coincidencias es cuántas hay resaltadas.
func (f *Folio) Coincidencias() int { return len(f.resaltes) }

// Editable dice si se puede reemplazar.
func (f *Folio) Editable() bool { return f.editable() }

// PanelBuscar es el cuerpo de «Buscar» (el de `editor`, sobre este folio). Se construye
// UNA vez por folio: lo tecleado se conserva al cerrar y abrir el panel. La pausa de lo
// tecleado va con el reloj del folio, que las pruebas paran.
func (f *Folio) PanelBuscar(t func(string, i18n.Vars) string) fyne.CanvasObject {
	if f.buscador == nil {
		f.buscador = editor.NuevoBuscador(f, t)
		f.buscador.Programar = func(d time.Duration, fn func()) { f.Programar(d, fn) }
	}
	return f.buscador.Objeto()
}

// botonPequeno y alDerecha viven en `editor` desde que Calcus busca con el mismo panel.
type (
	botonPequeno = editor.BotonPequeno
	alDerecha    = editor.AlDerecha
)

// nuevoBotonAccion es ACTION_CLASS de Bibliografia.tsx: `h-6 w-full` con el texto en
// medio (el botón pequeño lo centra siempre, como un `<button>`).
func nuevoBotonAccion(texto string, alPulsar func()) *botonPequeno {
	b := editor.NuevoBotonPequeno(texto, alPulsar)
	b.Alto = tema.Espacio(6)
	return b
}

// MargenCuerpo es el `p-3` de los cuerpos de panel.
var MargenCuerpo = tema.Espacio(3)

func trim(s string) string {
	rs := []rune(s)
	i, j := 0, len(rs)
	for i < j && unicode.IsSpace(rs[i]) {
		i++
	}
	for j > i && unicode.IsSpace(rs[j-1]) {
		j--
	}
	return string(rs[i:j])
}

// ---- esquema ----

// PanelEsquema es el cuerpo de «Esquema»: los títulos h1–h3 del documento, sangrados
// por nivel (`pl-2`, `pl-4`, `pl-6`); pulsar uno lo lleva arriba de la vista.
func (f *Folio) PanelEsquema(t func(string, i18n.Vars) string) fyne.CanvasObject {
	tr := func(k string) string {
		if t == nil {
			return k
		}
		return t(k, nil)
	}
	var filas []fyne.CanvasObject
	for i, b := range f.bloques {
		if b.Tipo != rico.Titulo || b.Nivel > 3 {
			continue
		}
		texto := trim(b.Texto())
		if texto == "" {
			texto = tr("outline.untitled")
		}
		filas = append(filas, nuevaFilaEsquema(texto, b.Nivel, func() {
			f.llevarArriba(i)
		}))
	}
	if len(filas) == 0 {
		// `<p class="text-text-muted p-3 text-sm">`.
		p := ui.NuevoParrafo(tr("outline.empty"), tema.TextoSM, ui.Normal, "text-muted",
			tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		return container.New(layout.NewCustomPaddedLayout(MargenCuerpo, MargenCuerpo, MargenCuerpo, MargenCuerpo), p)
	}
	// `px-1 py-3` y `gap-px` entre filas.
	lista := container.New(ui.Columna{Hueco: 1}, filas...)
	return container.NewVScroll(container.New(
		layout.NewCustomPaddedLayout(MargenCuerpo, MargenCuerpo, tema.Espacio(1), tema.Espacio(1)), lista))
}

// llevarArriba desplaza para que el bloque i quede arriba (`block: 'start'`).
func (f *Folio) llevarArriba(i int) {
	if i < 0 || i >= len(f.cajas) {
		return
	}
	z := f.escala()
	_, oy := f.pag.origen()
	tope := max(f.pag.MinSize().Height-f.scroll.Size().Height, 0)
	f.scroll.ScrollToOffset(fyne.NewPos(f.scroll.Offset.X, min(max(oy+f.cajas[i].y*z, 0), tope)))
}

// filaEsquema es ROW_BTN: `block w-full truncate rounded-sm py-1 pr-2 text-left text-sm
// text-text hover:bg-bg-subtle`.
type filaEsquema struct {
	widget.BaseWidget
	Texto            string
	nivel            int
	alPulsar         func()
	encima, enfocado bool
}

func nuevaFilaEsquema(texto string, nivel int, alPulsar func()) *filaEsquema {
	f := &filaEsquema{Texto: texto, nivel: nivel, alPulsar: alPulsar}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaEsquema) Tapped(*fyne.PointEvent) {
	if f.alPulsar != nil {
		f.alPulsar()
	}
}
func (f *filaEsquema) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaEsquema) MouseMoved(*desktop.MouseEvent) {}
func (f *filaEsquema) MouseOut()                      { f.encima = false; f.Refresh() }
func (f *filaEsquema) FocusGained()                   { f.enfocado = true; f.Refresh() }
func (f *filaEsquema) FocusLost()                     { f.enfocado = false; f.Refresh() }
func (f *filaEsquema) TypedRune(r rune) {
	if r == ' ' {
		f.Tapped(nil)
	}
}

func (f *filaEsquema) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		f.Tapped(nil)
	}
}
func (f *filaEsquema) AccessibilityLabel() string             { return f.Texto }
func (f *filaEsquema) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// sangriaEsquema es el `pl-2`/`pl-4`/`pl-6` de cada nivel.
func sangriaEsquema(nivel int) float32 { return tema.Espacio(float32(2 * max(nivel, 1))) }

func (f *filaEsquema) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(0, tema.TextoSM.Linea+2*tema.Espacio(1))
}

func (f *filaEsquema) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaEsquemaRenderer{f: f, caja: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM), aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v)}
	r.texto = ui.Texto(f.Texto, tema.TextoSM, ui.Normal, tema.Color("text", v))
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.texto)...)
	return r
}

type filaEsquemaRenderer struct {
	f     *filaEsquema
	caja  *ui.Caja
	aro   *ui.Aro
	texto *canvas.Text
	objs  []fyne.CanvasObject
}

func (r *filaEsquemaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaEsquemaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaEsquemaRenderer) Destroy()                     {}
func (r *filaEsquemaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x := sangriaEsquema(r.f.nivel)
	ui.Truncar(r.texto, r.f.Texto, tema.TextoSM, ui.Normal, max(t.Width-x-tema.Espacio(2), 0))
	ui.ColocarTexto(r.texto, tema.TextoSM, x, tema.Espacio(1))
}

func (r *filaEsquemaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.encima {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.enfocado)
	if s := r.f.Size(); s.Width > 0 { // sin tamaño todavía: la caja con borde saldría negativa
		r.Layout(s)
	}
}

// panelesDe es el `Paneles` del contexto de Docus: buscar, esquema y revisión los da el
// folio.
func panelesDe(f *Folio, t func(string, i18n.Vars) string, loc i18n.Locale) func(editor.Panel) fyne.CanvasObject {
	return func(p editor.Panel) fyne.CanvasObject {
		switch p {
		case editor.PanelBuscar:
			return f.PanelBuscar(t)
		case editor.PanelEsquema:
			return f.PanelEsquema(t)
		case editor.PanelRevision:
			return f.PanelRevision(t, loc)
		case editor.SinPanel, editor.PanelComentarios, editor.PanelIA:
		}
		return nil
	}
}
