// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

/*
La CINTA de CertiPDF (`pdf/Barra.tsx`): una fila con la salida, el cajón y las seis
pestañas, y debajo los grupos de la pestaña abierta.

Era una barra única de 415 px con catorce herramientas, seis colores, el grosor, la
paginación, el zoom y seis diálogos, todo a la vez. Ahora son pestañas por el mismo
motivo que en el editor: en un programa con sesenta acciones lo que ahorra tiempo no es
tenerlas a la vista, es saber dónde mirar.

De la cinta del editor, `Barra.tsx` importa EXACTAMENTE una cosa: `RibbonGroup`. Aquí
igual: [editor.NuevoGrupo] y nada más. Las pestañas NO son las del editor —aquellas son
`text-sm` con subrayado; estas, `text-xs` con la esquina de arriba redondeada y el fondo
de la banda en la abierta— y los botones tampoco (`h-8`/`h-7` con `rounded-md`).
*/

// Herramienta es la del puntero sobre el documento. Son las de `Barra.tsx`.
type Herramienta uint8

// Las herramientas, en el orden de `HERRAMIENTAS`.
const (
	Mano Herramienta = iota
	Resaltar
	Subrayar
	Tachar
	Nota
	Lapiz
	Rectangulo
	Elipse
	Linea
	Flecha
	Medida
	TextoLibre
	Sello
	CampoNuevo
	Recorte
	Censura
)

// herramienta es la fila de la tabla `HERRAMIENTAS` de la web: su clave y su glifo.
type herramienta struct {
	id    Herramienta
	clave string
	icono iconos.Icono
}

// herramientas es `HERRAMIENTAS` de `Barra.tsx`, con el MISMO glifo en cada fila.
var herramientas = []herramienta{
	{Mano, "pdf.tool.hand", iconos.Mano},
	{Resaltar, "pdf.tool.highlight", iconos.Rotulador},
	{Subrayar, "pdf.tool.underline", iconos.Subrayado},
	{Tachar, "pdf.tool.strike", iconos.Tachado},
	{Nota, "pdf.tool.note", iconos.Comentario},
	{Lapiz, "pdf.tool.pen", iconos.Lapiz},
	{Rectangulo, "pdf.tool.rect", iconos.Cuadrado},
	{Elipse, "pdf.tool.ellipse", iconos.Circulo},
	{Linea, "pdf.tool.line", iconos.Linea},
	{Flecha, "pdf.tool.arrow", iconos.FlechaArribaDerecha},
	{Medida, "pdf.tool.measure", iconos.Regla},
	{TextoLibre, "pdf.tool.text", iconos.Texto},
	{Sello, "pdf.tool.stamp", iconos.Sello},
	{CampoNuevo, "pdf.tool.field", iconos.CampoDeFormulario},
	{Recorte, "pdf.tool.snapshot", iconos.Recorte},
	{Censura, "pdf.tool.redact", iconos.Censura},
}

// PestanaCinta es una de las seis de la cinta.
type PestanaCinta uint8

// Las seis, en el orden en que se usan a lo largo de un documento.
const (
	PestInicio PestanaCinta = iota
	PestAnotar
	PestFormulario
	PestOrganizar
	PestFirmar
	PestVer
)

// rotuloPestana es la clave de catálogo del rótulo de cada pestaña (`PESTANAS`).
var rotuloPestana = map[PestanaCinta]string{
	PestInicio:     "ribbon.tab.home",
	PestAnotar:     "pdf.ribbon.tab.annotate",
	PestFormulario: "pdf.ribbon.tab.form",
	PestOrganizar:  "pdf.menu.organize",
	// «Firmar» y no «Proteger» (orden del dueño, 2026-10-16): firmar es a lo que se viene
	// a CertiPDF. El rótulo es el del botón «Firmar», como «Organizar» es el del suyo;
	// `pdf.menu.protect` sigue nombrando el botón de la contraseña, que sí protege.
	PestFirmar: "pdf.menu.sign",
	PestVer:    "ribbon.tab.view",
}

// enPestana es `EN_PESTANA`: qué herramientas viven en cada pestaña.
//
// Censurar vive en Firmar, con la contraseña, y no junto al resaltador: es lo ÚNICO de la
// cinta que DESTRUYE contenido.
var enPestana = map[PestanaCinta][]Herramienta{
	PestInicio:     {Mano, Resaltar, Subrayar, Tachar, Nota},
	PestAnotar:     {Lapiz, Rectangulo, Elipse, Linea, Flecha, Medida, TextoLibre, Sello},
	PestFormulario: {CampoNuevo, Recorte},
	PestOrganizar:  nil,
	PestFirmar:     {Censura},
	PestVer:        nil,
}

