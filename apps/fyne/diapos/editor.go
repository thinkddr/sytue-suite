// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"cmp"
	"errors"
	"image/color"
	"slices"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El lienzo de Diapos en el cromo del editor
//
// Editor es lo que SlidesCanvas.tsx pinta dentro de EditorApp: el carril de miniaturas
// con sus tres botones, la mini-barra sobre el escenario, el escenario y el panel de
// notas. Lo de alrededor —cabecera, cinta, carril de paneles y barra de estado— es el
// cromo compartido con Docus y Calcus (`apps/fyne/editor`), y este tipo es su
// `editor.Lienzo`.
//
// Todo cambio al DOCUMENTO pasa por `cambiar`, que es el único sitio que comprueba el
// solo lectura, marca el guardado como sucio y avisa a quien guarda. Es el mismo
// cortafuegos que `editor.Contexto.Mandar` pone a la cinta, aplicado a lo que la cinta
// no ve: los botones del carril, la mini-barra y las teclas.

// Medidas de SlidesCanvas.tsx, con el rem de 14.
var (
	// AltoMiniBarra es el `h-9` de la mini-barra, con su `border-b` dentro.
	AltoMiniBarra = tema.Espacio(9)
	// HuecoMiniBarra es su `gap-1`.
	HuecoMiniBarra = tema.Espacio(1)
	// AltoPieCarril es el pie del carril: `border-t p-1` y un botón de la cinta.
	AltoPieCarril = tema.Borde + 2*tema.Espacio(1) + editor.LadoBotonCinta
	// RellenoArea es el `p-6` del área del escenario.
	RellenoArea = tema.Espacio(6)
	// AltoNotas es el panel de notas: `border-t`, la cabecera `pt-1` en una línea de
	// `text-xs` y el `<textarea>` de `h-20`.
	AltoNotas = tema.Borde + tema.Espacio(1) + tema.TextoXS.Linea + tema.Espacio(20)
	// AnchoSelectorTema y AnchoSelectorTransicion son los `width` de sus RibbonSelect. El del
	// tema mide 172 y no 128: con 128, «Papel (predeterminado)», que es lo que enseña casi
	// siempre, se cortaba en «Papel (predeterm» (medido el 2026-10-16).
	AnchoSelectorTema       float32 = 172
	AnchoSelectorTransicion float32 = 118
	// AnchoSelectorConector es el `width={104}` del tipo de conector.
	AnchoSelectorConector float32 = 104
	// Los de la selección: tipo de gráfico, disposición del diagrama y enlace de acción.
	anchoSelectorGrafico float32 = 92
	anchoSelectorSmart   float32 = 104
	anchoSelectorAccion  float32 = 148
)

var formasUI = editor.FormasUI

var disposicionesSmartUI = editor.DisposicionesSmartUI

// paletaFondo es SLIDE_BG_PALETTE: 'transparent' (el del tema) y los sólidos de los
// temas, sin un hex nuevo.
var paletaFondo = []string{
	"transparent", "#FFFFFF", "#0A0A0A", "#475569", "#FED7AA", "#BBF7D0", "#BFDBFE",
}

// temasSelector son THEME_OPTIONS: Papel primero y con valor vacío.
var temasSelector = []struct{ ID, Clave string }{
	{"", "slides.theme.paper"},
	{"carbon", "slides.theme.charcoal"},
	{"pizarra", "slides.theme.slate"},
	{"arena", "slides.theme.sand"},
	{"menta", "slides.theme.mint"},
	{"cielo", "slides.theme.sky"},
}

// transiciones son TRANSITION_OPTIONS.
var transiciones = []struct{ ID, Clave string }{
	{"none", "slides.trans.none"},
	{"fade", "slides.trans.fade"},
	{"slide", "slides.trans.slide"},
	{"morph", "slides.trans.morph"},
}

// Editor es el lienzo de Diapos para el cromo del editor.
type Editor struct {
	ctx *editor.Contexto
	m   *Modelo

	activa int
	// abajo es `bottom`: el panel bajo el escenario —notas, animaciones, datos del
	// gráfico o elementos del diagrama— o "" sin panel. Notas por defecto, como en la web.
	abajo string
	// sel es la caja primaria elegida (la de los tiradores) y extra las añadidas con
	// Mayús; ver `seleccion.go`.
	sel   string
	extra []string
	capa  *capaEdicion
	// panel es el de debajo que no son las notas, y clavePanel su forma: se rehace solo
	// cuando cambia (ver `panelAbajo`). catAnim es la categoría elegida para añadir.
	panel      fyne.CanvasObject
	clavePanel string
	catAnim    Categoria
	// escribiendo es la caja de texto en edición (ver `escribir.go`), o nil.
	escribiendo *escritura
	// colorTexto, resalte e interlinea son lo último aplicado desde «Fuente» y
	// «Párrafo»: lo que enseñan la barrita de las muestras y el selector, porque
	// `FormatState` no los expone (`useState` de InicioSlides.tsx).
	colorTexto, resalte, interlinea string
	// patron dice si se está editando el patrón (`editingMaster`): las cajas que se
	// eligen, mueven, escriben, insertan y borran son las suyas (ver `objetivo`).
	patron bool
	// selCon es el conector elegido (`selConn`): o cajas o un conector, nunca los dos.
	selCon string
	// aviso es la banda de lo que este lienzo no dibuja todavía; vacía si lo pinta todo.
	aviso string

	// Idioma es el de la sesión: da el separador decimal de los números de la cinta
	// (`formatNumber(locale, …)` en la web).
	Idioma i18n.Locale

	// AlCambiar se llama tras cada cambio al documento. Es de quien guarda: aquí no se
	// sabe si el documento vive en el servicio o en un fichero.
	AlCambiar func()

	// ElegirImagen abre el selector de imágenes del sistema y devuelve los bytes del
	// fichero elegido y si era JPEG. Lo pone quien monta la ventana; sin él, «Imagen»
	// va apagado.
	ElegirImagen func(listo func(datos []byte, jpeg bool))
	// ElegirMedio es el selector de audio y vídeo de «Subir fichero…»: el nombre, los
	// bytes y si pasaba de MaxMedioIncrustado (entonces sin leerlo). Sin él, el botón va
	// apagado.
	ElegirMedio func(listo func(nombre string, datos []byte, grande bool))

	// Cast, si no es nil, ofrece «Presentar con Cast» (ver cast.go).
	Cast *Cast

	// Programar, si no es nil, sustituye al reloj del folio de la caja que se escribe
	// (el que repinta la cinta con el formato bajo el cursor): las pruebas lo ponen para
	// no depender del tiempo.
	Programar func(time.Duration, func())

	raiz      *fyne.Container
	carril    *carrilDiapos
	zona      *fyne.Container
	miniBarra *fyne.Container
	// deslizBarra es el `overflow-x-auto` de la mini-barra: en un teléfono sus ~483 px
	// de controles no caben y se desliza de lado, sin encoger ninguno.
	deslizBarra *container.Scroll
	escenario   *Lienzo
	sombra      *ui.Sombra
	desliz      *container.Scroll
	notas       *panelNotas
	avisos      *ui.PilaAvisos
}

