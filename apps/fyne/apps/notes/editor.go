// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"image"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: RichText.tsx —el editor de una nota de texto y su barra— y el cromo de
// las barras de edición de `@sytue/ui` (cromoDeEditor.ts).
//
// # Por qué un editor propio y no `widget.Entry`
//
// `widget.Entry` escribe TEXTO PLANO: la negrita, las listas, las tareas y los títulos de
// la nota se perderían en cuanto alguien tocara una letra, y el contrato de este paquete
// es que un documento no se corrompe. El editor escribe sobre el MISMO modelo que lee
// (`Documento`) y pinta con el MISMO maquetador que la lectura (`Maquetar`), así que lo
// que se ve escribiendo es lo que se ve después, a la letra.
//
// ponytail: dos límites conocidos. (1) No hay IME: el driver de Fyne no lo tiene en
// escritorio (docs/design.md, «Techos del toolkit»), así que el chino o el japonés no se
// teclean aquí ni en ningún campo nativo. (2) La barra no entra en el orden de tabulación
// (el Tab del cuerpo es sangrar, como en la web); se usa con el puntero o con sus atajos
// (Ctrl+B, Ctrl+I, Ctrl+U, Ctrl+K, Ctrl+Alt+0…6). El cursor sí parpadea, como el del
// navegador (cursor_parpadeo.go).

// Medidas de la barra y del cuerpo, de la hoja con el rem de 16.
const (
	LadoBotonEd      = 7 * 0.25 * tema.Rem   // `h-7 w-7`: 28
	HuecoBarraEd     = 0.5 * 0.25 * tema.Rem // `gap-0.5`
	PXBarraEd        = 1.5 * 0.25 * tema.Rem // `px-1.5`
	PYBarraEd        = 1 * 0.25 * tema.Rem   // `py-1`
	AltoSeparadorEd  = 5 * 0.25 * tema.Rem   // `h-5`
	MargenSeparadorE = 1 * 0.25 * tema.Rem   // `mx-1`
	GlifoEd          = 16
	PXZonaEd         = 3 * 0.25 * tema.Rem  // `px-3`
	PYZonaEd         = 2 * 0.25 * tema.Rem  // `py-2`
	MinAltoZonaEd    = 24 * 0.25 * tema.Rem // `min-h-24`
	// AnchoMaxBloqueEd es el `max-w-24` del rótulo del desplegable de bloque.
	AnchoMaxBloqueEd = 24 * 0.25 * tema.Rem
)

// ColoresTexto son los de la paleta de color del editor (`COLORES` de RichText.tsx).
var ColoresTexto = []string{"#b91c1c", "#c2410c", "#a16207", "#15803d", "#0e7490", "#1d4ed8", "#7e22ce", "#be185d"}

// ColorResaltado es el `RESALTADO` de la web.
const ColorResaltado = "#fff3a3"

// BloquesEd son los nueve del desplegable, con su clave: el párrafo, los seis títulos,
// la cita y el código, como `BLOQUES` de RichText.tsx.
var BloquesEd = []struct{ Valor, Clave string }{
	{"p", "editor.paragraph"},
	{"h1", "editor.h1"},
	{"h2", "editor.h2"},
	{"h3", "editor.h3"},
	{"h4", "editor.h4"},
	{"h5", "editor.h5"},
	{"h6", "editor.h6"},
	{"blockquote", "editor.quote"},
	{"pre", "editor.codeBlock"},
}

// Glifos del editor que vienen de glyphs.tsx.
const (
	NegritaD   = "M6.5 4.5h6a3.75 3.75 0 0 1 0 7.5h-6ZM6.5 12h7a3.75 3.75 0 0 1 0 7.5h-7Z"
	CursivaD   = "M14.5 4.5h-5M14.5 19.5h-5M14 4.5 10 19.5"
	SubrayadoD = "M7 4v6a5 5 0 0 0 10 0V4M5.5 20.5h13"
	TachadoD   = "M4.5 12h15M8 7.5A3.5 3.5 0 0 1 11.5 4h1.5a3.5 3.5 0 0 1 3.3 2.3M16 16.2a3.5 3.5 0 0 1-3.3 3.8h-1.5A3.5 3.5 0 0 1 7.7 16"
	VinetasD   = "M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01M9 6.5h10.5M9 12h10.5M9 17.5h10.5"
	NumerosD   = "M4 5h1.5v4M4 9h3M4 13.5h3v2H4v3h3M10 7h10M10 14.5h10"
	SangrarD   = "M4.5 5.5h15M10 10h9.5M10 14h9.5M4.5 18.5h15M4.5 9.5 7.5 12l-3 2.5"
	DesangrarD = "M4.5 5.5h15M10 10h9.5M10 14h9.5M4.5 18.5h15M7.5 9.5 4.5 12l3 2.5"
	IzquierdaD = "M4.5 6h15M4.5 10h9M4.5 14h15M4.5 18h9"
	CentroD    = "M4.5 6h15M7.5 10h9M4.5 14h15M7.5 18h9"
	DerechaD   = "M4.5 6h15M10.5 10h9M4.5 14h15M10.5 18h9"
	GomaD      = "M9 19.5h10.5M4.6 15.4l8.8-8.8a1.5 1.5 0 0 1 2.1 0l3.9 3.9a1.5 1.5 0 0 1 0 2.1L13.5 18.5H9.4l-4.8-3.1Z"
	TituloD    = "M6 5v14M18 5v14M6 12h12"
	// SimboloD es la omega de «Símbolos» y MayusD, el «Aa» de distinguir mayúsculas.
	// Propios del editor nativo, con el trazo de 24 y el grosor de los de glyphs.tsx.
	SimboloD = "M5 19.5h4.5v-2.3a6.5 6.5 0 1 1 5 0v2.3H19"
	// FormatoD es la «A» subrayada de «Más formato».
	FormatoD = "M6.5 16 11 4.5h2L17.5 16M8.2 12h7.6M4.5 20h15"
	// CopiarD son las dos hojas de «Copiar», TijerasD las tijeras de «Cortar» y PegarD
	// la tablilla del portapapeles de «Pegar»: los de la burbuja de la web (glyphs.tsx,
	// `copy`, `tijeras` y `pegar`), con el mismo trazo redondeado. Antes la burbuja
	// copiaba con el icono de «copias» del catálogo (esquinas vivas) junto a dos dibujos
	// propios que no eran los de la web.
	CopiarD  = "M10 8.5h8a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5h-8A1.5 1.5 0 0 1 8.5 18v-8A1.5 1.5 0 0 1 10 8.5ZM8.5 15.5H6A1.5 1.5 0 0 1 4.5 14V6A1.5 1.5 0 0 1 6 4.5h8A1.5 1.5 0 0 1 15.5 6v2.5"
	TijerasD = "M8.5 7a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0ZM8.5 17a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0ZM8 8.5l11.5 9M8 15.5l11.5-9"
	PegarD   = "M9 4.5h6v3H9ZM9 6H6.5A1.5 1.5 0 0 0 5 7.5v11A1.5 1.5 0 0 0 6.5 20h11a1.5 1.5 0 0 0 1.5-1.5v-11A1.5 1.5 0 0 0 17.5 6H15"
	// ArrobaD es la arroba de «Mencionar» y EstadoD, la etiqueta de un estado.
	ArrobaD = "M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Zm0 0v1.5a2.5 2.5 0 0 0 5 0V12a9 9 0 1 0-3.5 7.1"
	EstadoD = "M4.5 8.5a2 2 0 0 1 2-2h11a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-11a2 2 0 0 1-2-2ZM8 12h8"
	MayusD  = "M3.5 18 7.5 6l4 12M5 13.5h5M20.5 12v6M20.5 15.25a2.75 2.75 0 1 1-5.5 0 2.75 2.75 0 0 1 5.5 0Z"
	// CaritaD es la cara de «Emoji»; SubindiceD y SuperindiceD, la equis con el dos abajo
	// o arriba; AjusteD, la línea que vuelve («Ajustar líneas largas»). De glyphs.tsx.
	CaritaD      = "M12 3.5a8.5 8.5 0 1 1 0 17 8.5 8.5 0 0 1 0-17ZM8.5 14a4 4 0 0 0 7 0M9 9.5h.01M15 9.5h.01"
	SubindiceD   = "M4.5 6.5l7 9M11.5 6.5l-7 9M15.5 15a1.75 1.75 0 1 1 3.5 0c0 1.2-3.5 2.5-3.5 4.5h3.5"
	SuperindiceD = "M4.5 8.5l7 9M11.5 8.5l-7 9M15.5 5a1.75 1.75 0 1 1 3.5 0c0 1.2-3.5 2.5-3.5 4.5h3.5"
	AjusteD      = "M4.5 6.5h15M4.5 12h12a2.75 2.75 0 0 1 0 5.5h-4M14.5 15.5l-2 2 2 2M4.5 17.5h5"
	// CombinarD y CombinarAbajoD son dos celdas que se juntan en una, de lado o de arriba
	// abajo, y SepararD, una que se parte en dos (combinar.go). Propios, con el trazo de
	// 24 de los de la barra de tabla.
	CombinarD      = "M4.5 5.5h15v13h-15ZM8 12h8M13.5 9.5 16 12l-2.5 2.5M10.5 9.5 8 12l2.5 2.5"
	CombinarAbajoD = "M5.5 4.5v15h13v-15ZM12 8v8M9.5 13.5 12 16l2.5-2.5M9.5 10.5 12 8l2.5 2.5"
	SepararD       = "M4.5 5.5h15v13h-15ZM12 5.5v13M9.5 10 7.5 12l2 2M14.5 10l2 2-2 2"
)