// ColorAnotacion es uno de los seis de la paleta de anotación (`COLORES`).
type ColorAnotacion struct {
	// Hex es el color tal cual lo escribe la web: es el que va DENTRO del PDF.
	Hex   string
	Clave string
}

// ColoresAnotacion son los seis de `COLORES`, en su orden.
var ColoresAnotacion = []ColorAnotacion{
	{"#f9b923", "pdf.color.amber"},
	{"#22c55e", "pdf.color.green"},
	{"#3b82f6", "pdf.color.blue"},
	{"#ef4444", "pdf.color.red"},
	{"#a855f7", "pdf.color.violet"},
	{"#111827", "pdf.color.ink"},
}

// Traductor es lo que la cinta necesita del catálogo.
type Traductor func(clave string) string

// Ajuste es cómo se calcula el zoom: al ancho, a la página o a mano.
type Ajuste uint8

// Los tres de `PdfApp.tsx`. El cero es «al ancho», que es con el que nace.
const (
	AjusteAncho Ajuste = iota
	AjustePagina
	AjusteLibre
)

// Barra es la cinta entera. Sus campos son las props de `BarraHerramientas`: la
// pantalla los pone antes de componer.
type Barra struct {
	t      Traductor
	activa PestanaCinta
	// Herramienta es la seleccionada.
	Herramienta Herramienta
	// Color es el índice en [ColoresAnotacion].
	Color int
	// Grosor es el del trazo, de [GrosorMin] a [GrosorMax].
	Grosor int
	// PuedeAnotar sale de los permisos del documento.
	PuedeAnotar bool

	// Lo de «Ver»: página, zoom y lectura.
	Pagina, Total int
	Zoom          float64
	Ajuste        Ajuste
	DosPaginas    bool
	Leyendo       bool
	// Trabajando es la rueda del final de la fila (`pdf.working`) y apaga los botones
	// de los diálogos.
	Trabajando bool
	// DesdeDrive cambia el rótulo de la salida: vuelve a la carpeta y no al Hub.
	DesdeDrive bool
	// PanelAbierto es el `aria-expanded` del botón del cajón.
	PanelAbierto bool

	// Portal es donde se pintan las pistas de sus botones, fuera del desplazador que las
	// recortaría. Nil: junto al botón.
	Portal *fyne.Container

	// AlCambiar lo llama la cinta cuando algo suyo cambia. Puede ser nil.
	AlCambiar func()
	// AlPulsarDialogo lo llama un botón de texto de Organizar o Firmar.
	AlPulsarDialogo func(d Dialogo)
	AlIrA           func(n int)
	AlZoom          func(delta float64)
	AlAjuste        func(a Ajuste)
	AlDosPaginas    func(bool)
	AlLeer          func()
	AlVolver        func()
	AlPanel         func(abierto bool)
	// AlNuevo y AlAbrir son «Crear nuevo» y «Abrir un archivo» del grupo «Archivo»:
	// orden del dueño para los editores de Fyne (2026-09-23), que la cinta de la web no
	// tiene —allí se crea y se abre desde la portada—. Nil: el grupo no sale.
	AlNuevo, AlAbrir func()

	// porciento es el rótulo del zoom, que el ajuste cambia sin rehacer la cinta.
	porciento *ui.Rotulo
	// Ancho es el de la ventana: por debajo de `md` los grupos se encogen y envuelven, y
	// la banda crece. Lo pone la pantalla antes de componer.
	Ancho float32
	// grupos son los de la última banda compuesta, para saber cuánto mide.
	grupos []*editor.Grupo
}

// md es el `md:` de Tailwind medido con la raíz de 16: 768 px de ventana.
const md float32 = 768

// Dialogo es uno de los seis diálogos de la cinta (`type Dialogo` en `Paneles.tsx`).
type Dialogo uint8

// Los seis.
const (
	SinDialogo Dialogo = iota
	DialogoOrganizar
	DialogoHerramientas
	DialogoCapas
	DialogoComparar
	DialogoProteger
	DialogoFirmar
	DialogoPedirFirma
)

// Las medidas de `Barra.tsx`, con raíz de16px.
var (
	// ladoSalida es el `h-9 w-9` de la salida y del botón del cajón.
	ladoSalida = tema.Espacio(9)
	// margenFila es el `px-1 pt-1` de la fila de arriba y el `gap-1` entre sus piezas.
	margenFila = tema.Espacio(1)
	// pestanaPXBarra y pestanaPYBarra son el `px-3 py-1.5` de una pestaña.
	pestanaPXBarra = tema.Espacio(3)
	pestanaPYBarra = tema.Espacio(1.5)
	// rellenoCintaY es el `py-0.5` de la banda; el `px-1` y el `gap-1` son margenFila.
	rellenoCintaY = tema.Espacio(0.5)
)