var _ editor.Lienzo = (*Editor)(nil)

// NuevoEditor monta el lienzo de Diapos con una presentación vacía; el contenido de
// verdad llega por Cargar.
func NuevoEditor(ctx *editor.Contexto) *Editor {
	e := &Editor{
		ctx: ctx, abajo: abajoNotas, avisos: ui.NuevaPilaAvisos(), Idioma: "es",
		colorTexto: editor.PaletaContenido[0], resalte: editor.PaletaResaltado[0], interlinea: "1.25",
	}
	e.m = ModeloVacio(e.t)
	e.escenario = NuevoLienzo(e.m, 0)
	e.escenario.Rotulo = e.t
	e.sombra = ui.NuevaSombra(tema.SombraMD, 0)
	area := &areaDiapo{e: e}
	e.capa = nuevaCapaEdicion(e)
	e.desliz = container.NewScroll(container.New(area, e.sombra.Objeto(), e.escenario, e.capa))
	area.desliz = e.desliz
	e.carril = nuevoCarrilDiapos(e)
	e.notas = nuevoPanelNotas(e)
	e.miniBarra = container.New(&filaMiniBarra{})
	e.deslizBarra = container.NewHScroll(e.miniBarra)
	e.zona = container.New(&zonaDiapo{e: e})
	e.raiz = container.New(&disposicionDiapos{e: e}, e.carril, e.zona)
	e.recomponer()
	return e
}

// t traduce una clave sin variables.
func (e *Editor) t(clave string) string { return e.ctx.Traducir(clave) }

// objetivo es el índice de las cajas que se editan: el de la diapositiva activa o, con
// «Editar patrón», IndicePatron.
func (e *Editor) objetivo() int {
	if e.patron {
		return IndicePatron
	}
	return e.activa
}

// EditarPatron es `enterMaster`: el escenario pasa a editar las cajas del patrón, sin
// selección y con el panel de abajo cerrado (notas y animaciones no son del patrón).
func (e *Editor) EditarPatron() {
	if e.patron {
		return
	}
	e.TerminarEscritura()
	e.patron, e.abajo = true, ""
	e.sel, e.extra, e.selCon = "", nil, ""
	e.recomponer()
}

// CerrarPatron es `exitMaster`: vuelve a editar la diapositiva activa.
func (e *Editor) CerrarPatron() {
	if !e.patron {
		return
	}
	e.TerminarEscritura()
	e.patron = false
	e.sel, e.extra, e.selCon = "", nil, ""
	e.recomponer()
}

// EditandoPatron dice si el escenario edita el patrón.
func (e *Editor) EditandoPatron() bool { return e.patron }

// NuevoNumeroPagina es `addPageNumberBox`: una caja de texto con el campo {n} abajo a
// la derecha, que es el número de página clásico del patrón.
func (e *Editor) NuevoNumeroPagina() {
	e.empujarCaja(nuevaCaja(84, 89, 12, 8, `<p style="text-align:right">{n}</p>`))
}

// Avisos es la pila de avisos del lienzo. Va encima de TODA la ventana, que es donde la
// web pone sus toasts; la coloca quien monta la ventana.
func (e *Editor) Avisos() *ui.PilaAvisos { return e.avisos }

// Modelo es la presentación que se está enseñando.
func (e *Editor) Modelo() *Modelo { return e.m }

// Activa es el índice de la diapositiva seleccionada.
func (e *Editor) Activa() int { return e.activa }

// ---- editor.Lienzo ----

// Objeto es lo que el cromo aloja en el centro.
func (e *Editor) Objeto() fyne.CanvasObject { return e.raiz }

// Cargar recibe el valor del registro `doc:slides` y dice si lo entiende ENTERO.
//
// Tres casos, y el primero es el que la web no tiene: un modelo que no se reescribe
// igual (`Fiel` dice que no) se enseña y NO se deja tocar. Los otros dos son los de la
// web: vacío = presentación nueva; ilegible o sin diapositivas = el modelo vacío y el
// aviso de `slides.msg.readFailed`. Ilegible, además, en solo lectura: la web lo
// sobrescribiría con el primer cambio, y un documento que no sabemos leer no se pisa.
func (e *Editor) Cargar(contenido string) bool {
	entendido := true
	e.aviso = ""
	switch m, err := Leer([]byte(contenido)); {
	case contenido == "":
		e.m = ModeloVacio(e.t)
	case err != nil:
		e.m, entendido = ModeloVacio(e.t), false
		e.avisar("slides.msg.readFailed", "slides.msg.readFailed.detail")
	case len(m.Diapos) == 0:
		e.m = ModeloVacio(e.t)
		e.avisar("slides.msg.readFailed", "slides.msg.readFailed.detail")
	default:
		e.m, entendido = m, m.Fiel([]byte(contenido))
		if faltan := m.NoPintado(); len(faltan) > 0 {
			e.aviso = Resumen(faltan, e.t)
		}
	}
	e.activa, e.patron, e.selCon = 0, false, ""
	e.sel, e.extra, e.escribiendo, e.escenario.Oculta = "", nil, nil, ""
	e.escenario.M = e.m
	e.recomponer()
	return entendido
}

// Serializar es el modelo para guardar.
func (e *Editor) Serializar() string {
	s, err := e.m.Serializar()
	if err != nil {
		return "" // el modelo es JSON puro: no puede pasar
	}
	return s
}

// Estadisticas: en Diapos la unidad es la DIAPOSITIVA, y no hay palabras.
func (e *Editor) Estadisticas() (unidad, total, palabras int) {
	return e.activa + 1, len(e.m.Diapos), 0
}

// Mandar es el `exec` de la cinta sobre la caja que se escribe: lo hace su folio, sobre
// lo elegido o, sin selección, para lo próximo que se teclee. Sin caja en edición no hay
// texto al que darle formato, como en la web.
func (e *Editor) Mandar(c editor.Comando, valor string) {
	if w := e.escribiendo; w != nil {
		w.folio.Mandar(c, valor)
	}
}

// Formato es el de lo elegido en la caja que se escribe; sin ella, el cero.
func (e *Editor) Formato() editor.Formato {
	if w := e.escribiendo; w != nil {
		return w.folio.Formato()
	}
	return editor.Formato{}
}

// Refrescar repinta lo que depende del contexto (el zoom, el solo lectura). Lo cablea
// quien monta el marco detrás del `Refrescar` del contexto.
func (e *Editor) Refrescar() { e.recomponer() }

// ---- cambios al documento ----