// foto es un estado del editor para deshacer: el documento y la selección.
type foto struct {
	doc        *Documento
	cur, ancla Pos
}

// EditorNota es el editor de una nota: la barra de formato encima y el cuerpo editable.
//
// Lo que escribe se entrega con `AlCambiar` (el HTML que se guarda) en cada cambio; quien
// lo monta decide cuándo mandarlo —la pantalla lo hace con la pausa de 400 ms y el techo
// de 2 s de la web—. `AlTerminar` es salir: Escape, Ctrl+Intro o el foco a otra cosa.
type EditorNota struct {
	widget.BaseWidget

	tr         *i18n.Translator
	doc        *Documento
	cur, ancla Pos
	// pendiente es el estilo de lo siguiente que se escriba cuando se ha pulsado negrita
	// (o cursiva…) sin nada seleccionado; se olvida al mover el cursor.
	pendiente *Tramo
	deshacer  []foto
	rehacer   []foto
	// escribiendo agrupa las letras de una racha en un solo paso de deshacer.
	escribiendo bool
	xDeseada    float32
	hayX        bool
	shift       bool

	marca string
	zona  *zonaEdicion
	// barra es la barra de formato (barra_editor.go): una fila con lo que cabe y «Más».
	barra   *fyne.Container
	barraEd *barraEd
	// estado es el pie con las palabras, las letras y el tiempo de lectura
	// (estado_nota.go). Nil: sin pie (Mail).
	estado *pieEstado
	// AutoformatoEnLinea convierte `**x**`, `*x*`, `_x_`, `` `x` `` y `~~x~~` en su
	// formato al cerrar la marca (autoformato.go). Lo enciende Notepads; el redactor de
	// Mail, que en la web no lo tiene, escribe las marcas tal cual.
	AutoformatoEnLinea bool
	// cambios cuenta los cambios del documento (lo escribe avisarCambio): el pie sabe así
	// si tiene que volver a contar.
	cambios int
	// barraTabla es la de las acciones de la tabla, que solo se ve en una celda.
	barraTabla *fyne.Container
	tablaEd    *barraEd
	// barraCodigo es la de un bloque de código, con su lenguaje (insertar.go).
	barraCodigo *fyne.Container
	codigoEd    *barraEd
	botones     map[string]*botonBarra
	bloqueBt    *botonBloque
	// Estirado es la tarjeta estirada: el cuerpo crece hasta abajo (`flex-1`).
	Estirado bool
	// SinAro quita el aro de foco del cuerpo (`sinAro` de RichText.tsx): en la hoja del
	// árbol y en la fila de lista o diario el editor es la página, y un recuadro de color
	// alrededor del texto la parte; el foco ya lo dicen el cursor y la barra abierta. En
	// una tarjeta (columnas, lienzo) el aro se queda.
	SinAro bool

	AlCambiar  func(html string)
	AlTerminar func()
	// conservaEdicion reconoce las herramientas del cuaderno que aloja este editor;
	// mover allí el foco conserva el documento y su selección, sin robarle el foco.
	conservaEdicion func(fyne.Focusable) bool
	// AlEnlazar abre la herramienta de enlaces colgando del botón; la pantalla sabe qué
	// notas ofrecer. Devuelve por `aplicarEnlace`.
	AlEnlazar func(disparador fyne.CanvasObject)
	// AlAdjuntar pide una imagen (true) o un fichero para ponerlos en el texto: quien
	// aloja el editor los guarda como adjuntos de la nota y vuelve con InsertarAdjunto
	// (adjuntos_cuerpo.go). Sin él (Mail), «Imagen» y «Archivo» no se ofrecen.
	AlAdjuntar func(imagen bool)
	// AlCrecer avisa de que el alto ha cambiado, para que quien la contiene recoloque.
	AlCrecer func()
	// Tecla y Atajo ven las teclas y las combinaciones ANTES que el editor y devuelven
	// true si se las quedan. Son de quien lo aloja fuera de Notepads: el redactor de
	// Sytue Mail envía con ⌘/Ctrl + Intro y cierra su diálogo con Escape, que aquí son
	// «terminar». Mismo contrato que `ui.EditorPlano`.
	Tecla func(*fyne.KeyEvent) bool
	Atajo func(fyne.Shortcut) bool
	// Imagenes da la imagen de un `src` para pintar los `<img>` del cuerpo (ver
	// `Documento.Imagenes`). Sytue Mail la da con las partes `cid:` de su mensaje.
	Imagenes func(src string) image.Image
	// AlMoverCursor avisa, tras cada cambio o movimiento, de dónde está el cursor dentro
	// de `Zona()`: la Y de arriba de la raya y su alto. Es para quien pone el cuerpo en
	// un desplazador con tope (el de 28rem de Mail) y tiene que tenerlo a la vista.
	AlMoverCursor func(arriba, alto float32)
	// Asistir es Sytue AI (`api.IA.Asistir`) e Idioma la lengua en que contesta; los pone
	// ConAsistente. Sin Asistir no hay asistente (asistente.go).
	Asistir func(context.Context, api.PeticionIA) (string, error)
	Idioma  string
	// AlAsistir avisa de cuándo empieza y acaba una petición al asistente, y de si ha
	// fallado: es para quien pone su propio botón (Mail), que lo apaga mientras tanto.
	AlAsistir func(trabajando, fallo bool)
	// TitulosEnNegrita escribe los títulos que devuelve el asistente como un párrafo en
	// negrita, que es lo que hace el redactor de Mail en la web: un correo no lleva títulos.
	TitulosEnNegrita bool
	// iaTrabajando es una petición en marcha, e iaFallo, que la última no ha vuelto;
	// aviso es su línea, debajo de la barra.
	iaTrabajando, iaFallo bool
	aviso                 fyne.CanvasObject
	// AlIrA abre «Ir a» (Ctrl/⌘+P o +O): con el cuerpo enfocado, el atajo llega aquí y no
	// al lienzo de la ventana.
	AlIrA func()

	// menu es el de «/», «[[» o «@» abierto (comandos.go) y descartada, el disparador al
	// que se dijo que no con Escape; burbuja, la de formato (burbuja.go); buscador,
	// «Buscar en la nota» (buscar_nota.go).
	menu       *menuComandos
	descartada *Pos
	burbuja    *flotante
	buscador   *buscadorNota
	// simbolos es el último selector de símbolos abierto (simbolos.go) e insertando, el
	// último menú «+» (insertar.go).
	simbolos   *selectorSimbolos
	insertando *ui.Emergente
	// enlaces es lo que ofrece «[[»: la búsqueda de la herramienta de enlaces, sin Drive.
	// Sin ella (Mail, una nota compartida conmigo), «[[» es texto.
	enlaces *busquedaEnlaces
	// gente es la de la organización, para «@»; `listo` avisa cuando llega la que se ha
	// pedido. Sin ella (un cuaderno cifrado, donde el servicio no lee a quién avisar;
	// Mail), «@» es una arroba.
	gente func(listo func()) []api.Miembro
	// portal es donde van el menú y la burbuja para que nada los tape (flotante.go); sin
	// él (Mail, las pruebas), van con el editor.
	portal *portal
	// tecladoAlto es lo que tapa el teclado del teléfono mientras se escribe: con él, la
	// barra de formato va pegada encima (barra_teclado.go).
	tecladoAlto float32
}

var _ fyne.Widget = (*EditorNota)(nil)

// NuevoEditorNota prepara el editor de un documento. El cursor queda al final.
func NuevoEditorNota(tr *i18n.Translator, d *Documento, marcaDeAgua string) *EditorNota {
	e := &EditorNota{tr: tr, doc: d.ParaEditar(), marca: marcaDeAgua}
	e.doc.rotuloExpandir = tr.T("editor.expand", nil)
	e.ExtendBaseWidget(e)
	e.zona = &zonaEdicion{e: e, encima: -1, llevando: -1, soltar: -1}
	e.zona.ExtendBaseWidget(e.zona)
	e.cur = e.fin()
	e.ancla = e.cur
	e.barra = e.armarBarra()
	e.barraTabla = e.armarBarraTabla()
	e.barraTabla.Hidden = !e.enTabla()
	e.barraCodigo = e.armarBarraCodigo()
	e.barraCodigo.Hidden = !e.enCodigo()
	e.burbuja = e.armarBurbuja()
	e.refrescarBarra()
	return e
}

// Zona es el cuerpo editable: lo que recibe el foco.
func (e *EditorNota) Zona() fyne.Focusable { return e.zona }

// HTML es lo que se guarda.
func (e *EditorNota) HTML() string { return e.doc.ParaGuardar() }

// Documento es el modelo que se está escribiendo.
func (e *EditorNota) Documento() *Documento { return e.doc }

// fin es la última posición escribible.
func (e *EditorNota) fin() Pos {
	for b := len(e.doc.Bloques) - 1; b >= 0; b-- {
		if editable(e.doc.Bloques[b]) {
			return Pos{b, largo(e.doc.Bloques[b])}
		}
	}
	return Pos{}
}

// editable dice si en un bloque se escribe: la regla y la tabla no.
func editable(b Bloque) bool { return b.Tipo != Regla && b.Tipo != BloqueOpaco }

// ---- Órdenes ----

// hacer aplica un cambio con su paso de deshacer y avisa.
func (e *EditorNota) hacer(f func()) {
	e.escribiendo = false
	e.guardarFoto()
	f()
	e.cambiado()
}