// AltoFilaBarra es la fila de arriba: su `pt-1` y la salida de `h-9`, que es lo más
// alto que lleva (las pestañas miden 28 px y van centradas).
func AltoFilaBarra() float32 { return margenFila + ladoSalida }

// AltoGrupoBarra es un `RibbonGroup` a su alto natural: `pt-1.5`, la fila de `min-h-11`,
// el `gap-0.5`, el rótulo con `leading-none` y el `pb-1`. En el editor la banda tiene
// alto fijo y el grupo lo llena; aquí la banda mide lo que mide el grupo.
func AltoGrupoBarra() float32 {
	return editor.ArribaGrupo + editor.AltoFilaGrupo + editor.HuecoControl + editor.AltoRotuloGrupo + editor.AbajoGrupo
}

// AltoBarra es lo que ocupa la cinta cuando sus grupos caben sin encogerse: la fila, la
// banda con su `py-0.5` y el `border-b`.
func AltoBarra() float32 {
	return AltoFilaBarra() + AltoGrupoBarra() + 2*rellenoCintaY + tema.Borde
}

// Alto es lo que ocupa la cinta con sus grupos: sólo envuelve el grupo cuyo ancho
// natural supera la ventana, y entonces hace crecer la banda entera.
func (b *Barra) Alto() float32 {
	_, alto := repartoBanda(b.grupos, b.disponible())
	return AltoFilaBarra() + alto + 2*rellenoCintaY + tema.Borde
}

// disponible es el ancho de la banda: la ventana menos el `px-1`.
func (b *Barra) disponible() float32 {
	if b.Ancho <= 0 {
		return 0
	}
	return b.Ancho - 2*margenFila
}

// repartoBanda es la banda overflow-x-auto con RibbonGroup shrink-0 max-w-full.
// Cada grupo conserva su ancho natural, limitado individualmente al ancho de la
// banda. La suma puede superar ese ancho: el desplazador conserva el desbordamiento.
func repartoBanda(grupos []*editor.Grupo, disponible float32) (anchos []float32, alto float32) {
	anchos = make([]float32, len(grupos))
	for i, g := range grupos {
		anchos[i] = editor.MedidaGrupo(g).Width
		if disponible > 0 {
			anchos[i] = min(anchos[i], disponible)
		}
		// Por debajo de `md` el grupo no lleva rótulo (RibbonGroup) y la banda es más baja.
		alto = max(alto, editor.AltoGrupoEn(g, anchos[i]))
	}
	if alto == 0 {
		alto = AltoGrupoBarra()
	}
	return anchos, alto
}

// NuevaBarra monta la cinta con la pestaña de Inicio abierta, que es como nace.
func NuevaBarra(t Traductor) *Barra {
	b := &Barra{t: t, PuedeAnotar: true, Zoom: 1}
	b.reiniciar()
	return b
}

// reiniciar la deja como nace, para otro documento: Inicio, la mano, el primer color y
// el grosor de siempre.
func (b *Barra) reiniciar() {
	b.activa, b.Herramienta, b.Color, b.Grosor = PestInicio, Mano, 0, GrosorPorDefecto
}

// Elegir abre una pestaña.
func (b *Barra) Elegir(p PestanaCinta) {
	b.activa = p
	b.cambio()
}

func (b *Barra) cambio() {
	if b.AlCambiar != nil {
		b.AlCambiar()
	}
}

// Activa es la pestaña abierta.
func (b *Barra) Activa() PestanaCinta { return b.activa }

// Objeto compone la cinta: `border-border bg-surface flex flex-col border-b`.
func (b *Barra) Objeto() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	banda := canvas.NewRectangle(tema.Color("bg-subtle", v))
	grupos, s := deslizableCon(b.bandaDeGrupos())
	// La banda es el `<Toolbar>` que la web enmascara entero —fondo incluido— sobre el
	// `bg-surface` de la cinta.
	cinta := conMascara(container.New(ui.Margen{Izq: margenFila, Arr: rellenoCintaY, Der: margenFila, Abj: rellenoCintaY}, grupos), s, "surface")
	return container.New(pilaBarra{b: b},
		canvas.NewRectangle(tema.Color("surface", v)), canvas.NewRectangle(tema.Color("border", v)), banda,
		b.fila(), cinta)
}