// cambiar aplica un cambio al documento: comprueba el solo lectura, lo aplica, marca el
// guardado como sucio y avisa a quien guarda. `repintar` es false solo para lo que ya
// está en pantalla —las notas mientras se teclean—.
func (e *Editor) cambiar(f func(), repintar bool) {
	if e.ctx.SoloLectura {
		return
	}
	f()
	antes := e.ctx.Guardado
	e.ctx.Guardado = editor.Sucio
	if repintar {
		e.recomponer()
	}
	// El cromo se rehace entero al refrescar —la cinta incluida—, así que no se le
	// pide en cada letra de las notas: solo cuando algo suyo cambia (el chip pasa a
	// «sin guardar», o la barra de estado cuenta otras diapositivas).
	if (repintar || antes != editor.Sucio) && e.ctx.Refrescar != nil {
		e.ctx.Refrescar()
	}
	if e.AlCambiar != nil {
		e.AlCambiar()
	}
}

// Elegir cambia de diapositiva. No es un cambio al documento.
func (e *Editor) Elegir(i int) {
	if i < 0 || i >= len(e.m.Diapos) || i == e.activa {
		return
	}
	e.TerminarEscritura()
	e.activa = i
	e.sel, e.extra, e.selCon = "", nil, "" // la selección es de una diapositiva
	e.recomponer()
	if e.ctx.Refrescar != nil {
		e.ctx.Refrescar() // la barra de estado: «Diapositiva 3 de 9»
	}
}

// NuevaDiapositiva es `addSlide`: detrás de la activa, con título y cuerpo.
func (e *Editor) NuevaDiapositiva() {
	e.cambiar(func() { e.activa = e.m.InsertarTras(e.activa, NuevaDiapo(DispTituloCuerpo, e.t)) }, true)
}

// InsertarBloque es `insertarBloque`: la diapositiva de un bloque reutilizable, detrás de
// la activa, que pasa a ser ella.
func (e *Editor) InsertarBloque(crudo string) {
	d, ok := DiapoDeBloque(crudo)
	if !ok {
		return
	}
	e.cambiar(func() { e.activa = e.m.InsertarTras(e.activa, d) }, true)
}

// Duplicar es `duplicateSlide`; si la copia no cabe en un guardado, lo dice y no hace
// nada.
func (e *Editor) Duplicar() {
	e.cambiar(func() {
		pos, err := e.m.Duplicar(e.activa)
		if errors.Is(err, ErrDemasiadoGrande) {
			e.avisar("slides.msg.dupeFailed", "slides.msg.dupeFailed.detail")
			return
		}
		e.activa = pos
	}, true)
}

// Eliminar es `deleteSlide`: pregunta antes y siempre deja una.
func (e *Editor) Eliminar() {
	if e.ctx.SoloLectura || len(e.m.Diapos) <= 1 {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(e.raiz)
	if c == nil {
		return
	}
	borrar := e.t("slides.delete")
	cuerpo := ui.NuevoParrafo(e.ctx.Traducir("dialog.slide.delete", i18n.Vars{"n": e.activa + 1}),
		tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam * 1.625}, ui.Normal, "text", e.variante())
	var d *ui.Dialogo
	d = ui.Confirmacion(borrar, e.ctx.Traducir("ui.cancel"), borrar, true, cuerpo,
		func() { d.Cerrar() },
		func() {
			d.Cerrar()
			e.cambiar(func() {
				if e.m.Eliminar(e.activa) {
					e.activa = min(e.activa, len(e.m.Diapos)-1)
				}
			}, true)
		})
	d.Mostrar(c)
}

// Mover es `moveSlideTo`: la activa sigue siendo la misma diapositiva.
func (e *Editor) Mover(desde, hasta int) {
	e.cambiar(func() {
		if e.m.Mover(desde, hasta) && desde == e.activa {
			e.activa = hasta
		}
	}, true)
}

// AplicarDisposicion es `applyLayout` sobre la activa.
func (e *Editor) AplicarDisposicion(disp string) {
	if e.patron {
		return // la disposición es de las diapositivas, no del patrón
	}
	e.cambiar(func() { e.m.AplicarDisposicion(e.activa, disp, e.t) }, true)
}

// PonerTamano es `setSlideSize`.
func (e *Editor) PonerTamano(t *Tamano) { e.cambiar(func() { e.m.PonerTamano(t) }, true) }

// Los paneles de debajo del escenario (`bottom`).
const (
	abajoNotas   = "notes"
	abajoAnim    = "anim"
	abajoGrafico = "chart"
	abajoSmart   = "smartart"
)

// alternarAbajo abre ese panel o, si ya estaba abierto, lo cierra: los botones de la
// mini-barra son conmutadores.
func (e *Editor) alternarAbajo(p string) {
	if e.abajo == p {
		e.abajo = ""
	} else {
		e.abajo = p
	}
	e.recomponer()
}

// AlternarNotas es el conmutador de notas de la mini-barra.
func (e *Editor) AlternarNotas() {
	e.alternarAbajo(abajoNotas)
	e.recomponer()
}

// NuevaCajaTexto es `addTextBox`.
func (e *Editor) NuevaCajaTexto() {
	e.empujarCaja(nuevaCaja(30, 40, 40, 14, "<p>"+e.t("slides.textBox.sample")+"</p>"))
}

// NuevoGrafico es `addChartBox`: barras con los datos de arranque de la web, elegido y
// con el panel de sus datos abierto.
func (e *Editor) NuevoGrafico() {
	c := nuevaCaja(20, 18, 60, 64, "")
	c.Tipo = "chart"
	c.Grafico = &Grafico{Tipo: "bar", Rejilla: [][]string{
		{"", "Serie 1"}, {"Ene", "10"}, {"Feb", "15"}, {"Mar", "7"}, {"Abr", "12"},
	}}
	e.abajo = abajoGrafico
	e.empujarCaja(c)
}

func (e *Editor) avisar(titulo, detalle string) {
	e.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, e.t(titulo), e.t(detalle)))
}

func (e *Editor) variante() tema.Variante {
	return tema.De(fyne.CurrentApp().Settings().ThemeVariant())
}

// ---- composición ----