func (e *EditorNota) guardarFoto() {
	e.deshacer = append(e.deshacer, foto{e.doc.copia(), e.cur, e.ancla})
	if len(e.deshacer) > 200 {
		e.deshacer = e.deshacer[1:]
	}
	e.rehacer = nil
}

// rechazarCambio restores the last accepted HTML when the original session
// refuses a new edit. It cannot retain that unaccepted change in Undo/Redo.
func (e *EditorNota) rechazarCambio(html string) {
	e.doc = Leer(html).ParaEditar()
	e.doc.rotuloExpandir = e.tr.T("editor.expand", nil)
	e.cur, e.ancla = e.acotar(e.cur), e.acotar(e.ancla)
	e.deshacer, e.rehacer = nil, nil
	e.pendiente = nil
	e.repintar()
}

// cambiado es lo que sigue a todo cambio: acotar el cursor, avisar y repintar.
func (e *EditorNota) cambiado() {
	e.cur, e.ancla = e.acotar(e.cur), e.acotar(e.ancla)
	e.pendiente = nil
	e.avisarCambio()
	e.repintar()
}

// avisarCambio cuenta hacia fuera que el documento ha cambiado (para guardarlo) y, con
// «Buscar en la nota» abierta, rehace las coincidencias para que los velos sigan al texto.
func (e *EditorNota) avisarCambio() {
	e.cambios++
	e.rehacerHallados()
	llamar1(e.AlCambiar, e.HTML())
}

func (e *EditorNota) repintar() {
	antes, tabla, codigo := e.zona.Size().Height, e.barraTabla.Visible(), e.barraCodigo.Visible()
	e.zona.encenderCursor() // el cursor que se mueve o escribe se ve, como en el navegador
	e.refrescarBarra()
	e.zona.Refresh()
	cambia := e.enTabla() != tabla || e.enCodigo() != codigo
	if cambia {
		e.barraTabla.Hidden, e.barraCodigo.Hidden = !e.enTabla(), !e.enCodigo()
		e.Refresh()
	}
	if e.AlCrecer != nil && (e.zona.MinSize().Height != antes || cambia) {
		e.AlCrecer()
	}
	if e.AlMoverCursor != nil {
		_, arriba, alto := e.zona.cajaCursor()
		e.AlMoverCursor(PYZonaEd+arriba, alto)
	}
	e.mirarMenu()
	e.Refresh() // la burbuja sigue a la selección
}

// acotar deja una posición dentro del documento y en un bloque escribible.
func (e *EditorNota) acotar(p Pos) Pos {
	if len(e.doc.Bloques) == 0 {
		return Pos{}
	}
	p.B = min(max(p.B, 0), len(e.doc.Bloques)-1)
	if !editable(e.doc.Bloques[p.B]) {
		for b := p.B + 1; b < len(e.doc.Bloques); b++ {
			if editable(e.doc.Bloques[b]) {
				return Pos{b, inicioEditable(e.doc.Bloques[b])}
			}
		}
		for b := p.B - 1; b >= 0; b-- {
			if editable(e.doc.Bloques[b]) {
				return Pos{b, largo(e.doc.Bloques[b])}
			}
		}
		return p
	}
	bl := e.doc.Bloques[p.B]
	p.O = min(max(p.O, inicioEditable(bl)), largo(bl))
	return p
}

func (e *EditorNota) haySeleccion() bool { return e.cur != e.ancla }

// seleccion son los extremos en orden.
func (e *EditorNota) seleccion() (Pos, Pos) { return ordenar(e.ancla, e.cur) }

// escribir mete texto donde está el cursor, pisando lo seleccionado.
func (e *EditorNota) escribir(s string) {
	if !e.escribiendo || e.haySeleccion() {
		e.guardarFoto()
	}
	pendiente := e.pendiente
	if e.haySeleccion() {
		a, b := e.seleccion()
		e.cur = e.doc.Borrar(a, b)
	}
	if len(e.doc.Bloques) == 0 {
		e.doc.Bloques = []Bloque{{Tipo: Parrafo}}
	}
	estilo := estiloEn(e.doc.Bloques[e.cur.B], e.cur.O)
	if pendiente != nil {
		estilo = *pendiente
	}
	e.cur = e.doc.Insertar(e.cur, s, estilo)
	e.ancla = e.cur
	e.escribiendo = true
	e.hayX = false
	e.avisarCambio()
	e.pendiente = pendiente
	e.repintar()
}

// Orden aplica una orden de la barra por su nombre. Es la entrada única de la barra, de
// los atajos y de los guardianes.
//
//nolint:gocyclo // Es el único despachador de órdenes del editor.
func (e *EditorNota) Orden(orden, valor string) {
	a, b := e.seleccion()
	switch orden {
	case "undo":
		e.deshacerUno()
	case "redo":
		e.rehacerUno()
	case "b", "i", "u", "s", "code", "sub", "sup":
		if !e.haySeleccion() {
			// Sin selección, la marca es para lo que se escriba a continuación.
			t := estiloEn(e.bloqueActual(), e.cur.O)
			if e.pendiente != nil {
				t = *e.pendiente
			}
			t = conMarca(t, orden, !tieneMarca(t, orden))
			e.pendiente = &t
			e.refrescarBarra()
			return
		}
		e.hacer(func() { e.doc.AlternarMarca(a, b, orden) })
	case "mark":
		e.hacer(func() { e.doc.Resaltar(a, b, ColorResaltado) })
	case "resaltar": // el `hiliteColor` con el color de quien lo aloja (la paleta de Mail)
		e.hacer(func() { e.doc.Resaltar(a, b, valor) })
	case "color":
		e.hacer(func() { e.doc.Pintar(a, b, valor) })
	case "fuente", "tamano":
		// `fontName` y `fontSize` de la barra de Mail: sin selección, para lo que se
		// escriba a continuación, como una marca.
		poner := func(t *Tramo) {
			if orden == "fuente" {
				t.Fuente = valor
			} else {
				t.Tamano = valor
			}
			t.EnSpan = t.EnSpan || valor != ""
		}
		if !e.haySeleccion() {
			t := estiloEn(e.bloqueActual(), e.cur.O)
			if e.pendiente != nil {
				t = *e.pendiente
			}
			poner(&t)
			e.pendiente = &t
			return
		}
		e.hacer(func() { e.doc.tramos(a, b, poner) })
	case "imagen":
		// El `<img>` de la barra de Mail, con el `src` que dé quien la aloja (`cid:` o
		// `https:`), pisando lo seleccionado.
		e.hacer(func() {
			if e.haySeleccion() {
				e.cur = e.doc.Borrar(a, b)
			}
			e.cur = e.doc.InsertarImagen(e.cur, valor)
			e.ancla = e.cur
		})
	case "bloque":
		e.hacer(func() { e.doc.Formatear(a, b, valor) })
	case "vinetas":
		e.hacer(func() { e.doc.AlternarLista(a, b, Vineta) })
	case "numeros":
		e.hacer(func() { e.doc.AlternarLista(a, b, Numerada) })
	case "sangrar":
		e.hacer(func() { e.doc.Sangrar(a, b, 1) })
	case "desangrar":
		e.hacer(func() { e.doc.Sangrar(a, b, -1) })
	case "alinear":
		e.hacer(func() { e.doc.Alinear(a, b, valor) })
	case "tarea":
		e.hacer(func() { e.cur = e.doc.PonerTarea(e.cur); e.ancla = e.cur })
	case "regla":
		e.hacer(func() { e.cur = e.doc.Regla(e.cur); e.ancla = e.cur })
	case "limpiar":
		e.hacer(func() { e.doc.QuitarFormato(a, b) })
	case "tabla":
		// `insertarTabla(3, 3)`: la de partida de cualquier editor, que crece con la
		// barra de tabla.
		if e.bloqueActual().Celda.EnTabla() {
			return
		}
		e.hacer(func() {
			if e.haySeleccion() {
				e.cur = e.doc.Borrar(a, b)
			}
			e.cur = e.doc.InsertarTabla(e.cur, 3, 3)
			e.ancla = e.cur
		})
	case "filaEncima", "filaDebajo", "colAntes", "colDespues", "borrarFila", "borrarCol", "borrarTabla":
		if !e.enTabla() {
			return
		}
		e.hacer(func() { e.cur = e.doc.EditarTabla(orden, e.cur); e.ancla = e.cur })
	default:
		e.ordenDeBloque(orden, valor)
	}
}

// ordenDeBloque son las órdenes de la barra de tabla que cambian su forma (cabeceras,
// fondo, filas numeradas, combinar) y las de la barra del código: cada una solo donde
// vale, en su paso de deshacer.
func (e *EditorNota) ordenDeBloque(orden, valor string) {
	switch orden {
	case "cabeceraFila", "cabeceraCol", "fondoCelda", "numerada":
		if e.enTabla() {
			e.hacer(func() { e.doc.FormatearTabla(orden, valor, e.cur) })
		}
	case "combinarDerecha", "combinarAbajo", "separarCelda":
		if e.doc.PuedeCombinar(orden, e.cur) {
			e.hacer(func() { e.cur = e.doc.CombinarCeldas(orden, e.cur); e.ancla = e.cur })
		}
	case "lineas", "ajuste":
		if e.enCodigo() {
			e.hacer(func() { e.doc.AlternarCodigo(e.cur.B, orden) })
		}
	}
}