// pilaBarra coloca [fondo, línea, banda, fila, cinta]: la fila arriba con su alto, la
// cinta debajo sobre la banda `bg-bg-subtle` y el `border-b` al pie.
type pilaBarra struct{ b *Barra }

func (p pilaBarra) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(max(objs[3].MinSize().Width, 0), p.b.Alto())
}

func (pilaBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	fila := AltoFilaBarra()
	cinta := t.Height - fila - tema.Borde
	objs[2].Move(fyne.NewPos(0, fila))
	objs[2].Resize(fyne.NewSize(t.Width, cinta))
	objs[3].Move(fyne.NewPos(0, 0))
	objs[3].Resize(fyne.NewSize(t.Width, fila))
	objs[4].Move(fyne.NewPos(0, fila))
	objs[4].Resize(fyne.NewSize(t.Width, cinta))
}

// fila es `flex items-center gap-1 px-1 pt-1`: la salida, el botón del cajón (solo en
// estrecho), la tira de pestañas y, al final, la rueda si se está trabajando.
func (b *Barra) fila() fyne.CanvasObject {
	rotulo := "pdf.back"
	if b.DesdeDrive {
		rotulo = "pdf.backToDrive"
	}
	salida := botonBarra(iconos.ChevronIzquierda, b.t(rotulo), ladoSalida, 18, b.AlVolver)
	cajon := botonBarra(iconos.Menu, b.t("pdf.panel.title"), ladoSalida, 18, func() {
		if b.AlPanel != nil {
			b.AlPanel(!b.PanelAbierto)
		}
	})
	cajon.Pulsado = b.PanelAbierto
	pestanas := make([]fyne.CanvasObject, 0, 6)
	for p := PestInicio; p <= PestVer; p++ {
		q := p
		pestanas = append(pestanas, nuevaPestanaBarra(b.t(rotuloPestana[p]), p == b.activa, func() { b.Elegir(q) }))
	}
	tira := deslizable(container.New(ui.Fila{Hueco: tema.Espacio(0.5)}, pestanas...))
	var rueda fyne.CanvasObject = canvas.NewRectangle(color.Transparent)
	if b.Trabajando {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		rueda = ui.NuevaRueda(16, "text-muted", v)
	}
	return container.New(&filaBarra{}, b.pista(salida, salida.Rotulo), b.pista(cajon, cajon.Rotulo), tira, rueda)
}

// filaBarra coloca [salida, cajón, tira, rueda]. El cajón es `lg:hidden`, y `lg:` mide
// la VENTANA con el rem de 16: 1024. La fila ocupa la ventana entera de ancho, así que
// su propio ancho es el de la ventana.
type filaBarra struct{}

// Estrecho es `max-lg:`: por debajo de 1024 el panel es un cajón.
const Estrecho float32 = 1024

func (*filaBarra) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*margenFila+2*ladoSalida, AltoFilaBarra())
}

func (*filaBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := margenFila
	x := margenFila
	objs[0].Move(fyne.NewPos(x, y))
	objs[0].Resize(fyne.NewSize(ladoSalida, ladoSalida))
	x += ladoSalida + margenFila
	if t.Width < Estrecho {
		objs[1].Show()
		objs[1].Move(fyne.NewPos(x, y))
		objs[1].Resize(fyne.NewSize(ladoSalida, ladoSalida))
		// `mr-1` además del `gap-1`.
		x += ladoSalida + margenFila + margenFila
	} else {
		objs[1].Hide()
	}
	rueda := float32(0)
	if _, es := objs[3].(*ui.Rueda); es {
		// `mr-2` y el `gap-1`.
		rueda = 16 + tema.Espacio(2) + margenFila
		objs[3].Move(fyne.NewPos(t.Width-16-tema.Espacio(2), y+(ladoSalida-16)/2))
		objs[3].Resize(fyne.NewSize(16, 16))
	}
	// `min-w-0` con `flex-1` detrás: la tira mide lo que sus pestañas y, si no caben, se
	// desliza.
	alto := objs[2].MinSize().Height
	ancho := min(contenidoDe(objs[2]).Width, max(t.Width-x-margenFila-rueda, 0))
	objs[2].Move(fyne.NewPos(x, y+(ladoSalida-alto)/2))
	objs[2].Resize(fyne.NewSize(ancho, alto))
}