// recomponer rehace lo que depende del modelo y de la diapositiva activa. Son unas
// decenas de objetos; el escenario y el área de notas se conservan (el escenario
// recompone sus cajas él solo, y el área de notas no puede perder el cursor).
func (e *Editor) recomponer() {
	e.activa = min(max(e.activa, 0), max(len(e.m.Diapos)-1, 0))
	// Lo elegido que ya no existe —se borró, se deshizo, cambió la disposición— se suelta.
	if e.sel != "" && e.m.caja(e.objetivo(), e.sel) == nil {
		e.sel, e.extra = "", nil
	}
	if e.selCon != "" && e.m.conector(e.activa, e.selCon) == nil {
		e.selCon = ""
	}
	e.escenario.M, e.escenario.Indice, e.escenario.Variante = e.m, e.activa, e.variante()
	e.escenario.Patron, e.escenario.Conector = e.patron, e.selCon
	e.carril.rehacer()
	e.miniBarra.Objects = e.controlesMiniBarra()
	e.notas.sincronizar()

	objs := []fyne.CanvasObject{e.miniBarraConFondo()}
	if e.patron {
		objs = append(objs, e.bandaPatron())
	}
	if e.aviso != "" {
		objs = append(objs, cartel(e.aviso, e.variante()))
	}
	objs = append(objs, container.NewStack(canvas.NewRectangle(colorLienzo(e.variante())), e.desliz))
	if p := e.panelAbajo(); p != nil {
		objs = append(objs, p)
	}
	e.zona.Objects = objs
	e.zona.Refresh()
	e.miniBarra.Refresh()
	e.escenario.Refresh()
	e.capa.Refresh()
	e.desliz.Refresh() // el zoom cambia lo que mide el contenido desplazable
}

func (e *Editor) miniBarraConFondo() fyne.CanvasObject {
	v := e.variante()
	return container.New(&bandaInferior{}, canvas.NewRectangle(tema.Color("surface", v)),
		canvas.NewRectangle(tema.Color("border", v)), e.deslizBarra)
}

// controlesMiniBarra es la mini-barra de §7.2 en el orden de la web; lo que depende de
// la caja o del conector elegido va detrás (`controlesSeleccion`, `controlesConector`).
// Editando el patrón cambia: ver `EditarPatron`.
func (e *Editor) controlesMiniBarra() []fyne.CanvasObject {
	ro := e.ctx.SoloLectura
	boton := func(i iconos.Ed, clave string, activo, inerte bool, f func()) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, e.t(clave), f)
		b.Activo, b.Inerte = activo, inerte
		return b
	}
	d := e.m.Diapos[e.activa]

	temas := make([]editor.Opcion, 0, len(temasSelector)+len(e.m.Temas))
	for _, o := range temasSelector {
		temas = append(temas, editor.Opcion{Valor: o.ID, Rotulo: e.t(o.Clave)})
	}
	// Los temas propios de la presentación, tras los de serie (se crean y se editan en la web).
	for _, p := range e.m.Temas {
		if _, ok := e.m.temaPropio(p.ID); ok {
			temas = append(temas, editor.Opcion{Valor: p.ID, Rotulo: cmp.Or(p.Nombre, p.ID)})
		}
	}
	selTema := editor.NuevoSelectorCinta(e.t("slides.theme"), AnchoSelectorTema, temas, e.m.Tema,
		func(v string) { e.cambiar(func() { e.m.PonerTema(v) }, true) })
	selTema.Inerte = ro

	objs := []fyne.CanvasObject{
		boton(iconos.EdCajaTexto, "slides.textBox", false, ro, e.NuevaCajaTexto),
		boton(iconos.EdGrafico, "data.chart.insert", false, ro, e.NuevoGrafico),
	}
	if e.patron {
		// En el patrón: el número de página, y ni notas ni animaciones, ni fondo,
		// transición u «ocultar patrón», que son de cada diapositiva.
		objs = append(objs, boton(iconos.EdNumeroPagina, "slides.insertNumber", false, ro, e.NuevoNumeroPagina),
			editor.Divisor(), selTema)
		return append(objs, e.controlesSeleccion()...)
	}
	objs = append(objs,
		editor.Divisor(),
		boton(iconos.EdComentario, "slides.notes", e.abajo == abajoNotas, false, e.AlternarNotas),
		boton(iconos.EdFlecha, "slides.animations.hint", e.abajo == abajoAnim, false, func() { e.alternarAbajo(abajoAnim) }),
		editor.Divisor(),
		selTema,
	)

	fondo := d.Fondo
	if fondo == "" {
		fondo = "transparent"
	}
	muestra := editor.NuevaMuestraCinta(e.ctx, iconos.EdImagen, e.t("slides.background"), fondo, paletaFondo,
		func(c string) { e.cambiar(func() { e.m.PonerFondo(e.activa, c) }, true) })
	muestra.Inerte = ro

	opciones := make([]editor.Opcion, len(transiciones))
	for i, o := range transiciones {
		opciones[i] = editor.Opcion{Valor: o.ID, Rotulo: e.t(o.Clave)}
	}
	tr := d.Transicion
	if tr == "" {
		tr = "none"
	}
	selTr := editor.NuevoSelectorCinta(e.t("slides.transition"), AnchoSelectorTransicion, opciones, tr,
		func(v string) { e.cambiar(func() { e.m.PonerTransicion(e.activa, v) }, true) })
	selTr.Inerte = ro
	objs = append(objs, muestra, selTr)

	if e.m.Patron != nil { // `model.master &&`: también con el patrón vacío
		oculto := d.SinPatron != nil && *d.SinPatron
		objs = append(objs, boton(iconos.EdPatron, "slides.hideMaster", oculto, ro,
			func() { e.cambiar(func() { e.m.AlternarPatron(e.activa) }, true) }))
	}
	return append(append(objs, e.controlesSeleccion()...), e.controlesConector()...)
}

// QuitarConector es `deleteConnector` del conector elegido.
func (e *Editor) QuitarConector() {
	id := e.selCon
	if id == "" {
		return
	}
	e.cambiar(func() {
		e.m.QuitarConector(e.activa, id)
		e.selCon = ""
	}, true)
}

// controlesConector son los de la mini-barra con un conector elegido: tipo de trazado,
// las dos puntas, el color y eliminar.
func (e *Editor) controlesConector() []fyne.CanvasObject {
	c := e.m.conector(e.activa, e.selCon)
	if c == nil || e.patron {
		return nil
	}
	ro, id := e.ctx.SoloLectura, c.ID
	aplicar := func(f func()) { e.cambiar(f, true) }
	ops := make([]editor.Opcion, len(conectoresUI))
	for i, o := range conectoresUI {
		ops[i] = editor.Opcion{Valor: o.ID, Rotulo: e.t(o.Clave)}
	}
	tipo := editor.NuevoSelectorCinta(e.t("connector.type"), AnchoSelectorConector, ops, c.Trazado,
		func(v string) { aplicar(func() { e.m.PonerTrazadoConector(e.activa, id, v) }) })
	tipo.Inerte = ro
	boton := func(i iconos.Ed, clave string, activo bool, f func()) *editor.BotonCinta {
		b := editor.NuevoBotonCinta(i, e.t(clave), f)
		b.Activo, b.Inerte = activo, ro
		return b
	}
	trazo := c.Trazo
	if trazo == "" {
		trazo = hexDe(tinta)
	}
	muestra := editor.NuevaMuestraCinta(e.ctx, iconos.EdLinea, e.t("connector.color"), trazo, editor.PaletaContenido,
		func(v string) { aplicar(func() { e.m.PonerColorConector(e.activa, id, v) }) })
	muestra.Inerte = ro
	return []fyne.CanvasObject{
		editor.Divisor(), tipo,
		boton(iconos.EdConRecto, "connector.arrowStart", c.PuntaInicio != nil && *c.PuntaInicio,
			func() { aplicar(func() { e.m.AlternarPunta(e.activa, id, true) }) }),
		boton(iconos.EdFlecha, "connector.arrowEnd", c.PuntaFin != nil && *c.PuntaFin,
			func() { aplicar(func() { e.m.AlternarPunta(e.activa, id, false) }) }),
		muestra,
		boton(iconos.EdPapelera, "connector.delete", false, e.QuitarConector),
	}
}