// Enlazar es aplicarEnlace para quien trae su propio panel de enlace (el redactor de
// Mail, que pide rótulo y dirección como su web).
func (e *EditorNota) Enlazar(url, rotulo string) { e.aplicarEnlace(url, rotulo) }

// Seleccionado es el texto seleccionado, en plano: el rótulo que propone ese panel.
func (e *EditorNota) Seleccionado() string {
	if !e.haySeleccion() {
		return ""
	}
	a, b := e.seleccion()
	return e.doc.TextoEntre(a, b)
}

// aplicarEnlace es la vuelta de la herramienta de enlaces: con texto seleccionado, ese
// texto es el enlace; sin él, se inserta el nombre de lo enlazado y un espacio duro.
func (e *EditorNota) aplicarEnlace(url, rotulo string) {
	a, b := e.seleccion()
	e.hacer(func() {
		if a != b {
			e.doc.Enlazar(a, b, url)
			return
		}
		e.escribirEnlace(url, rotulo)
	})
}

// escribirEnlace pone el enlace con su nombre donde está el cursor, y un espacio duro
// para seguir escribiendo fuera de él (`escribirEnlace` de RichText.tsx). Es lo que
// escriben la herramienta de enlaces sin selección y el `[[` (comandos.go).
func (e *EditorNota) escribirEnlace(url, rotulo string) {
	if rotulo == "" {
		rotulo = url
	}
	estilo := estiloEn(e.bloqueActualDe(e.cur), e.cur.O)
	inicio := e.cur
	fin := e.doc.Insertar(e.cur, rotulo, estilo)
	e.doc.Enlazar(inicio, fin, url)
	e.cur = e.doc.Insertar(fin, "\u00a0", estilo)
	e.ancla = e.cur
}

func (e *EditorNota) bloqueActual() Bloque { return e.bloqueActualDe(e.cur) }

func (e *EditorNota) bloqueActualDe(p Pos) Bloque {
	if p.B < len(e.doc.Bloques) {
		return e.doc.Bloques[p.B]
	}
	return Bloque{Tipo: Parrafo}
}

func (e *EditorNota) deshacerUno() {
	if len(e.deshacer) == 0 {
		return
	}
	f := e.deshacer[len(e.deshacer)-1]
	e.deshacer = e.deshacer[:len(e.deshacer)-1]
	e.rehacer = append(e.rehacer, foto{e.doc.copia(), e.cur, e.ancla})
	e.doc, e.cur, e.ancla = f.doc, f.cur, f.ancla
	e.escribiendo = false
	e.avisarCambio()
	e.repintar()
}

func (e *EditorNota) rehacerUno() {
	if len(e.rehacer) == 0 {
		return
	}
	f := e.rehacer[len(e.rehacer)-1]
	e.rehacer = e.rehacer[:len(e.rehacer)-1]
	e.deshacer = append(e.deshacer, foto{e.doc.copia(), e.cur, e.ancla})
	e.doc, e.cur, e.ancla = f.doc, f.cur, f.ancla
	e.escribiendo = false
	e.avisarCambio()
	e.repintar()
}

// estadoMarca dice si la marca está en el cursor (o en lo que se va a escribir).
// Activa dice si una orden de la barra está encendida donde está el cursor —negrita,
// una lista, una alineación—: es el `aria-pressed` de la barra de quien lo aloja.
func (e *EditorNota) Activa(orden, valor string) bool {
	switch orden {
	case "vinetas":
		return len(e.doc.Bloques) > 0 && e.bloqueActual().Tipo == Vineta
	case "numeros":
		return len(e.doc.Bloques) > 0 && e.bloqueActual().Tipo == Numerada
	case "alinear":
		a := ""
		if len(e.doc.Bloques) > 0 {
			a = e.bloqueActual().Alineacion
		}
		return a == valor || (a == "" && valor == "left")
	case "tabla": // el cursor en una celda: la barra de tabla de quien lo aloja
		return e.enTabla()
	}
	return e.estadoMarca(orden)
}

// PegarTexto pega texto plano donde está el cursor, pisando lo seleccionado, en un solo
// paso de deshacer.
func (e *EditorNota) PegarTexto(t string) {
	if t == "" {
		return
	}
	e.escribiendo = false
	e.escribir(strings.ReplaceAll(t, "\r\n", "\n"))
	e.escribiendo = false
}

// PegarConMarkdown es el ⌘V de una nota: el texto que parece Markdown entra con su
// formato (títulos, listas, tareas…) y el resto, como texto plano. En un bloque de
// código el Markdown se queda en letras, como al teclear (`lineaAntes`) y como en la web.
func (e *EditorNota) PegarConMarkdown(t string) {
	if e.bloqueActual().Tipo == Codigo {
		e.PegarTexto(t)
		return
	}
	if d := documentoDePegado(t); d != nil {
		e.Pegar(d)
		return
	}
	e.PegarTexto(t)
}

// Pegar pega un documento con su formato donde está el cursor, pisando lo seleccionado,
// en un solo paso de deshacer. Es para quien pega HTML (el redactor de Mail) y para el
// Markdown pegado en una nota (`PegarConMarkdown`).
func (e *EditorNota) Pegar(d *Documento) {
	if d.Vacio() {
		return
	}
	e.hacer(func() {
		if e.haySeleccion() {
			a, b := e.seleccion()
			e.cur = e.doc.Borrar(a, b)
		}
		e.cur = e.doc.InsertarDocumento(e.cur, d)
		e.ancla = e.cur
	})
}

// Mayus dice si Mayúsculas está pulsada: quien ve el tabulador con Tecla sabe así si
// va hacia atrás.
func (e *EditorNota) Mayus() bool { return e.shift }

// AlPrincipio pone el cursor al principio: una respuesta se escribe ENCIMA de la cita.
func (e *EditorNota) AlPrincipio() {
	e.cur = e.acotar(Pos{})
	e.ancla = e.cur
	e.repintar()
}

func (e *EditorNota) estadoMarca(m string) bool {
	if e.pendiente != nil {
		return tieneMarca(*e.pendiente, m)
	}
	if len(e.doc.Bloques) == 0 {
		return false
	}
	return tieneMarca(estiloEn(e.bloqueActual(), e.cur.O), m)
}

// ---- Teclado ----

// tecla atiende una tecla del cuerpo.
func (e *EditorNota) tecla(k fyne.KeyName) {
	if e.teclaMenu(k) {
		return
	}
	switch k {
	case fyne.KeyLeft, fyne.KeyRight:
		e.moverHorizontal(k == fyne.KeyRight)
	case fyne.KeyUp, fyne.KeyDown:
		e.moverVertical(k == fyne.KeyDown)
	case fyne.KeyHome, fyne.KeyEnd:
		e.moverExtremo(k == fyne.KeyEnd)
	case fyne.KeyBackspace, fyne.KeyDelete:
		e.hacer(func() {
			switch {
			case e.haySeleccion():
				a, b := e.seleccion()
				e.cur = e.doc.Borrar(a, b)
			case k == fyne.KeyBackspace:
				e.cur = e.doc.Retroceso(e.cur)
			default:
				e.cur = e.doc.Suprimir(e.cur)
			}
			e.ancla = e.cur
		})
	case fyne.KeyReturn, fyne.KeyEnter:
		e.intro()
	case fyne.KeyTab:
		switch {
		case e.enTabla():
			e.irACelda(e.shift) // en una tabla, de celda en celda (tabla_barra.go)
		case e.shift:
			e.Orden("desangrar", "")
		default:
			e.Orden("sangrar", "")
		}
	case fyne.KeyEscape:
		llamar0(e.AlTerminar)
	default:
	}
}

// intro es Intro: Mayús+Intro es un salto dentro del bloque; «---» y Intro, una regla; y si
// no, partir el bloque.
func (e *EditorNota) intro() {
	if e.shift {
		e.escribiendo = false
		e.escribir("\n")
		e.escribiendo = false
		return
	}
	if orden, _, borrar, ok := atajoMarkdown(e.lineaAntes(), true); ok && orden == "regla" {
		e.hacer(func() {
			inicio := Pos{e.cur.B, e.cur.O - borrar}
			e.cur = e.doc.Borrar(inicio, e.cur)
			e.cur = e.doc.Regla(e.cur)
			e.ancla = e.cur
		})
		return
	}
	e.hacer(func() {
		if e.haySeleccion() {
			a, b := e.seleccion()
			e.cur = e.doc.Borrar(a, b)
		}
		e.cur = e.doc.Partir(e.cur)
		e.ancla = e.cur
	})
}

// lineaAntes es el texto del bloque antes del cursor, si el bloque admite atajos: no en
// un bloque de código ni en un elemento de lista (`aplicarAtajoMarkdown`).
func (e *EditorNota) lineaAntes() string {
	if len(e.doc.Bloques) == 0 {
		return ""
	}
	b := e.bloqueActual()
	if b.Tipo == Codigo || esLista(b.Tipo) || b.Tipo == Tarea {
		return ""
	}
	r := []rune(b.Texto())
	return string(r[:min(e.cur.O, len(r))])
}