// bandaDeGrupos es la banda: los grupos de la pestaña abierta, con `gap-1`.
func (b *Barra) bandaDeGrupos() fyne.CanvasObject {
	grupos := make([]fyne.CanvasObject, 0, 4)

	if b.activa == PestInicio && (b.AlNuevo != nil || b.AlAbrir != nil) {
		var archivo []fyne.CanvasObject
		if b.AlNuevo != nil {
			archivo = append(archivo, b.botonAccion("list.new.heading", b.AlNuevo))
		}
		if b.AlAbrir != nil {
			archivo = append(archivo, b.botonAccion("list.new.open", b.AlAbrir))
		}
		grupos = append(grupos, editor.NuevoGrupo(b.t("file.menu"), archivo...))
	}
	// Lo primero de Firmar es firmar. Detrás, lo que protege el documento —quién lo abre y
	// qué se le puede hacer— y, la última, la censura: son los pasos con que se cierra un
	// documento antes de enviarlo.
	if b.activa == PestFirmar {
		grupos = append(grupos,
			editor.NuevoGrupo(b.t("pdf.panel.signatures"),
				b.botonDialogo("pdf.menu.sign", DialogoFirmar),
				b.botonDialogo("pdf.menu.requestSignature", DialogoPedirFirma)),
			editor.NuevoGrupo(b.t("pdf.ribbon.group.security"),
				b.botonDialogo("pdf.menu.protect", DialogoProteger)))
	}
	if hs := enPestana[b.activa]; len(hs) > 0 {
		grupos = append(grupos, editor.NuevoGrupo(b.t("pdf.tools"), b.botonesDeHerramienta(hs)...))
		if conTrazo(hs) {
			grupos = append(grupos, editor.NuevoGrupo(b.t("pdf.ribbon.group.stroke"),
				append(b.muestrasDeColor(), b.controlDeGrosor())...))
		}
	}

	switch b.activa {
	case PestOrganizar:
		grupos = append(grupos, editor.NuevoGrupo(b.t("pdf.ribbon.group.document"),
			b.botonDialogo("pdf.menu.organize", DialogoOrganizar),
			b.botonDialogo("pdf.menu.tools", DialogoHerramientas),
			b.botonDialogo("pdf.menu.stamp", DialogoCapas),
			b.botonDialogo("pdf.menu.compare", DialogoComparar)))
	case PestVer:
		grupos = append(grupos, b.gruposDeVer()...)
	case PestInicio, PestAnotar, PestFormulario, PestFirmar:
	}
	b.grupos = b.grupos[:0]
	for _, g := range grupos {
		if eg, ok := g.(*editor.Grupo); ok {
			eg.AlPrincipio = b.Ancho > 0 && b.Ancho < md
			b.grupos = append(b.grupos, eg)
		}
	}
	return container.New(bandaBarra{b: b}, grupos...)
}

// bandaBarra es flex items-stretch gap-1: grupos sin encoger por la suma de sus
// anchos, y todos al alto del más alto (ver repartoBanda).
type bandaBarra struct{ b *Barra }

func (l bandaBarra) MinSize(objs []fyne.CanvasObject) fyne.Size {
	anchos, alto := repartoBanda(l.b.grupos, l.b.disponible())
	var ancho float32
	for i, a := range anchos {
		if i > 0 {
			ancho += margenFila
		}
		ancho += a
	}
	return fyne.NewSize(ancho, alto)
}

func (l bandaBarra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	// t.Width es el contenido del scroll; max-w-full se refiere a la ventana visible.
	anchos, alto := repartoBanda(l.b.grupos, l.b.disponible())
	var x float32
	for i, o := range objs {
		w := o.MinSize().Width
		if i < len(anchos) {
			w = anchos[i]
		}
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(w, alto))
		x += w + margenFila
	}
}