// paletaRellenoForma es SHAPE_FILL_PALETTE: la de contenido con «sin relleno» en vez
// del blanco puro.
var paletaRellenoForma = func() []string {
	p := slices.Clone(editor.PaletaContenido)
	for i, c := range p {
		if c == "#FFFFFF" {
			p[i] = "transparent"
		}
	}
	return p
}()

// controlesSeleccion son los de la mini-barra que dependen de la caja elegida: las
// muestras de una forma, el tipo y los datos de un gráfico, la disposición y los
// elementos de un diagrama, y el enlace de acción de cualquier caja.
func (e *Editor) controlesSeleccion() []fyne.CanvasObject {
	p := e.Primaria()
	if p == nil {
		return nil
	}
	ro, id := e.ctx.SoloLectura, p.ID
	aplicar := func(f func()) { e.cambiar(f, true) }
	selector := func(clave string, ancho float32, ops []editor.Opcion, valor string, f func(string)) *editor.SelectorCinta {
		s := editor.NuevoSelectorCinta(e.t(clave), ancho, ops, valor, f)
		s.Inerte = ro
		return s
	}
	var objs []fyne.CanvasObject
	switch p.Tipo {
	case "shape":
		objs = append(objs, editor.Divisor())
		if p.Forma != "line" && p.Forma != "arrow" {
			relleno := p.Relleno
			if relleno == "" {
				relleno = rellenoForma
			}
			m := editor.NuevaMuestraCinta(e.ctx, iconos.EdRect, e.t("shape.fill"), relleno, paletaRellenoForma,
				func(c string) { aplicar(func() { e.m.PonerRelleno(e.objetivo(), id, c) }) })
			m.Inerte = ro
			objs = append(objs, m)
		}
		trazo := p.Trazo
		if trazo == "" {
			trazo = trazoForma
		}
		m := editor.NuevaMuestraCinta(e.ctx, iconos.EdLinea, e.t("shape.outline"), trazo, nil,
			func(c string) { aplicar(func() { e.m.PonerTrazo(e.objetivo(), id, c) }) })
		m.Inerte = ro
		objs = append(objs, m)
	case "chart":
		if p.Grafico != nil {
			ops := []editor.Opcion{
				{Valor: "bar", Rotulo: e.t("chart.kind.bar")},
				{Valor: "line", Rotulo: e.t("chart.kind.line")},
				{Valor: "pie", Rotulo: e.t("chart.kind.pie")},
			}
			datos := editor.NuevoBotonCinta(iconos.EdTabla, e.t("chart.editData"), func() { e.alternarAbajo(abajoGrafico) })
			datos.Activo = e.abajo == abajoGrafico
			objs = append(objs, editor.Divisor(),
				selector("chart.type", anchoSelectorGrafico, ops, p.Grafico.Tipo,
					func(v string) { aplicar(func() { e.m.PonerTipoGrafico(e.objetivo(), id, v) }) }),
				datos)
		}
	case "smartart":
		if p.Diagrama != nil {
			ops := make([]editor.Opcion, 0, len(disposicionesSmartUI))
			for _, d := range disposicionesSmartUI {
				ops = append(ops, editor.Opcion{Valor: d.ID, Rotulo: e.t(d.Clave)})
			}
			elems := editor.NuevoBotonCinta(iconos.EdEsquema, e.t("smartart.editItems"), func() { e.alternarAbajo(abajoSmart) })
			elems.Activo = e.abajo == abajoSmart
			objs = append(objs, editor.Divisor(),
				selector("smartart.layout", anchoSelectorSmart, ops, p.Diagrama.Disposicion,
					func(v string) { aplicar(func() { e.m.PonerDisposicionSmart(e.objetivo(), id, v) }) }),
				elems)
		}
	}
	return append(objs, editor.Divisor(),
		selector("slides.onClick", anchoSelectorAccion, e.opcionesAccion(), ValorAccion(p.Accion),
			func(v string) { aplicar(func() { e.m.PonerAccion(e.objetivo(), id, v) }) }))
}

// opcionesAccion son las de «Al hacer clic»: ninguna, las cuatro relativas y una por
// diapositiva.
func (e *Editor) opcionesAccion() []editor.Opcion {
	ops := make([]editor.Opcion, 0, 5+len(e.m.Diapos))
	ops = append(ops,
		editor.Opcion{Valor: "", Rotulo: e.t("slides.link.none")},
		editor.Opcion{Valor: "next", Rotulo: e.t("nav.nextItem")},
		editor.Opcion{Valor: "prev", Rotulo: e.t("nav.previous")},
		editor.Opcion{Valor: "first", Rotulo: e.t("slides.link.first")},
		editor.Opcion{Valor: "last", Rotulo: e.t("slides.link.last")},
	)
	for i := range e.m.Diapos {
		ops = append(ops, editor.Opcion{
			Valor: strconv.Itoa(i + 1), Rotulo: e.ctx.Traducir("slides.number", i18n.Vars{"n": i + 1}),
		})
	}
	return ops
}

// disposicionDiapos es el `flex` de SlidesCanvas: el carril de 160 a la izquierda y la
// zona del escenario con el resto; y en un teléfono (`max-sm:flex-col`, ventana de menos
// de 640), el carril ARRIBA como tira horizontal y la zona debajo. En columna se comía
// media pantalla: en un iPhone SE la diapositiva no pasaba de 77 px (medido en la web).
type disposicionDiapos struct{ e *Editor }

func (disposicionDiapos) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(AnchoCarrilDiapos+240, 160)
}

func (d disposicionDiapos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	tira := d.e.enTira()
	if tira != d.e.carril.tira {
		d.e.carril.ponerTira(tira) // la ventana ha cruzado los 640
	}
	if tira {
		alto := d.e.carril.altoTira()
		objs[0].Resize(fyne.NewSize(t.Width, alto))
		objs[1].Move(fyne.NewPos(0, alto))
		objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-alto, 0)))
		return
	}
	objs[0].Resize(fyne.NewSize(AnchoCarrilDiapos, t.Height))
	objs[1].Move(fyne.NewPos(AnchoCarrilDiapos, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-AnchoCarrilDiapos, 0), t.Height))
}