// espacio es la barra espaciadora: dispara los atajos de Markdown al principio de la
// línea; si no, es una letra más.
func (e *EditorNota) espacio() {
	orden, valor, borrar, ok := atajoMarkdown(e.lineaAntes(), false)
	if !ok || e.haySeleccion() || orden == "regla" {
		e.escribir(" ")
		return
	}
	// El espacio se escribe primero, como cualquier letra, y la conversión es su PROPIO
	// paso de deshacer: Ctrl/⌘+Z justo después devuelve «## » tal cual (autoformato.go).
	e.escribiendo = false
	e.escribir(" ")
	e.hacer(func() {
		inicio := Pos{e.cur.B, e.cur.O - borrar - 1}
		e.cur = e.doc.Borrar(inicio, e.cur)
		e.ancla = e.cur
		switch orden {
		case "vinetas":
			e.doc.AlternarLista(e.cur, e.cur, Vineta)
		case "numeros":
			e.doc.AlternarLista(e.cur, e.cur, Numerada)
		case "bloque":
			e.doc.Formatear(e.cur, e.cur, valor)
		case "tarea":
			e.cur = e.doc.PonerTarea(e.cur)
			e.ancla = e.cur
		}
	})
}

func (e *EditorNota) moverA(p Pos) {
	if !e.shift {
		e.ancla = p
	}
	e.cur = p
	e.pendiente = nil
	e.escribiendo = false
	e.repintar()
}

func (e *EditorNota) moverHorizontal(adelante bool) {
	e.hayX = false
	if e.haySeleccion() && !e.shift {
		a, b := e.seleccion()
		if adelante {
			e.moverA(b)
		} else {
			e.moverA(a)
		}
		return
	}
	p := e.cur
	if len(e.doc.Bloques) == 0 {
		return
	}
	bl := e.doc.Bloques[p.B]
	switch {
	case adelante && p.O < largo(bl):
		p.O++
	case !adelante && p.O > inicioEditable(bl):
		p.O--
	case adelante:
		for b := p.B + 1; b < len(e.doc.Bloques); b++ {
			if editable(e.doc.Bloques[b]) {
				p = Pos{b, inicioEditable(e.doc.Bloques[b])}
				break
			}
		}
	default:
		for b := p.B - 1; b >= 0; b-- {
			if editable(e.doc.Bloques[b]) {
				p = Pos{b, largo(e.doc.Bloques[b])}
				break
			}
		}
	}
	e.moverA(p)
}

// lineaDe es el índice de la línea maquetada donde está una posición.
func (e *EditorNota) lineaDe(m *Maquetado, p Pos) int {
	mejor := -1
	for i, l := range m.Lineas {
		if l.Bloque != p.B {
			continue
		}
		if mejor < 0 || (p.O >= l.Desde && (p.O < l.Hasta || i == len(m.Lineas)-1 || m.Lineas[i+1].Bloque != p.B)) {
			mejor = i
			if p.O >= l.Desde && p.O < l.Hasta {
				return i
			}
		}
	}
	return mejor
}

func (e *EditorNota) moverVertical(abajo bool) {
	m := e.zona.maquetado()
	i := e.lineaDe(m, e.cur)
	if i < 0 {
		return
	}
	if !e.hayX {
		e.xDeseada, e.hayX = m.Lineas[i].XDe(e.cur.O), true
	}
	j := lineaVecina(m, e.doc.Bloques, i, e.xDeseada, abajo)
	if j < 0 {
		x := e.xDeseada
		e.moverExtremo(abajo)
		e.xDeseada, e.hayX = x, true
		return
	}
	l := m.Lineas[j]
	x := e.xDeseada
	e.moverA(e.acotar(Pos{l.Bloque, l.PosEn(x)}))
	e.xDeseada, e.hayX = x, true
}

// moverExtremo es Inicio y Fin: el principio o el final de la LÍNEA.
func (e *EditorNota) moverExtremo(fin bool) {
	m := e.zona.maquetado()
	i := e.lineaDe(m, e.cur)
	if i < 0 {
		return
	}
	l := m.Lineas[i]
	if fin {
		e.moverA(e.acotar(Pos{l.Bloque, l.Hasta}))
		return
	}
	e.moverA(e.acotar(Pos{l.Bloque, l.Desde}))
}

// atajo atiende las combinaciones con Ctrl (Cmd en macOS).
func (e *EditorNota) atajo(s fyne.Shortcut) {
	switch x := s.(type) {
	case *fyne.ShortcutCopy:
		e.copiar(x.Clipboard)
	case *fyne.ShortcutCut:
		if e.copiar(x.Clipboard) {
			e.tecla(fyne.KeyDelete)
		}
	case *fyne.ShortcutPaste:
		// Ver la cabecera de RichText.tsx: el portapapeles entra como TEXTO PLANO, salvo
		// que parezca Markdown, que llega con su formato (markdown.go). Ctrl+Mayús+V,
		// en atajoPropio, lo pega plano siempre.
		if x.Clipboard != nil {
			e.PegarConMarkdown(x.Clipboard.Content())
		}
	case *fyne.ShortcutSelectAll:
		if len(e.doc.Bloques) > 0 {
			e.ancla, e.cur = e.acotar(Pos{0, 0}), e.fin()
			e.repintar()
		}
	case *fyne.ShortcutUndo:
		e.deshacerUno()
	case *fyne.ShortcutRedo:
		e.rehacerUno()
	case *desktop.CustomShortcut:
		if !e.pegarSinFormato(x) {
			e.atajoPropio(x)
		}
	}
}

func (e *EditorNota) copiar(c fyne.Clipboard) bool {
	if !e.haySeleccion() || c == nil {
		return false
	}
	a, b := e.seleccion()
	c.SetContent(e.doc.TextoEntre(a, b))
	return true
}

// pegarSinFormato es Ctrl/⌘+Mayús+V: pegar como texto plano, también el Markdown, como
// en la web y en el redactor de Mail. Dice si era ese atajo.
func (e *EditorNota) pegarSinFormato(x *desktop.CustomShortcut) bool {
	if x.KeyName != fyne.KeyV || x.Modifier&fyne.KeyModifierShift == 0 ||
		x.Modifier&fyne.KeyModifierShortcutDefault == 0 || x.Modifier&fyne.KeyModifierAlt != 0 {
		return false
	}
	if a := fyne.CurrentApp(); a != nil {
		e.PegarTexto(a.Clipboard().Content())
	}
	return true
}

func (e *EditorNota) atajoPropio(x *desktop.CustomShortcut) {
	ctrl := x.Modifier&fyne.KeyModifierShortcutDefault != 0
	if !ctrl {
		return
	}
	if x.Modifier&fyne.KeyModifierAlt != 0 {
		// Ctrl+Alt+0 párrafo, Ctrl+Alt+1…6 títulos (`ATAJO_BLOQUE` de la web).
		if n := string(x.KeyName); len(n) == 1 && n[0] >= '0' && n[0] <= '6' {
			valor := "p"
			if n != "0" {
				valor = "h" + n
			}
			e.Orden("bloque", valor)
		}
		return
	}
	switch x.KeyName {
	case fyne.KeyF:
		e.abrirBuscador()
	case fyne.KeyPeriod:
		e.alternarPlegado()
	case fyne.KeyP, fyne.KeyO:
		llamar0(e.AlIrA)
	case fyne.KeyB:
		e.Orden("b", "")
	case fyne.KeyI:
		e.Orden("i", "")
	case fyne.KeyU:
		e.Orden("u", "")
	case fyne.KeyK:
		llamar1(e.AlEnlazar, e.disparadorDe("enlace"))
	case fyne.KeySlash:
		e.verAtajos()
	case fyne.KeyH:
		e.abrirReemplazar()
	case fyne.KeyReturn, fyne.KeyEnter:
		llamar0(e.AlTerminar)
	case fyne.KeyY:
		e.rehacerUno()
	case fyne.KeyZ:
		if x.Modifier&fyne.KeyModifierShift != 0 {
			e.rehacerUno()
		}
	default:
	}
}

// ---- La barra ----

// botonDeOrden devuelve el foco a la escritura después de una acción directa. Los
// disparadores de menús sustituyen AlPulsar: su capa conserva el foco hasta cerrarse.
func (e *EditorNota) botonDeOrden(glifo string, icono iconos.Icono, rotulo string, aplicar func()) *botonBarra {
	return nuevoBotonBarra(glifo, icono, rotulo, func() {
		llamar0(aplicar)
		e.reenfocar()
	})
}

// enTabla dice si el cursor está en una celda: es lo que enseña la barra de tabla.
func (e *EditorNota) enTabla() bool {
	return len(e.doc.Bloques) > 0 && e.bloqueActual().Celda.EnTabla() && !e.bloqueActual().Celda.Tabla.EsColumnas()
}

// barras son las barras que se ven, en su orden: la de formato y, con el cursor en una
// celda, la de tabla.
func (e *EditorNota) barras() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if !e.enTeclado() {
		out = append(out, e.barra) // si no, va pegada al teclado (barra_teclado.go)
	}
	if e.buscador != nil {
		out = append(out, e.buscador.barra)
	}
	if e.iaFallo && e.aviso != nil {
		out = append(out, e.aviso) // encima de la de tabla, como en la web
	}
	if e.enTabla() {
		out = append(out, e.barraTabla)
	}
	if e.enCodigo() {
		out = append(out, e.barraCodigo)
	}
	return out
}