// gruposDeVer son los tres grupos de la pestaña «Ver»: página, zoom y lectura.
func (b *Barra) gruposDeVer() []fyne.CanvasObject {
	anterior := botonSimple(iconos.ChevronIzquierda, b.t("pdf.page.prev"), func() { b.irA(b.Pagina - 1) })
	anterior.Inerte = b.Pagina <= 1
	siguiente := botonSimple(iconos.ChevronDerecha, b.t("pdf.page.next"), func() { b.irA(b.Pagina + 1) })
	siguiente.Inerte = b.Pagina >= b.Total

	// `border-border w-14 rounded-md border px-1.5 py-1 text-center text-xs tabular-nums`:
	// 23 de alto, no los 24 de la entrada `sm` de la suite.
	// ponytail: sin las flechas que Chromium pinta en un `type="number"` al pasar por
	// encima; se sube y se baja con los botones de al lado.
	campo := ui.NuevaEntrada("").ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1.5), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border", Centrado: true,
	})
	campo.Ancho = tema.Espacio(14)
	campo.PonerTexto(strconv.Itoa(b.Pagina))
	// `onChange` de un `<input type="number">`: salta al confirmar con Intro y al salir.
	// Y el campo es CONTROLADO (`value={pagina}`): lo que no lleva a otra página —letras,
	// un número fuera de rango que se queda en la misma— vuelve a enseñar la actual.
	ir := func() {
		if n, err := strconv.Atoi(strings.TrimSpace(campo.Texto())); err == nil && n != b.Pagina {
			b.irA(n)
		}
		campo.PonerTexto(strconv.Itoa(b.Pagina))
	}
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			ir()
			return true
		}
		return false
	}
	campo.AlEnfocar = func(dentro bool) {
		if !dentro {
			ir()
		}
	}
	pagina := container.New(filaCentrada{hueco: tema.Espacio(1)}, campo,
		ui.NuevoRotulo("/ "+strconv.Itoa(b.Total), tema.TextoXS, ui.Normal, "text-muted"))

	menos := botonSimple(iconos.Menos, b.t("pdf.zoom.out"), func() { b.zoom(-pasoZoom) })
	mas := botonSimple(iconos.Mas, b.t("pdf.zoom.in"), func() { b.zoom(pasoZoom) })
	porciento := ui.NuevoRotulo(strconv.Itoa(int(math.Round(b.Zoom*100)))+"%", tema.TextoXS, ui.Normal, "text-muted")
	b.porciento = porciento
	// `w-12 shrink-0 text-center`.
	caja := canvas.NewRectangle(color.Transparent)
	caja.SetMinSize(fyne.NewSize(tema.Espacio(12), tema.TextoXS.Linea))
	ancho := botonSimple(iconos.AjustarAncho, b.t("pdf.zoom.width"), func() { b.ajuste(AjusteAncho) })
	ancho.FondoPulsado, ancho.Pulsado = "selected", b.Ajuste == AjusteAncho
	pag := botonSimple(iconos.AjustarPagina, b.t("pdf.zoom.page"), func() { b.ajuste(AjustePagina) })
	pag.FondoPulsado, pag.Pulsado = "selected", b.Ajuste == AjustePagina

	dos := botonSimple(iconos.DosPaginas, b.t("pdf.view.twoUp"), func() {
		if b.AlDosPaginas != nil {
			b.AlDosPaginas(!b.DosPaginas)
		}
	})
	dos.FondoPulsado, dos.Pulsado = "selected", b.DosPaginas
	leer := "pdf.speak.start"
	if b.Leyendo {
		leer = "pdf.speak.stop"
	}
	voz := botonSimple(iconos.Voz, b.t(leer), func() {
		if b.AlLeer != nil {
			b.AlLeer()
		}
	})
	voz.FondoPulsado, voz.Pulsado = "selected", b.Leyendo

	return []fyne.CanvasObject{
		editor.NuevoGrupo(b.t("pdf.ribbon.group.page"), b.conPista(anterior), pagina, b.conPista(siguiente)),
		editor.NuevoGrupo(b.t("pdf.ribbon.group.zoom"), b.conPista(menos),
			container.New(centrado{}, caja, porciento), b.conPista(mas), b.conPista(ancho), b.conPista(pag)),
		editor.NuevoGrupo(b.t("pdf.ribbon.group.reading"), b.conPista(dos), b.conPista(voz)),
	}
}

// conPista es [Barra.pista] con el nombre del botón: el `title={titulo}` de `BotonSimple`.
func (b *Barra) conPista(bot *ui.BotonIcono) fyne.CanvasObject { return b.pista(bot, bot.Rotulo) }

func (b *Barra) irA(n int) {
	if b.AlIrA != nil {
		b.AlIrA(n)
	}
}

func (b *Barra) zoom(d float64) {
	if b.AlZoom != nil {
		b.AlZoom(d)
	}
}

func (b *Barra) ajuste(a Ajuste) {
	if b.AlAjuste != nil {
		b.AlAjuste(a)
	}
}

// centrado coloca [caja, rótulo]: la caja fija el ancho y el rótulo va en su centro.
type centrado struct{}

func (centrado) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	m := objs[1].MinSize()
	objs[1].Move(fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2))
	objs[1].Resize(m)
}