// enTira dice si la VENTANA es de teléfono: `max-sm:` mide la ventana con el rem de
// 16 del navegador (640 px), no el hueco del lienzo, que es 80 px más estrecho.
func (e *Editor) enTira() bool {
	ancho := e.raiz.Size().Width
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.raiz); c != nil {
		ancho = c.Size().Width
	}
	return ancho > 0 && ancho < ui.CorteSM
}

// zonaDiapo apila la mini-barra, la banda de aviso si la hay, el área del escenario
// (que se queda con lo que sobre) y el panel de notas si está abierto. Los objetos
// llegan en ese orden desde `recomponer`.
type zonaDiapo struct{ e *Editor }

func (z *zonaDiapo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(240, 160) }

func (z *zonaDiapo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	altos := make([]float32, len(objs))
	altos[0] = AltoMiniBarra
	area := 1
	if b, ok := objs[area].(*bandaPatron); ok {
		altos[area] = b.alto(t.Width)
		area++
	}
	if z.e.aviso != "" {
		altos[area] = banda{}.MinSize(nil).Height
		area++
	}
	if area+1 < len(objs) {
		altos[area+1] = ui.AltoA(objs[area+1], t.Width)
	}
	var fijo float32
	for _, a := range altos {
		fijo += a
	}
	altos[area] = max(t.Height-fijo, 0)
	y := float32(0)
	for i, o := range objs {
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, altos[i]))
		y += altos[i]
	}
}

// bandaInferior coloca un fondo, su línea de un píxel ABAJO y el contenido encima: la
// mini-barra (`h-9 border-b bg-surface`).
type bandaInferior struct{}

func (bandaInferior) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoMiniBarra) }

func (bandaInferior) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	objs[2].Move(fyne.NewPos(0, 0))
	objs[2].Resize(fyne.NewSize(t.Width, t.Height-tema.Borde))
}

// filaMiniBarra es `flex items-center justify-center-safe gap-1 px-1 *:shrink-0`: los
// controles a su tamaño y centrados si caben; si no, pegados al principio con su
// `px-1` y el resto se desliza (el desplazador es `deslizBarra`). Un `justify-center`
// que desborda dejaba lo de la izquierda donde el desplazamiento no llega.
type filaMiniBarra struct{}

func (filaMiniBarra) ancho(objs []fyne.CanvasObject) float32 {
	var ancho float32
	for i, o := range objs {
		if i > 0 {
			ancho += HuecoMiniBarra
		}
		ancho += o.MinSize().Width
	}
	return ancho
}

func (f filaMiniBarra) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.ancho(objs)+2*tema.Espacio(1), AltoMiniBarra-tema.Borde)
}

func (f filaMiniBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := max((t.Width-f.ancho(objs))/2, tema.Espacio(1))
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + HuecoMiniBarra
	}
}

// areaDiapo es el contenido del área desplazable del escenario: `flex-1 overflow-auto
// p-6` con la diapositiva `m-auto` dentro.
//
// La escala se calcula con lo que se VE —el tamaño del desplazador—, no con lo que
// ocupa el contenido: con zoom el contenido crece, y si la escala saliera de él, cada
// pasada de layout la agrandaría otra vez. Es el `area` que mide el ResizeObserver de
// la web.
type areaDiapo struct {
	e      *Editor
	desliz *container.Scroll
}

func (a *areaDiapo) escala() float32 {
	return a.e.m.Encaje(a.desliz.Size(), HuecoEncaje, float32(a.e.ctx.Zoom))
}

func (a *areaDiapo) MinSize([]fyne.CanvasObject) fyne.Size {
	k := a.escala()
	return fyne.NewSize(AnchoEscenario*k+2*RellenoArea, a.e.m.AltoEscenario()*k+2*RellenoArea)
}

func (a *areaDiapo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	k := a.escala()
	esc := fyne.NewSize(AnchoEscenario*k, a.e.m.AltoEscenario()*k)
	pos := fyne.NewPos(max((t.Width-esc.Width)/2, RellenoArea), max((t.Height-esc.Height)/2, RellenoArea))
	a.e.escenario.Escala = k
	a.e.escenario.Move(pos)
	a.e.escenario.Resize(esc)
	a.e.capa.Move(pos)
	a.e.capa.Resize(esc)
	a.e.sombra.Ajustar(pos, esc, a.e.variante())
}

// ---- el carril ----

// carrilDiapos es la columna de la izquierda: las miniaturas en su desplazador y el
// pie con nueva, duplicar y eliminar.
type carrilDiapos struct {
	widget.BaseWidget
	e      *Editor
	pila   *fyne.Container
	desliz *container.Scroll
	pie    []fyne.CanvasObject
	// tira dice si el carril es la tira horizontal de un teléfono.
	tira bool
}

func nuevoCarrilDiapos(e *Editor) *carrilDiapos {
	c := &carrilDiapos{e: e}
	c.pila = container.New(pila{tam: MedidaMiniatura(e.m)})
	c.desliz = container.NewVScroll(c.pila)
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityRole lo anuncia como contenedor; lo que se pulsa son las miniaturas.
func (c *carrilDiapos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

func (c *carrilDiapos) rehacer() {
	e := c.e
	minis := make([]fyne.CanvasObject, len(e.m.Diapos))
	for i := range e.m.Diapos {
		t := NuevaMiniatura(e.m, i, e.Elegir)
		t.Activa, t.Variante = i == e.activa, e.variante()
		t.AlMover = func(hasta int) { e.Mover(i, hasta) }
		t.Rotulo = func(clave string, n, total int) string {
			return e.ctx.Traducir(clave, i18n.Vars{"n": n, "total": total})
		}
		minis[i] = t
	}
	c.pila.Objects = minis
	c.ponerTira(e.enTira())

	ro := e.ctx.SoloLectura
	nueva := editor.NuevoBotonCinta(iconos.EdMas, e.t("slides.new"), e.NuevaDiapositiva)
	dup := editor.NuevoBotonCinta(iconos.EdDiapoNueva, e.t("slides.duplicate"), e.Duplicar)
	borrar := editor.NuevoBotonCinta(iconos.EdPapelera, e.t("slides.delete"), e.Eliminar)
	nueva.Inerte, dup.Inerte, borrar.Inerte = ro, ro, ro || len(e.m.Diapos) <= 1
	c.pie = []fyne.CanvasObject{nueva, dup, borrar}
	c.Refresh()
}

// ponerTira cambia el carril entre columna y tira de teléfono.
func (c *carrilDiapos) ponerTira(tira bool) {
	c.tira = tira
	c.pila.Layout = pila{tam: MedidaMiniatura(c.e.m), horizontal: tira}
	c.desliz.Direction = container.ScrollVerticalOnly
	if tira {
		c.desliz.Direction = container.ScrollHorizontalOnly
	}
	c.pila.Refresh()
	c.desliz.Refresh()
	c.Refresh()
}

// altoTira es lo que mide el carril hecho tira en un teléfono: la miniatura con su
// `p-2` arriba y abajo y el `border-b`.
func (c *carrilDiapos) altoTira() float32 {
	return MedidaMiniatura(c.e.m).Height + 2*tema.Espacio(2) + tema.Borde
}

func (c *carrilDiapos) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(AnchoCarrilDiapos, AnchoMiniatura)
}

func (c *carrilDiapos) CreateRenderer() fyne.WidgetRenderer {
	r := &carrilRenderer{
		c:      c,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		borde:  canvas.NewRectangle(color.NRGBA{}),
		lineaP: canvas.NewRectangle(color.NRGBA{}),
	}
	r.Refresh()
	return r
}

type carrilRenderer struct {
	c                    *carrilDiapos
	fondo, borde, lineaP *canvas.Rectangle
}

func (r *carrilRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *carrilRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.fondo, r.c.desliz, r.lineaP, r.borde}, r.c.pie...)
}
func (r *carrilRenderer) Destroy() {}