// refrescarBarra pone el estado de cada botón: las marcas y el tipo de bloque del cursor,
// y deshacer y rehacer apagados si no hay nada.
func (e *EditorNota) refrescarBarra() {
	if e.botones == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.barraEd.fondo.FillColor = tema.Color("surface", v)
	e.barraEd.filete.FillColor = tema.Color("border", v)
	for _, b := range []*barraEd{e.tablaEd, e.codigoEd} {
		b.fondo.FillColor, b.filete.FillColor = tema.Color("surface", v), tema.Color("border", v)
	}
	bl := Bloque{Tipo: Parrafo}
	if len(e.doc.Bloques) > 0 {
		bl = e.bloqueActual()
	}
	estados := map[string]bool{
		"b": e.estadoMarca("b"), "i": e.estadoMarca("i"), "u": e.estadoMarca("u"), "s": e.estadoMarca("s"),
		"code": e.estadoMarca("code"), "tarea": bl.Tipo == Tarea, "buscar": e.buscador != nil,
		"vinetas": bl.Tipo == Vineta && bl.Lista == "", "numeros": bl.Tipo == Numerada,
		"cabeceraFila": bl.Celda.EnTabla() && cabeceraDeFila(bl.Celda.Tabla),
		"cabeceraCol":  bl.Celda.EnTabla() && cabeceraDeColumna(bl.Celda.Tabla),
		"numerada":     bl.Celda.EnTabla() && numerada(bl.Celda.Tabla),
		"lineas":       bl.Tipo == Codigo && bl.Lineas,
		"ajuste":       bl.Tipo == Codigo && bl.Ajuste,
		"left":         bl.Alineacion == "left", "center": bl.Alineacion == "center", "right": bl.Alineacion == "right",
	}
	for clave, b := range e.botones {
		activo, conEstado := estados[clave]
		b.ConEstado, b.Activo = conEstado, activo
		switch clave {
		case "undo":
			b.Inerte = len(e.deshacer) == 0
		case "redo":
			b.Inerte = len(e.rehacer) == 0
		case "tabla":
			b.Inerte = bl.Celda.EnTabla() // una tabla dentro de otra no se edita (documento.go)
		case "combinarDerecha", "combinarAbajo", "separarCelda":
			b.Inerte = !e.doc.PuedeCombinar(clave, e.cur)
		case "ia":
			b.Inerte = e.iaTrabajando
		}
		b.Refresh()
	}
	etiqueta := EtiquetaDe(bl)
	for _, x := range BloquesEd {
		if x.Valor == etiqueta {
			e.bloqueBt.Texto = e.tr.T(x.Clave, nil)
		}
	}
	e.bloqueBt.Refresh()
	e.barra.Refresh()
	e.barraTabla.Refresh()
	e.refrescarBarraCodigo()
	e.refrescarEstado()
}

// menuBloque abre el desplegable de «Estilo del texto» con los nueve, el actual marcado,
// colgando de `disparador` (su botón o «Más»).
func (e *EditorNota) menuBloque(disparador fyne.CanvasObject) {
	actual := "p"
	if len(e.doc.Bloques) > 0 {
		actual = EtiquetaDe(e.bloqueActual())
	}
	elementos := make([]fyne.CanvasObject, 0, len(BloquesEd))
	for _, x := range BloquesEd {
		it := ui.NuevoElementoMenu(e.tr.T(x.Clave, nil), iconos.Ninguno, func() {
			e.Orden("bloque", x.Valor)
			e.reenfocar()
		})
		it.Marcada = x.Valor == actual
		elementos = append(elementos, it)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = e.tr.T("editor.block", nil)
	m.Mostrar(disparador)
}

// paletaColor abre la paleta de «Color del texto»: el color por defecto y los ocho.
func (e *EditorNota) paletaColor(disparador fyne.CanvasObject) {
	var em *ui.Emergente
	elegir := func(c string) {
		e.Orden("color", c)
		if em != nil {
			em.Cerrar()
		}
		e.reenfocar()
	}
	defecto := NuevaOpcion(e.tr.T("editor.colorDefault", nil),
		lineaTok(e.tr.T("editor.colorDefault", nil), tema.TextoSM, ui.Normal, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant())),
		func() { elegir("") })
	defecto.PX, defecto.PY, defecto.Borde, defecto.Superficie = 2*0.25*tema.Rem, 1*0.25*tema.Rem, true, "surface"
	muestras := make([]fyne.CanvasObject, 0, len(ColoresTexto))
	for _, c := range ColoresTexto {
		muestras = append(muestras, NuevaMuestra(hexO(c, color.NRGBA{}), LadoBotonEd, 0, c, func() { elegir(c) }))
	}
	// `grid grid-cols-4 gap-1.5 p-2`: «por defecto» ocupa la fila y debajo los ocho.
	contenido := container.New(ui.Columna{Hueco: 1.5 * 0.25 * tema.Rem}, defecto,
		container.New(ui.Rejilla{Fijas: 4, Hueco: 1.5 * 0.25 * tema.Rem}, muestras...))
	// Es un `<Popover>` de la web (RichText.tsx): `surface-raised`, `p-4` y a 6 del
	// disparador, con la rejilla y su propio `p-2` dentro.
	p2 := tema.Espacio(2)
	em = ui.MostrarPopover(disparador, container.New(ui.Margen{Izq: p2, Arr: p2, Der: p2, Abj: p2}, contenido))
}

// reenfocar devuelve el foco al cuerpo después de usar la barra o un menú.
func (e *EditorNota) reenfocar() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil {
		c.Focus(e.zona)
	}
}

// AltoPara es lo que ocupa a ese ancho: la barra (que envuelve) y el cuerpo.
func (e *EditorNota) AltoPara(ancho float32) float32 {
	if ancho <= 0 {
		ancho = 1e6 // sin colocar todavía: sin repartir (ver `anchoUtil`)
	}
	var alto float32
	for _, b := range e.barras() {
		alto += altoDe(b, ancho)
	}
	if e.estado != nil {
		alto += altoPie
	}
	return alto + e.zona.altoPara(ancho)
}

// CreateRenderer: la barra arriba y el cuerpo debajo.
func (e *EditorNota) CreateRenderer() fyne.WidgetRenderer {
	return &editorRenderer{e: e}
}

type editorRenderer struct{ e *EditorNota }

func (r *editorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.e.AltoPara(r.e.Size().Width))
}

func (r *editorRenderer) Objects() []fyne.CanvasObject {
	objs := append(r.e.barras(), r.e.zona)
	if r.e.estado != nil {
		objs = append(objs, r.e.estado)
	}
	if r.e.portal == nil {
		objs = append(objs, r.e.flotantes()...)
	}
	return objs
}
func (r *editorRenderer) Destroy() {}

func (r *editorRenderer) Layout(t fyne.Size) {
	var alto float32
	for _, b := range r.e.barras() {
		h := altoDe(b, t.Width)
		b.Move(fyne.NewPos(0, alto))
		b.Resize(fyne.NewSize(t.Width, h))
		alto += h
	}
	pie := float32(0)
	if r.e.estado != nil {
		pie = altoPie
	}
	r.e.zona.Move(fyne.NewPos(0, alto))
	r.e.zona.Resize(fyne.NewSize(t.Width, max(t.Height-alto-pie, r.e.zona.altoPara(t.Width))))
	if r.e.estado != nil {
		r.e.estado.Move(fyne.NewPos(0, alto+r.e.zona.Size().Height))
		r.e.estado.Resize(fyne.NewSize(t.Width, pie))
	}
	if r.e.portal == nil {
		r.e.colocarFlotantes(r.e.zona.Position(), t.Width, 0)
		return
	}
	off := r.e.portal.desde(r.e)
	r.e.colocarFlotantes(r.e.zona.Position().Add(off), r.e.portal.c.Size().Width, r.e.portal.c.Size().Height)
	flotan := r.e.flotantes()
	if r.e.enTeclado() {
		r.e.colocarEnTeclado()
		flotan = append(flotan, r.e.barra)
	}
	r.e.portal.poner(r.e, flotan...)
}

func (r *editorRenderer) Refresh() {
	r.e.refrescarBarra()
	r.e.zona.Refresh()
	r.Layout(r.e.Size()) // la barra de tabla entra y sale con el cursor
}

// ---- El cuerpo ----

// zonaEdicion es el cuerpo editable: `sytue-prose px-3 py-2 min-h-24 leading-relaxed` con
// el anillo `ring-2 ring-primary/40` al tener el foco.
type zonaEdicion struct {
	widget.BaseWidget
	e        *EditorNota
	enfocado bool
	maq      *Maquetado
	ancho    float32
	arrastre bool
	// encima es el bloque bajo el ratón, que enseña su asa (bloque_menu.go); -1, ninguno.
	encima int
	// pararTeclado deja de mirar el teclado del teléfono (barra_teclado.go).
	pararTeclado chan struct{}
	// llevando es el bloque que se arrastra por su asa, y soltar e ySoltar, delante de qué
	// bloque caería y la altura de su raya (arrastre_bloque.go); -1, ninguno.
	llevando, soltar int
	ySoltar          float32
	// contexto es el doble toque y el menú contextual (editor_menu.go).
	contexto contextoZona
	// El parpadeo del cursor (cursor_parpadeo.go): la raya pintada, si está en su fase
	// apagada, desde cuándo cuenta, la animación que lo mueve y el reloj de las pruebas.
	raya     *canvas.Rectangle
	apagado  bool
	desde    time.Time
	parpadeo *fyne.Animation
	ahora    func() time.Time
}

var (
	_ fyne.Focusable     = (*zonaEdicion)(nil)
	_ fyne.Tappable      = (*zonaEdicion)(nil)
	_ fyne.Draggable     = (*zonaEdicion)(nil)
	_ fyne.Shortcutable  = (*zonaEdicion)(nil)
	_ fyne.Tabbable      = (*zonaEdicion)(nil)
	_ desktop.Keyable    = (*zonaEdicion)(nil)
	_ desktop.Cursorable = (*zonaEdicion)(nil)
	_ desktop.Mouseable  = (*zonaEdicion)(nil)
)