// botonBarra es un botón de solo icono de `Barra.tsx`: `rounded-md text-text
// hover:bg-bg-subtle`, `bg-selected` si está pulsado y al 40 % si está apagado.
//
// Al pulsarlo SUELTA el foco: casi todo lo que hace la cinta la rehace, y el botón viejo
// se quedaba enfocado fuera de la pantalla. Con algo enfocado Fyne no da las teclas
// sueltas a [Pantalla.Tecla], así que tras elegir una herramienta Escape no volvía a la
// mano, y tras «siguiente» Av Pág no pasaba de hoja; en la web el `keydown` del
// documento las oye igual.
// ponytail: quien va con el teclado vuelve a empezar el tabulador desde el principio,
// como con cualquier control que la cinta rehace; la web sigue desde el botón.
func botonBarra(i iconos.Icono, rotulo string, lado, icono float32, alPulsar func()) *ui.BotonIcono {
	var b *ui.BotonIcono
	b = ui.NuevoBotonIcono(i, rotulo, func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil && c.Focused() == b {
			c.Unfocus()
		}
		if alPulsar != nil {
			alPulsar()
		}
	})
	b.Lado, b.TamIcono, b.Radio = lado, icono, tema.Forma("radius-control")
	b.Trazo, b.TrazoEncima = "text", "text"
	b.FondoPulsado = ""
	return b
}

// botonSimple es `BotonSimple`: `h-7 w-7` con el glifo a 15.
func botonSimple(i iconos.Icono, rotulo string, alPulsar func()) *ui.BotonIcono {
	return botonBarra(i, rotulo, tema.Espacio(7), 15, alPulsar)
}

// pista es el `title` de la web en un control de la cinta: debajo, en el portal.
// ponytail: la del navegador sale junto al puntero y con su retardo; esta es la Tooltip
// de la suite, centrada bajo el control a los 300 ms.
func (b *Barra) pista(o fyne.CanvasObject, texto string) fyne.CanvasObject {
	p := ui.NuevaPista(texto, o)
	p.Abajo, p.Portal = true, b.Portal
	return p
}

// botonesDeHerramienta son los botones de una pestaña, ya con el permiso aplicado.
func (b *Barra) botonesDeHerramienta(hs []Herramienta) []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(hs))
	for _, id := range hs {
		h, ok := buscarHerramienta(id)
		if !ok {
			continue
		}
		bloqueada := h.id != Mano && !b.PuedeAnotar
		rotulo := b.t(h.clave)
		// Lo que el documento PROHÍBE no se enseña deshabilitado a secas: su pista es el
		// MOTIVO, que es lo que evita que se pruebe tres veces (`title={motivo}`); el
		// nombre sigue siendo el de la herramienta, como el `aria-label` de la web.
		pista := rotulo
		if bloqueada {
			pista = b.t("pdf.blocked.annotate")
		}
		id := h.id
		// `BotonHerramienta`: `h-8 w-8 rounded-md` con glifo de 16, y `bg-selected`
		// —NO `bg-accent`: en tema claro accent y accent-text son el mismo gris y la
		// herramienta activa salía como un cuadrado negro sin icono—.
		bot := botonBarra(h.icono, rotulo, ui.LadoIconoLG, iconos.TallaSM, func() {
			b.Herramienta = id
			b.cambio()
		})
		bot.FondoPulsado = "selected"
		bot.Pulsado = b.Herramienta == h.id
		bot.Inerte = bloqueada
		objs = append(objs, b.pista(bot, pista))
	}
	return objs
}

// botonAccion es un `BotonTexto` que no abre un diálogo sino que hace algo.
func (b *Barra) botonAccion(clave string, f func()) fyne.CanvasObject {
	a := NuevaAccion(b.t(clave), f)
	a.Inerte = b.Trabajando
	return a
}

// botonDialogo es `BotonTexto`: `text-text hover:bg-bg-subtle rounded-md px-2 py-1.5
// text-xs font-medium`, apagado mientras se trabaja.
func (b *Barra) botonDialogo(clave string, d Dialogo) fyne.CanvasObject {
	a := NuevaAccion(b.t(clave), func() {
		if b.AlPulsarDialogo != nil {
			b.AlPulsarDialogo(d)
		}
	})
	a.Inerte = b.Trabajando
	return a
}

// El recorrido del grosor es el `min="1" max="12" step="1"` de `Barra.tsx`, y el valor
// de arranque el `useState(2)` de `PdfApp.tsx`.
const (
	GrosorMin        = 1
	GrosorMax        = 12
	GrosorPorDefecto = 2
)

// AnchoDeslizadorGrosor es w-16 del input type=range: 64 px con raíz de 16.
var AnchoDeslizadorGrosor = tema.Espacio(16)

// muestrasDeColor son los seis botones redondos de la paleta.
func (b *Barra) muestrasDeColor() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(ColoresAnotacion))
	for i, c := range ColoresAnotacion {
		tinta, ok := tokens.ParseColor(c.Hex)
		if !ok {
			continue
		}
		m := ui.NuevaMuestraColor(tinta, b.t(c.Clave), func() {
			b.Color = i
			b.cambio()
		})
		// `h-[24px] w-[24px]`: el lado de CertiPDF es fijo, independiente del rem.
		m.Lado, m.Elegida = 24, b.Color == i
		objs = append(objs, b.pista(m, b.t(c.Clave)))
	}
	return objs
}