func (r *carrilRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	lado, p1 := editor.LadoBotonCinta, tema.Espacio(1)
	anchoPie := float32(len(r.c.pie))*lado + float32(max(len(r.c.pie)-1, 0))*p1
	if r.c.tira {
		r.layoutTira(t, lado, anchoPie)
		return
	}
	dentro := t.Width - tema.Borde // `border-r` dentro de los 160
	r.borde.Move(fyne.NewPos(dentro, 0))
	r.borde.Resize(fyne.NewSize(tema.Borde, t.Height))
	arriba := max(t.Height-AltoPieCarril, 0)
	r.c.desliz.Move(fyne.NewPos(0, 0))
	r.c.desliz.Resize(fyne.NewSize(dentro, arriba))
	r.lineaP.Move(fyne.NewPos(0, arriba))
	r.lineaP.Resize(fyne.NewSize(dentro, tema.Borde))
	// `flex items-center justify-center gap-1 p-1`.
	x := (dentro - anchoPie) / 2
	for _, b := range r.c.pie {
		b.Move(fyne.NewPos(x, arriba+tema.Borde+p1))
		b.Resize(fyne.NewSize(lado, lado))
		x += lado + p1
	}
}

// layoutTira es el carril en un teléfono: `max-sm:flex-row max-sm:border-b`, las
// miniaturas deslizándose de lado y el pie a la derecha con `max-sm:border-l`, sus
// botones centrados en el alto de la tira.
func (r *carrilRenderer) layoutTira(t fyne.Size, lado, anchoPie float32) {
	dentro := t.Height - tema.Borde
	r.borde.Move(fyne.NewPos(0, dentro))
	r.borde.Resize(fyne.NewSize(t.Width, tema.Borde))
	pie := anchoPie + 2*tema.Espacio(1) + tema.Borde
	izq := max(t.Width-pie, 0)
	r.c.desliz.Move(fyne.NewPos(0, 0))
	r.c.desliz.Resize(fyne.NewSize(izq, dentro))
	r.lineaP.Move(fyne.NewPos(izq, 0))
	r.lineaP.Resize(fyne.NewSize(tema.Borde, dentro))
	x := izq + tema.Borde + tema.Espacio(1)
	for _, b := range r.c.pie {
		b.Move(fyne.NewPos(x, (dentro-lado)/2))
		b.Resize(fyne.NewSize(lado, lado))
		x += lado + tema.Espacio(1)
	}
}

func (r *carrilRenderer) Refresh() {
	v := r.c.e.variante()
	r.fondo.FillColor = tema.Color("surface", v)
	r.borde.FillColor = tema.Color("border", v)
	r.lineaP.FillColor = tema.Color("border", v)
	for _, o := range r.Objects() {
		o.Refresh()
	}
	r.Layout(r.c.Size())
}

// ---- el panel de notas ----

// panelNotas es `bottom === 'notes'`: la cabecera con «Notas» y el número de la
// diapositiva, y el `<textarea>` de las notas del ponente.
//
//	<div class="border-border bg-surface shrink-0 border-t">
//	  <div class="flex items-center justify-between px-3 pt-1">
//	    <span class="text-text text-xs font-semibold">…</span>
//	    <span class="sytue-meta-dense">…</span>
//	  <textarea class="bg-surface text-text placeholder:text-text-subtle h-20 w-full
//	                   resize-none px-3 pt-1 pb-2 text-sm sytue-focus-ring">
type panelNotas struct {
	widget.BaseWidget
	e      *Editor
	area   *ui.EditorPlano
	enfoca bool
}

func nuevoPanelNotas(e *Editor) *panelNotas {
	p := &panelNotas{e: e}
	p.area = ui.NuevoEditorPlanoMultilinea(tema.TextoSM, func(f bool) { p.enfoca = f; p.Refresh() })
	p.area.AlCambiar(func(s string) {
		e.cambiar(func() { e.m.PonerNotas(e.activa, s) }, false)
	})
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el `aria-label` del área: «Notas de la diapositiva n».
func (p *panelNotas) AccessibilityLabel() string {
	return p.e.ctx.Traducir("slides.notes.of", i18n.Vars{"n": p.e.activa + 1})
}

// AccessibilityRole es contenedor: dentro está el área de texto, que se anuncia sola.
func (p *panelNotas) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// sincronizar trae el texto de la diapositiva activa, sin pisar lo que se está
// tecleando: si el área ya tiene ese texto no se toca, y el cursor se queda donde
// estaba.
func (p *panelNotas) sincronizar() {
	d := p.e.m.Diapos[p.e.activa]
	p.area.PlaceHolder = p.e.t("slides.notes.hint")
	if p.area.Texto() != d.Notas {
		onc := p.area.OnChanged
		p.area.OnChanged = nil
		p.area.SetText(d.Notas)
		p.area.OnChanged = onc
	}
	p.area.SoloLectura(p.e.ctx.SoloLectura)
	p.Refresh()
}

func (p *panelNotas) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(0, AltoNotas)
}