// AccessibilityLabel es el título de la nota, como el `aria-label` del `role="textbox"`.
func (z *zonaEdicion) AccessibilityLabel() string { return z.e.marca }

// AccessibilityRole: se lee y se escribe como texto.
func (z *zonaEdicion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (z *zonaEdicion) altoPara(ancho float32) float32 {
	return max(z.maquetar(ancho, tema.Claro).Alto+2*PYZonaEd, MinAltoZonaEd)
}

// maquetar es el cuerpo a ese ancho, con las imágenes de quien lo aloja.
func (z *zonaEdicion) maquetar(ancho float32, v tema.Variante) *Maquetado {
	if z.e.Imagenes != nil {
		z.e.doc.Imagenes = z.e.Imagenes
	}
	return Maquetar(z.e.doc, anchoUtil(ancho), v)
}

// maquetado es el de la última colocación, o uno nuevo al ancho que tenga.
func (z *zonaEdicion) maquetado() *Maquetado {
	if z.maq == nil {
		z.maq = z.maquetar(z.Size().Width, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	return z.maq
}

// anchoUtil es el ancho de escribir dentro del `px-3`. Sin colocar todavía (ancho 0), sin
// tope: repartir en una columna de un píxel pondría cada letra en su línea.
func anchoUtil(ancho float32) float32 {
	if ancho <= 0 {
		return 1e6
	}
	return max(ancho-2*PXZonaEd, 1)
}

// Cursor es el de texto.
func (z *zonaEdicion) Cursor() desktop.Cursor { return desktop.TextCursor }

// AcceptsTab: el tabulador es sangrar, no salir del cuerpo.
func (z *zonaEdicion) AcceptsTab() bool { return true }

func (z *zonaEdicion) FocusGained() {
	z.enfocado = true
	z.vigilarTeclado()
	z.encenderCursor()
	z.Refresh()
	z.e.Refresh()
}

// FocusLost: salir del cuerpo guarda y cierra el editor —se salga como se salga—, salvo
// que el foco se haya ido a un menú o a una paleta de la barra (una capa encima) o a
// ningún sitio (un clic en el lienzo vacío), que es lo que decide `onBlurCapture` en la
// web. Se decide DESPUÉS, cuando el foco ya ha caído en su sitio.
func (z *zonaEdicion) FocusLost() {
	z.enfocado = false
	z.dejarTeclado()
	z.pararParpadeo()
	z.Refresh()
	z.e.Refresh() // la burbuja solo se ve con el cuerpo enfocado
	fyne.Do(z.comprobarSalida)
}

func (z *zonaEdicion) comprobarSalida() {
	c := fyne.CurrentApp().Driver().CanvasForObject(z)
	if c == nil {
		return
	}
	// Con una capa abierta (el menú de bloque, la paleta, la herramienta de enlaces)
	// el foco está en ella y vuelve aquí al cerrarla: no es haberse ido.
	f := c.Focused()
	if f == nil || f == fyne.Focusable(z) || len(c.Overlays().List()) > 0 {
		return
	}
	// La caja de «Buscar en la nota» y las herramientas del cuaderno conservan la
	// sesión sin robar el foco a la herramienta que la persona está usando.
	if z.e.buscador != nil && z.e.buscador.conservaFoco(f) {
		return
	}
	if z.e.conservaEdicion != nil && z.e.conservaEdicion(f) {
		return
	}
	llamar0(z.e.AlTerminar)
}

func (z *zonaEdicion) TypedRune(r rune) {
	z.encenderCursor()
	if r == ' ' {
		z.e.espacio()
		return
	}
	z.e.escribir(string(r))
	if esLetraDeMarca(r) {
		z.e.autoformatoEnLinea()
	}
}

func (z *zonaEdicion) TypedKey(k *fyne.KeyEvent) {
	z.encenderCursor() // también una flecha contra el borde, que no mueve nada
	// Escape es del editor (`AlTerminar`, en `tecla`), no del diálogo de encima.
	if z.e.Tecla != nil && z.e.Tecla(k) {
		return
	}
	z.e.tecla(k.Name)
}

func (z *zonaEdicion) TypedShortcut(s fyne.Shortcut) {
	z.encenderCursor()
	if z.e.Atajo != nil && z.e.Atajo(s) {
		return
	}
	z.e.atajo(s)
}

// KeyDown y KeyUp llevan la cuenta de Mayús, que es lo que convierte mover en seleccionar.
func (z *zonaEdicion) KeyDown(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		z.e.shift = true
	}
}

func (z *zonaEdicion) KeyUp(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		z.e.shift = false
	}
}

// posEn es la letra bajo un punto del cuerpo.
func (z *zonaEdicion) posEn(p fyne.Position) (Pos, bool) {
	m := z.maquetado()
	x, y := p.X-PXZonaEd, p.Y-PYZonaEd
	mejor, dist := -1, float32(1e9)
	for i, l := range m.Lineas {
		var d float32
		switch {
		case y < l.Y:
			d = l.Y - y
		case y > l.Y+l.Alto:
			d = y - l.Y - l.Alto
		}
		d += fueraDeZona(l, x) // en una tabla, la celda de debajo del puntero
		if d < dist {
			mejor, dist = i, d
		}
	}
	if mejor < 0 {
		return Pos{}, false
	}
	l := m.Lineas[mejor]
	return z.e.acotar(Pos{l.Bloque, l.PosEn(x)}), true
}

// MouseDown pone el foco y el cursor donde se pulsa, antes de soltar: es lo que usa
// `ui.SoltarEnCampo` para que lo soltado entre por donde cae, como el `drop` de un
// `contenteditable`. El clic entero lo sigue atendiendo Tapped.
func (z *zonaEdicion) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button != desktop.MouseButtonPrimary {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(z); c != nil {
		c.Focus(z)
	}
	if p, ok := z.posEn(ev.Position); ok {
		z.e.hayX = false
		z.e.moverA(p)
	}
}

// MouseUp no hace nada: el clic lo remata Tapped y el arrastre, DragEnd.
func (z *zonaEdicion) MouseUp(*desktop.MouseEvent) {}

// Tapped pone el cursor donde se pulsa; sobre la casilla de una tarea, la marca.
func (z *zonaEdicion) Tapped(ev *fyne.PointEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(z); c != nil {
		c.Focus(z)
	}
	x, y := ev.Position.X-PXZonaEd, ev.Position.Y-PYZonaEd
	for _, a := range z.maquetado().Adornos {
		if a.Tipo == adornoCasilla && x >= a.X && x <= a.X+a.An && y >= a.Y && y <= a.Y+a.Al {
			bloque, marcada := a.Bloque, !a.Marcada
			z.e.hacer(func() { z.e.doc.MarcarTarea(bloque, marcada) })
			return
		}
	}
	if z.tocarAsa(ev.Position) {
		return // el menú del bloque (bloque_menu.go)
	}
	if z.tocarEspecial(ev.Position) {
		return // una etiqueta, una fecha o el título de un desplegable (insertar.go)
	}
	if z.dobleToque(ev.Position) {
		return // el segundo toque selecciona la palabra (editor_menu.go)
	}
	if p, ok := z.posEn(ev.Position); ok {
		z.e.hayX = false
		z.e.moverA(p)
	}
}

// Dragged selecciona arrastrando: el ancla en el punto de partida. Si se empieza en el
// asa de un bloque, lo que se arrastra es el bloque (arrastre_bloque.go).
func (z *zonaEdicion) Dragged(ev *fyne.DragEvent) {
	if z.llevando >= 0 {
		z.arrastrarAsa(ev.Position)
		return
	}
	if !z.arrastre {
		inicio := fyne.NewPos(ev.Position.X-ev.Dragged.DX, ev.Position.Y-ev.Dragged.DY)
		if z.enAsa(inicio) {
			z.llevando = z.encima
			z.arrastrarAsa(ev.Position)
			return
		}
		if p, ok := z.posEn(inicio); ok {
			z.e.ancla = p
		}
		z.arrastre = true
	}
	if p, ok := z.posEn(ev.Position); ok {
		z.e.cur = p
		z.e.escribiendo, z.e.pendiente = false, nil
		z.e.repintar()
	}
}

func (z *zonaEdicion) DragEnd() {
	z.arrastre = false
	if z.llevando >= 0 {
		z.soltarAsa()
	}
}

func (z *zonaEdicion) CreateRenderer() fyne.WidgetRenderer { return &zonaRenderer{z: z} }

type zonaRenderer struct {
	z       *zonaEdicion
	objetos []fyne.CanvasObject
}

func (r *zonaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.z.altoPara(max(r.z.Size().Width, r.z.ancho)))
}
func (r *zonaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *zonaRenderer) Destroy()                     { r.z.pararParpadeo() }