// controlDeGrosor es el `<label class="ml-2 flex items-center gap-1 text-xs">` con su
// deslizador de `w-16`.
func (b *Barra) controlDeGrosor() fyne.CanvasObject {
	rotulo := b.t("pdf.thickness")
	d := ui.NuevoDeslizador(GrosorMin, GrosorMax, float64(b.Grosor), func(v float64) {
		b.Grosor = int(math.Round(v))
		b.cambio()
	}).Con(rotulo)
	// El `<input type="range">` de la cinta va SIN estilo: es el de Chromium.
	d.Paso, d.Nativo = 1, true
	hueco := canvas.NewRectangle(color.Transparent)
	hueco.SetMinSize(fyne.NewSize(AnchoDeslizadorGrosor, d.MinSize().Height))
	return container.New(ui.Margen{Izq: tema.Espacio(2)},
		container.New(filaCentrada{hueco: tema.Espacio(1)},
			ui.NuevoRotulo(rotulo, tema.TextoXS, ui.Normal, "text-muted"),
			container.NewStack(hueco, d)))
}

// conTrazo dice si alguna de estas herramientas dibuja algo con color y grosor. La censura
// no: su barra es negra y opaca SIEMPRE ([colorCensura]), y seis colores que no cambian
// nada eran la mitad de la pestaña de firmar.
func conTrazo(hs []Herramienta) bool {
	for _, h := range hs {
		if h != Mano && h != Recorte && h != Censura {
			return true
		}
	}
	return false
}

func buscarHerramienta(id Herramienta) (herramienta, bool) {
	for _, h := range herramientas {
		if h.id == id {
			return h, true
		}
	}
	return herramienta{}, false
}

// ---- La pestaña de la cinta ----

// pestanaBarra es una pestaña de `Barra.tsx`: `rounded-t-md px-3 py-1.5 text-xs
// font-medium`, con `bg-bg-subtle text-text` si está abierta y `text-text-muted
// hover:bg-bg-subtle hover:text-text` si no.
type pestanaBarra struct {
	ui.Pulsable
	rotulo string
	activa bool
}

func nuevaPestanaBarra(rotulo string, activa bool, alPulsar func()) *pestanaBarra {
	p := &pestanaBarra{rotulo: rotulo, activa: activa}
	p.Nombre, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// MedidaPestanaBarra es el rótulo con su `px-3` y la caja de línea con su `py-1.5`.
func MedidaPestanaBarra(rotulo string) fyne.Size {
	return fyne.NewSize(ui.Medir(rotulo, tema.TextoXS, ui.Medio).Width+2*pestanaPXBarra,
		tema.TextoXS.Linea+2*pestanaPYBarra)
}

func (p *pestanaBarra) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return MedidaPestanaBarra(p.rotulo)
}

func (p *pestanaBarra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := canvas.NewRectangle(color.Transparent)
	f.TopLeftCornerRadius, f.TopRightCornerRadius = tema.Forma("radius-control"), tema.Forma("radius-control")
	r := &pestanaBarraRenderer{
		p: p, fondo: f,
		aro:    ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		rotulo: ui.Texto(p.rotulo, tema.TextoXS, ui.Medio, color.Black),
	}
	r.Refresh()
	return r
}

type pestanaBarraRenderer struct {
	p      *pestanaBarra
	fondo  *canvas.Rectangle
	aro    *ui.Aro
	rotulo *canvas.Text
}

func (r *pestanaBarraRenderer) MinSize() fyne.Size { return MedidaPestanaBarra(r.p.rotulo) }
func (r *pestanaBarraRenderer) Destroy()           {}
func (r *pestanaBarraRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.fondo, r.rotulo)
}

func (r *pestanaBarraRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	ui.ColocarTexto(r.rotulo, tema.TextoXS, pestanaPXBarra, pestanaPYBarra)
}

func (r *pestanaBarraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = color.Transparent
	r.rotulo.Color = tema.Color("text-muted", v)
	if r.p.activa || r.p.Encima() {
		r.fondo.FillColor = tema.Color("bg-subtle", v)
		r.rotulo.Color = tema.Color("text", v)
	}
	r.fondo.Refresh()
	r.rotulo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.p.Enfocado())
	r.Layout(r.p.Size())
}

var _ fyne.Widget = (*pestanaBarra)(nil)