func (p *panelNotas) CreateRenderer() fyne.WidgetRenderer {
	v := p.e.variante()
	r := &notasRenderer{
		p:      p,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		borde:  canvas.NewRectangle(tema.Color("border", v)),
		titulo: ui.Texto("", tema.TextoXS, ui.SemiNegrita, tema.Color("text", v)),
		numero: ui.Texto("", tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
		aro:    ui.NuevoAro(0, ui.SinHueco, v),
	}
	r.Refresh()
	return r
}

type notasRenderer struct {
	p              *panelNotas
	fondo, borde   *canvas.Rectangle
	titulo, numero *canvas.Text
	aro            *ui.Aro
}

func (r *notasRenderer) MinSize() fyne.Size { return r.p.MinSize() }
func (r *notasRenderer) Objects() []fyne.CanvasObject {
	return append(append([]fyne.CanvasObject{r.fondo, r.borde, r.titulo, r.numero}, r.aro.Objetos()...),
		r.p.area.Objeto())
}
func (r *notasRenderer) Destroy() {}

func (r *notasRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.borde.Resize(fyne.NewSize(t.Width, tema.Borde))
	x, y := tema.Espacio(3), tema.Borde+tema.Espacio(1)
	ui.ColocarTexto(r.titulo, tema.TextoXS, x, y)
	ancho := ui.Medir(r.numero.Text, tema.Texto2XS, ui.Normal).Width
	ui.ColocarTexto(r.numero, tema.Texto2XS, t.Width-x-ancho, y+(tema.TextoXS.Linea-tema.Texto2XS.Linea)/2)

	// El `<textarea>` ocupa su `h-20` debajo, a lo ancho; su texto empieza tras
	// `px-3 pt-1` y la primera línea va centrada en su caja de línea de CSS, como hace
	// `ColocarTexto`.
	caja := fyne.NewPos(0, y+tema.TextoXS.Linea)
	tamCaja := fyne.NewSize(t.Width, tema.Espacio(20))
	r.aro.ColocarEn(caja, tamCaja)
	medio := (tema.TextoSM.Linea - r.p.area.AltoFuente()) / 2
	r.p.area.Colocar(caja.AddXY(x, tema.Espacio(1)+medio),
		fyne.NewSize(t.Width-2*x, tamCaja.Height-tema.Espacio(1)-tema.Espacio(2)-medio))
}

func (r *notasRenderer) Refresh() {
	v := r.p.e.variante()
	r.fondo.FillColor = tema.Color("surface", v)
	r.borde.FillColor = tema.Color("border", v)
	r.titulo.Text, r.titulo.Color = r.p.e.t("slides.notes"), tema.Color("text", v)
	r.numero.Text = r.p.e.ctx.Traducir("slides.number", i18n.Vars{"n": r.p.e.activa + 1})
	r.numero.Color = tema.Color("text-subtle", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.p.enfoca)
	for _, o := range []fyne.CanvasObject{r.fondo, r.borde, r.titulo, r.numero} {
		o.Refresh()
	}
	r.Layout(r.p.Size())
}

// ---- la banda de lo que no se pinta ----

// cartel es la banda de lo que este lienzo NO dibuja todavía, dicho con todas las
// letras. No existe en la web —allí se pinta todo—; aquí es la mitad honesta de lo que
// falta: sin ella, alguien daría por buena una diapositiva a la que le falta el gráfico.
// Se pinta como la franja de solo lectura del cromo: `bg-selected`, borde inferior y
// `text-xs`.
func cartel(texto string, v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("selected", v))
	borde := canvas.NewRectangle(tema.Color("border", v))
	t := ui.Texto(texto, tema.TextoXS, ui.Normal, tema.Color("text", v))
	return container.New(banda{}, fondo, borde, t)
}

type banda struct{}

func (banda) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(1)+tema.TextoXS.Linea+tema.Borde)
}

func (banda) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	if texto, ok := objs[2].(*canvas.Text); ok {
		ui.ColocarTexto(texto, tema.TextoXS, tema.Espacio(3), tema.Espacio(1))
	}
}

// colorLienzo es `--editor-canvas-bg`: el fondo sobre el que flota el papel. Va literal
// porque no es un token de la paleta compartida, es del editor.
func colorLienzo(v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return color.NRGBA{R: 0x18, G: 0x1A, B: 0x1C, A: 255}
	}
	return color.NRGBA{R: 0xE9, G: 0xE6, B: 0xDD, A: 255}
}

// ---- «Editar patrón» ----

// bandaPatron es el aviso bajo la mini-barra mientras se edita el patrón: `bg-selected
// border-b px-3 py-1 flex items-center justify-between gap-3`, con el texto `text-xs`
// —que envuelve si no cabe— y «Cerrar patrón».
type bandaPatron struct {
	widget.BaseWidget
	texto  string
	cerrar *botonPanel
}

func (e *Editor) bandaPatron() *bandaPatron {
	cerrar := nuevoBotonPanel(iconos.EdTic, e.t("slides.master.close"), e.t("slides.master.close"), e.CerrarPatron)
	cerrar.Contorno = true
	// Los campos van por variable, como en la web: el interpolador no vuelve a leer lo
	// que sustituye, así que «{n}» llega literal al rótulo.
	b := &bandaPatron{
		texto:  e.ctx.Traducir("slides.master.banner", i18n.Vars{"fieldN": "{n}", "fieldTotal": "{total}"}),
		cerrar: cerrar,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *bandaPatron) lineas(ancho float32) []string {
	cabe := ancho - 2*tema.Espacio(3) - tema.Espacio(3) - b.cerrar.MinSize().Width
	return ui.Lineas(b.texto, tema.TextoXS, ui.Normal, max(cabe, 1))
}

// alto es el de la banda con ese ancho: `py-1`, lo más alto entre el texto y el botón,
// y el borde.
func (b *bandaPatron) alto(ancho float32) float32 {
	n := float32(len(b.lineas(ancho)))
	return 2*tema.Espacio(1) + max(n*tema.TextoXS.Linea, b.cerrar.MinSize().Height) + tema.Borde
}

func (b *bandaPatron) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(1, b.alto(b.Size().Width))
}

func (b *bandaPatron) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &bandaPatronRenderer{
		b:     b,
		fondo: canvas.NewRectangle(tema.Color("selected", v)),
		borde: canvas.NewRectangle(tema.Color("border", v)),
	}
}

type bandaPatronRenderer struct {
	b            *bandaPatron
	fondo, borde *canvas.Rectangle
	lineas       []*canvas.Text
}

func (r *bandaPatronRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.Resize(t)
	r.borde.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.borde.Resize(fyne.NewSize(t.Width, tema.Borde))
	dentro := t.Height - 2*tema.Espacio(1) - tema.Borde
	ls := r.b.lineas(t.Width)
	y := tema.Espacio(1) + (dentro-float32(len(ls))*tema.TextoXS.Linea)/2
	r.lineas = r.lineas[:0]
	for _, l := range ls {
		tx := ui.Texto(l, tema.TextoXS, ui.Normal, tema.Color("text", v))
		ui.ColocarTexto(tx, tema.TextoXS, tema.Espacio(3), y)
		r.lineas = append(r.lineas, tx)
		y += tema.TextoXS.Linea
	}
	m := r.b.cerrar.MinSize()
	r.b.cerrar.Move(fyne.NewPos(t.Width-tema.Espacio(3)-m.Width, tema.Espacio(1)+(dentro-m.Height)/2))
	r.b.cerrar.Resize(m)
}

func (r *bandaPatronRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *bandaPatronRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo, r.borde}
	for _, l := range r.lineas {
		objs = append(objs, l)
	}
	return append(objs, r.b.cerrar)
}
func (r *bandaPatronRenderer) Refresh() { r.Layout(r.b.Size()); canvas.Refresh(r.b) }
func (r *bandaPatronRenderer) Destroy() {}