func (r *zonaRenderer) Layout(t fyne.Size) {
	z := r.z
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	z.ancho = t.Width
	z.maq = z.maquetar(t.Width, v)
	var objs []fyne.CanvasObject
	if z.enfocado && !z.e.SinAro {
		// `.sytue-focus-ring`: 2 px de `focus-ring` por fuera de la caja. La esquina es la
		// que la zona tiene enfocada en la web: 4 arriba (la del aro global) y 6 abajo
		// (`rounded-b-md`). Medido en producción: «4px 4px 6px 6px».
		aro := ui.NuevoAro(0, ui.SinHueco, v).Abierto().ConEsquinas(tema.RadioSM, tema.RadioMD)
		aro.Colocar(t)
		aro.Ver(true)
		objs = append(objs, aro.Objetos()...)
	}
	// Los velos de la búsqueda y la selección van sobre los fondos de la prosa (el del
	// código, el del `<mark>`) y bajo el texto, como en la web; si no, una coincidencia
	// en un bloque de código no se vería.
	fondos, delante := pintarProsaEnCapas(z.maq, v)
	for _, o := range append(fondos, delante...) {
		o.Move(o.Position().Add(fyne.NewPos(PXZonaEd, PYZonaEd)))
	}
	objs = append(objs, fondos...)
	objs = append(objs, r.velosBuscar(v)...)
	objs = append(objs, r.seleccion(v)...)
	objs = append(objs, delante...)
	if z.e.doc.Vacio() && z.e.marca != "" {
		// La marca de agua (`top-2 left-3`), mientras no hay nada escrito.
		m := ui.Texto(z.e.marca, escalonCuerpo, ui.Normal, tema.Color("text-subtle", v))
		ui.ColocarTexto(m, escalonCuerpo, PXZonaEd, PYZonaEd)
		objs = append(objs, m)
	}
	z.raya = nil
	if z.enfocado && !z.e.haySeleccion() {
		z.raya = r.cursor(v)
		z.raya.Hidden = z.apagado
		objs = append(objs, z.raya)
	}
	if asa := z.asa(v); asa != nil {
		objs = append(objs, asa)
	}
	if raya := z.rayaSoltar(t.Width, v); raya != nil {
		objs = append(objs, raya)
	}
	if a := z.contexto.ancla; a != nil {
		objs = append(objs, a) // de donde cuelga el menú contextual (editor_menu.go)
	}
	r.objetos = objs
}

// cursor es la raya de 1 px del alto de la letra, en la tinta del texto.
// Parpadea (cursor_parpadeo.go): la raya se guarda en la zona para encenderla y apagarla
// sin volver a maquetar.
func (r *zonaRenderer) cursor(v tema.Variante) *canvas.Rectangle {
	x, arriba, alto := r.z.cajaCursor()
	return rect(tema.Color("text", v), PXZonaEd+x, PYZonaEd+arriba, 1, alto, 0)
}

// cajaCursor es dónde va la raya del cursor en lo maquetado (sin el relleno de la zona):
// su X, la Y de arriba y el alto de la letra que hay delante.
func (z *zonaEdicion) cajaCursor() (x, arriba, alto float32) {
	m := z.maquetado()
	alto = altoFuenteDe(escalonCuerpo.Tam)
	arriba = (escalonCuerpo.Linea - alto) / 2
	if i := z.e.lineaDe(m, z.e.cur); i >= 0 {
		l := m.Lineas[i]
		tam := l.tamEn(z.e.cur.O)
		return l.XDe(z.e.cur.O), l.Base - ascensoFigtree*tam, altoFuenteDe(tam)
	}
	return 0, arriba, alto
}

// seleccion son los rectángulos de lo seleccionado, detrás del texto.
func (r *zonaRenderer) seleccion(v tema.Variante) []fyne.CanvasObject {
	z := r.z
	if !z.e.haySeleccion() || z.e.actualSeleccionada() {
		return nil
	}
	a, b := z.e.seleccion()
	tinte := velo(tema.Color("primary", v), 0.25)
	var out []fyne.CanvasObject
	for _, l := range z.maq.Lineas {
		if l.Bloque < a.B || l.Bloque > b.B {
			continue
		}
		desde, hasta := l.Desde, l.Hasta
		if l.Bloque == a.B {
			desde = max(desde, a.O)
		}
		if l.Bloque == b.B {
			hasta = min(hasta, b.O)
		}
		if hasta < desde || (hasta == desde && l.Bloque == b.B) {
			continue
		}
		x0, x1 := l.XDe(desde), l.XDe(hasta)
		// Lo seleccionado que sigue en la línea siguiente o en otro bloque se ve con un
		// hueco al final, como el salto que el navegador pinta seleccionado.
		if (Pos{l.Bloque, l.Hasta}).antes(b) {
			x1 += escalonCuerpo.Tam * 0.3
		}
		out = append(out, rect(tinte, PXZonaEd+x0, PYZonaEd+l.Y, max(x1-x0, 0), l.Alto, 0))
	}
	return out
}

func (r *zonaRenderer) Refresh() {
	r.Layout(r.z.Size())
	canvas.Refresh(r.z)
}

// ---- Guardado ----

// Las dos esperas del autoguardado de NoteCard.tsx: se guarda al levantar la vista del
// teclado (400 ms sin teclas), y como mucho cada 2 s aunque no se pare.
const (
	PausaAutoguardado = 400 * time.Millisecond
	EsperaMaxima      = 2 * time.Second
)

// EsperaDeAutoguardado es `esperaDeAutoguardado`: la pausa, o lo que quede del techo si
// ya se acerca. La cuenta corre desde el PRIMER cambio sin guardar.
func EsperaDeAutoguardado(sucioDesde, ahora time.Time) time.Duration {
	return min(PausaAutoguardado, max(0, sucioDesde.Add(EsperaMaxima).Sub(ahora)))
}

// fueraDeZona es lo lejos que queda una X de la celda de la línea (cero dentro de ella,
// o si la línea es de lado a lado).
func fueraDeZona(l LineaProsa, x float32) float32 {
	switch {
	case l.ZonaX1 == 0:
		return 0
	case x < l.ZonaX0:
		return l.ZonaX0 - x
	case x > l.ZonaX1:
		return x - l.ZonaX1
	}
	return 0
}

// lineaVecina es la línea de encima o de debajo de la `i` a la altura de `x`, como las
// flechas del navegador: la más cercana en vertical y, si cae en una fila de tabla, de
// entre las de esa fila, la de la celda que tiene `x` (la misma columna) o, si ninguna la
// tiene, la más cercana de lado. Las otras celdas de la fila del cursor no cuentan: a
// ellas se va de lado, no hacia arriba. -1 si no hay más líneas en ese sentido. Una celda
// con `rowspan` está en todas las filas que ocupa.
func lineaVecina(m *Maquetado, bloques []Bloque, i int, x float32, abajo bool) int {
	v := vecindad{m: m, bloques: bloques, abajo: abajo}
	candidatas, mejor := v.candidatas(i)
	if mejor < 0 || !v.celda(mejor).EnTabla() {
		return mejor
	}
	return v.enLaFila(candidatas, v.celda(mejor), x)
}

// vecindad es lo que necesita lineaVecina para mirar las líneas de alrededor.
type vecindad struct {
	m       *Maquetado
	bloques []Bloque
	abajo   bool
}

// celda es la del bloque de la línea `k`; fuera de una tabla, la vacía.
func (v vecindad) celda(k int) Celda {
	if b := v.m.Lineas[k].Bloque; b >= 0 && b < len(v.bloques) {
		return v.bloques[b].Celda
	}
	return Celda{}
}

// mismaFila dice si dos celdas de la misma tabla comparten alguna fila: con `rowspan`,
// una celda está en todas las que ocupa, y así se entra en ella desde cualquiera.
func mismaFila(a, b Celda) bool {
	if !a.EnTabla() || a.Tabla != b.Tabla {
		return false
	}
	a0, a1 := filasDe(a)
	b0, b1 := filasDe(b)
	return a0 < b1 && b0 < a1
}

// filasDe son las filas [desde, hasta) que ocupa una celda.
func filasDe(c Celda) (int, int) {
	t := c.Tabla
	if c.Fila >= len(t.Filas) || c.Col >= len(t.Filas[c.Fila].Celdas) {
		return c.Fila, c.Fila + 1
	}
	return c.Fila, min(c.Fila+tramo(t.Filas[c.Fila].Celdas[c.Col], "rowspan"), len(t.Filas))
}

// antes dice si la línea `j` está más cerca que la `k` en el sentido de la flecha.
func (v vecindad) antes(j, k int) bool {
	if v.abajo {
		return v.m.Lineas[j].Y < v.m.Lineas[k].Y
	}
	return v.m.Lineas[j].Y > v.m.Lineas[k].Y
}

// candidatas son las líneas en el sentido de la flecha, sin las otras celdas de la fila
// del cursor, y la más cercana en vertical de ellas (-1 si no hay).
func (v vecindad) candidatas(i int) ([]int, int) {
	actual, aqui := v.m.Lineas[i], v.celda(i)
	var out []int
	mejor := -1
	for j, l := range v.m.Lineas {
		if j == i || (v.abajo && l.Y <= actual.Y+0.5) || (!v.abajo && l.Y >= actual.Y-0.5) {
			continue
		}
		if c := v.celda(j); mismaFila(c, aqui) && c != aqui {
			continue
		}
		out = append(out, j)
		if mejor < 0 || v.antes(j, mejor) {
			mejor = j
		}
	}
	return out, mejor
}

// enLaFila elige, de las candidatas de esa fila de tabla, la de la columna de `x` o, si
// ninguna la tiene, la más cercana de lado; a igual distancia, la más cercana en vertical.
func (v vecindad) enLaFila(candidatas []int, fila Celda, x float32) int {
	elegida, lejos := -1, float32(0)
	for _, j := range candidatas {
		if !mismaFila(v.celda(j), fila) {
			continue
		}
		d := fueraDeZona(v.m.Lineas[j], x)
		if elegida < 0 || d < lejos || (d == lejos && v.antes(j, elegida)) {
			elegida, lejos = j, d
		}
	}
	return elegida
}
