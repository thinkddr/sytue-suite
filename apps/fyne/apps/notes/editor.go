// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
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
// ponytail: tres límites conocidos. (1) No hay IME: el driver de Fyne no lo tiene en
// escritorio (docs/roadmap.md, «Techos del toolkit»), así que el chino o el japonés no se
// teclean aquí ni en ningún campo nativo. (2) El cursor no parpadea. (3) La barra no entra
// en el orden de tabulación (el Tab del cuerpo es sangrar, como en la web); se usa con el
// puntero o con sus atajos (Ctrl+B, Ctrl+I, Ctrl+U, Ctrl+K, Ctrl+Alt+0…5).

// Medidas de la barra y del cuerpo, de la hoja con el rem de 14.
const (
	LadoBotonEd      = 7 * 0.25 * tema.Rem   // `h-7 w-7`: 24,5
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

// BloquesEd son los ocho del desplegable, con su clave.
var BloquesEd = []struct{ Valor, Clave string }{
	{"p", "editor.paragraph"},
	{"h1", "editor.h1"},
	{"h2", "editor.h2"},
	{"h3", "editor.h3"},
	{"h4", "editor.h4"},
	{"h5", "editor.h5"},
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
	barra *fyne.Container
	// barraTabla es la de las acciones de la tabla, que solo se ve en una celda.
	barraTabla *fyne.Container
	// fondoBarra y fileteBarra son el `bg-surface` y el `border-b` de la barra, y
	// fondoTabla y fileteTabla los de la barra de tabla.
	fondoBarra, fileteBarra *canvas.Rectangle
	fondoTabla, fileteTabla *canvas.Rectangle
	botones                 map[string]*botonBarra
	bloqueBt                *botonBloque
	// Estirado es la tarjeta estirada: el cuerpo crece hasta abajo (`flex-1`).
	Estirado bool

	AlCambiar  func(html string)
	AlTerminar func()
	// AlEnlazar abre la herramienta de enlaces colgando del botón; la pantalla sabe qué
	// notas ofrecer. Devuelve por `aplicarEnlace`.
	AlEnlazar func(disparador fyne.CanvasObject)
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

	// menu es el de «/» abierto (comandos.go) y descartada, la barra a la que se dijo que
	// no con Escape; burbuja, la de formato (burbuja.go); buscador, «Buscar en la nota»
	// (buscar_nota.go).
	menu       *menuComandos
	descartada *Pos
	burbuja    *flotante
	buscador   *buscadorNota
	// portal es donde van el menú y la burbuja para que nada los tape (flotante.go); sin
	// él (Mail, las pruebas), van con el editor.
	portal *portal
}

var _ fyne.Widget = (*EditorNota)(nil)

// NuevoEditorNota prepara el editor de un documento. El cursor queda al final.
func NuevoEditorNota(tr *i18n.Translator, d *Documento, marcaDeAgua string) *EditorNota {
	e := &EditorNota{tr: tr, doc: d.ParaEditar(), marca: marcaDeAgua}
	e.ExtendBaseWidget(e)
	e.zona = &zonaEdicion{e: e}
	e.zona.ExtendBaseWidget(e.zona)
	e.cur = e.fin()
	e.ancla = e.cur
	e.barra = e.armarBarra()
	e.barraTabla = e.armarBarraTabla()
	e.barraTabla.Hidden = !e.enTabla()
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

// cambiado es lo que sigue a todo cambio: acotar el cursor, avisar y repintar.
func (e *EditorNota) cambiado() {
	e.cur, e.ancla = e.acotar(e.cur), e.acotar(e.ancla)
	e.pendiente = nil
	llamar1(e.AlCambiar, e.HTML())
	e.repintar()
}

func (e *EditorNota) repintar() {
	antes, tabla := e.zona.Size().Height, e.barraTabla.Visible()
	e.refrescarBarra()
	e.zona.Refresh()
	if e.enTabla() != tabla {
		e.barraTabla.Hidden = !e.enTabla()
		e.Refresh()
	}
	if e.AlCrecer != nil && (e.zona.MinSize().Height != antes || e.enTabla() != tabla) {
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
	llamar1(e.AlCambiar, e.HTML())
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
	case "b", "i", "u", "s", "code":
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
		if !e.bloqueActual().Celda.EnTabla() {
			return
		}
		e.hacer(func() { e.cur = e.doc.EditarTabla(orden, e.cur); e.ancla = e.cur })
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
		if rotulo == "" {
			rotulo = url
		}
		estilo := estiloEn(e.bloqueActualDe(e.cur), e.cur.O)
		inicio := e.cur
		fin := e.doc.Insertar(e.cur, rotulo, estilo)
		e.doc.Enlazar(inicio, fin, url)
		e.cur = e.doc.Insertar(fin, "\u00a0", estilo)
		e.ancla = e.cur
	})
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
	llamar1(e.AlCambiar, e.HTML())
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
	llamar1(e.AlCambiar, e.HTML())
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

// Pegar pega un documento con su formato donde está el cursor, pisando lo seleccionado,
// en un solo paso de deshacer. Es para quien pega HTML (el redactor de Mail); en una nota
// el portapapeles entra como texto, como en la web.
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
		if e.shift {
			e.Orden("desangrar", "")
		} else {
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
	e.hacer(func() {
		inicio := Pos{e.cur.B, e.cur.O - borrar}
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
		// Ver la cabecera de RichText.tsx: el portapapeles entra como TEXTO PLANO.
		if x.Clipboard != nil {
			e.PegarTexto(x.Clipboard.Content())
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
		e.atajoPropio(x)
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

func (e *EditorNota) atajoPropio(x *desktop.CustomShortcut) {
	ctrl := x.Modifier&fyne.KeyModifierShortcutDefault != 0
	if !ctrl {
		return
	}
	if x.Modifier&fyne.KeyModifierAlt != 0 {
		// Ctrl+Alt+0 párrafo, Ctrl+Alt+1…5 títulos.
		if n := string(x.KeyName); len(n) == 1 && n[0] >= '0' && n[0] <= '5' {
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
		if b := e.botones["enlace"]; b != nil {
			llamar1(e.AlEnlazar, fyne.CanvasObject(b))
		}
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

// armarBarra monta los botones en el orden de RichText.tsx.
func (e *EditorNota) armarBarra() *fyne.Container {
	t := e.tr.T
	e.botones = map[string]*botonBarra{}
	boton := func(clave, glifo string, icono iconos.Icono, rotulo string, f func()) *botonBarra {
		b := nuevoBotonBarra(glifo, icono, rotulo, f)
		e.botones[clave] = b
		return b
	}
	atajo := func(rotulo, teclas string) string {
		return t("editor.shortcut", i18n.Vars{"label": rotulo, "keys": teclas})
	}
	sep := func() fyne.CanvasObject { return nuevoSeparadorEd() }
	e.bloqueBt = nuevoBotonBloque(t("editor.block", nil), func() { e.menuBloque() })
	items := make([]fyne.CanvasObject, 0, 28)
	items = append(items,
		boton("undo", DeshacerD, iconos.Ninguno, atajo(t("editor.undo", nil), ui.AtajoDelSistema("Ctrl+Z")), func() { e.Orden("undo", "") }),
		boton("redo", RehacerD, iconos.Ninguno, atajo(t("editor.redo", nil), ui.AtajoDelSistema("Ctrl+Y")), func() { e.Orden("redo", "") }),
		sep(), e.bloqueBt, sep(),
		boton("b", NegritaD, iconos.Ninguno, atajo(t("editor.bold", nil), ui.AtajoDelSistema("Ctrl+B")), func() { e.Orden("b", "") }),
		boton("i", CursivaD, iconos.Ninguno, atajo(t("editor.italic", nil), ui.AtajoDelSistema("Ctrl+I")), func() { e.Orden("i", "") }),
		boton("u", SubrayadoD, iconos.Ninguno, atajo(t("editor.underline", nil), ui.AtajoDelSistema("Ctrl+U")), func() { e.Orden("u", "") }),
		boton("s", TachadoD, iconos.Ninguno, t("editor.strike", nil), func() { e.Orden("s", "") }),
		boton("mark", "", iconos.Rotulador, t("editor.highlight", nil), func() { e.Orden("mark", "") }),
		boton("code", "", iconos.Codigo, t("editor.inlineCode", nil), func() { e.Orden("code", "") }),
	)
	pintura := boton("color", PaletaD, iconos.Ninguno, t("editor.textColor", nil), nil)
	pintura.AlPulsar = func() { e.paletaColor(pintura) }
	items = append(items, pintura, sep(),
		boton("tarea", TareaD, iconos.Ninguno, atajo(t("note.newTodo", nil), "[]"), func() { e.Orden("tarea", "") }),
		boton("vinetas", VinetasD, iconos.Ninguno, t("editor.bulletList", nil), func() { e.Orden("vinetas", "") }),
		boton("numeros", NumerosD, iconos.Ninguno, t("editor.numberedList", nil), func() { e.Orden("numeros", "") }),
		boton("desangrar", DesangrarD, iconos.Ninguno, atajo(t("editor.outdent", nil), "Shift+Tab"), func() { e.Orden("desangrar", "") }),
		boton("sangrar", SangrarD, iconos.Ninguno, atajo(t("editor.indent", nil), "Tab"), func() { e.Orden("sangrar", "") }),
		sep(),
		boton("left", IzquierdaD, iconos.Ninguno, t("editor.alignLeft", nil), func() { e.Orden("alinear", "left") }),
		boton("center", CentroD, iconos.Ninguno, t("editor.alignCenter", nil), func() { e.Orden("alinear", "center") }),
		boton("right", DerechaD, iconos.Ninguno, t("editor.alignRight", nil), func() { e.Orden("alinear", "right") }),
		sep(),
	)
	enlace := boton("enlace", EnlaceD, iconos.Ninguno, atajo(t("editor.linkTool", nil), ui.AtajoDelSistema("Ctrl+K")), nil)
	enlace.AlPulsar = func() { llamar1(e.AlEnlazar, fyne.CanvasObject(enlace)) }
	items = append(items, enlace,
		boton("regla", "", iconos.Menos, t("editor.hr", nil), func() { e.Orden("regla", "") }),
		boton("limpiar", GomaD, iconos.Ninguno, t("editor.clearFormat", nil), func() { e.Orden("limpiar", "") }),
		// Tabla 3×3 de partida (`insertarTabla(3, 3)`).
		boton("tabla", TablaD, iconos.Ninguno, t("editor.table", nil), func() { e.Orden("tabla", "") }),
	)
	e.fondoBarra, e.fileteBarra = canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(color.NRGBA{})
	return container.New(&barraFlex{entera: -1, px: PXBarraEd, py: PYBarraEd, hueco: HuecoBarraEd},
		append([]fyne.CanvasObject{e.fondoBarra, e.fileteBarra}, items...)...)
}

// armarBarraTabla es la barra de «Acciones de la tabla», que en la web sale SOLO con el
// cursor en una celda (`enTabla`): el rótulo «Tabla» en `text-xs text-subtle px-1`, las
// cuatro de insertar —con el icono `minus` de la web, las cuatro— y, tras un separador,
// las tres de eliminar con la papelera.
func (e *EditorNota) armarBarraTabla() *fyne.Container {
	t := e.tr.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rotulo := container.New(ui.Margen{Izq: PXRotuloTabla, Der: PXRotuloTabla},
		lineaTok(t("editor.table", nil), tema.TextoXS, ui.Normal, "text-subtle", v))
	items := []fyne.CanvasObject{rotulo}
	for i, o := range OrdenesTabla {
		if i == 4 {
			items = append(items, nuevoSeparadorEd())
		}
		glifo, icono := "", iconos.Menos
		if i >= 4 {
			glifo, icono = PapeleraD, iconos.Ninguno
		}
		b := nuevoBotonBarra(glifo, icono, t(o.Clave, nil), func() { e.Orden(o.Orden, "") })
		e.botones[o.Orden] = b
		items = append(items, b)
	}
	e.fondoTabla, e.fileteTabla = canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(color.NRGBA{})
	return container.New(&barraFlex{entera: -1, px: PXBarraEd, py: PYBarraEd, hueco: HuecoBarraEd},
		append([]fyne.CanvasObject{e.fondoTabla, e.fileteTabla}, items...)...)
}

// PXRotuloTabla es el `px-1` del rótulo de la barra de tabla.
const PXRotuloTabla = 1 * 0.25 * tema.Rem

// enTabla dice si el cursor está en una celda: es lo que enseña la barra de tabla.
func (e *EditorNota) enTabla() bool {
	return len(e.doc.Bloques) > 0 && e.bloqueActual().Celda.EnTabla()
}

// barras son las barras que se ven, en su orden: la de formato y, con el cursor en una
// celda, la de tabla.
func (e *EditorNota) barras() []fyne.CanvasObject {
	out := []fyne.CanvasObject{e.barra}
	if e.buscador != nil {
		out = append(out, e.buscador.barra)
	}
	if e.iaFallo && e.aviso != nil {
		out = append(out, e.aviso) // encima de la de tabla, como en la web
	}
	if e.enTabla() {
		out = append(out, e.barraTabla)
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
	e.fondoBarra.FillColor = tema.Color("surface", v)
	e.fileteBarra.FillColor = tema.Color("border", v)
	e.fondoTabla.FillColor = tema.Color("surface", v)
	e.fileteTabla.FillColor = tema.Color("border", v)
	bl := Bloque{Tipo: Parrafo}
	if len(e.doc.Bloques) > 0 {
		bl = e.bloqueActual()
	}
	estados := map[string]bool{
		"b": e.estadoMarca("b"), "i": e.estadoMarca("i"), "u": e.estadoMarca("u"), "s": e.estadoMarca("s"),
		"vinetas": bl.Tipo == Vineta, "numeros": bl.Tipo == Numerada,
		"left": bl.Alineacion == "left", "center": bl.Alineacion == "center", "right": bl.Alineacion == "right",
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
			b.Inerte = e.enTabla() // una tabla dentro de otra no se edita (documento.go)
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
}

// menuBloque abre el desplegable de «Estilo del texto» con los ocho, el actual marcado.
func (e *EditorNota) menuBloque() {
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
	m.Mostrar(e.bloqueBt)
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
	r.e.zona.Move(fyne.NewPos(0, alto))
	r.e.zona.Resize(fyne.NewSize(t.Width, max(t.Height-alto, r.e.zona.altoPara(t.Width))))
	if r.e.portal == nil {
		r.e.colocarFlotantes(r.e.zona.Position(), t.Width, 0)
		return
	}
	off := r.e.portal.desde(r.e)
	r.e.colocarFlotantes(r.e.zona.Position().Add(off), r.e.portal.c.Size().Width, r.e.portal.c.Size().Height)
	r.e.portal.poner(r.e, r.e.flotantes()...)
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

func (z *zonaEdicion) FocusGained() { z.enfocado = true; z.Refresh(); z.e.Refresh() }

// FocusLost: salir del cuerpo guarda y cierra el editor —se salga como se salga—, salvo
// que el foco se haya ido a un menú o a una paleta de la barra (una capa encima) o a
// ningún sitio (un clic en el lienzo vacío), que es lo que decide `onBlurCapture` en la
// web. Se decide DESPUÉS, cuando el foco ya ha caído en su sitio.
func (z *zonaEdicion) FocusLost() {
	z.enfocado = false
	z.Refresh()
	z.e.Refresh() // la burbuja solo se ve con el cuerpo enfocado
	fyne.Do(func() {
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
		// La caja de «Buscar en la nota» es del propio editor: ir a ella no es salir.
		if z.e.buscador != nil && f == fyne.Focusable(z.e.buscador.campo) {
			return
		}
		llamar0(z.e.AlTerminar)
	})
}

func (z *zonaEdicion) TypedRune(r rune) {
	if r == ' ' {
		z.e.espacio()
		return
	}
	z.e.escribir(string(r))
}

func (z *zonaEdicion) TypedKey(k *fyne.KeyEvent) {
	// Escape es del editor (`AlTerminar`, en `tecla`), no del diálogo de encima.
	if z.e.Tecla != nil && z.e.Tecla(k) {
		return
	}
	z.e.tecla(k.Name)
}

func (z *zonaEdicion) TypedShortcut(s fyne.Shortcut) {
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
	if p, ok := z.posEn(ev.Position); ok {
		z.e.hayX = false
		z.e.moverA(p)
	}
}

// Dragged selecciona arrastrando: el ancla en el punto de partida.
func (z *zonaEdicion) Dragged(ev *fyne.DragEvent) {
	if !z.arrastre {
		inicio := fyne.NewPos(ev.Position.X-ev.Dragged.DX, ev.Position.Y-ev.Dragged.DY)
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

func (z *zonaEdicion) DragEnd() { z.arrastre = false }

func (z *zonaEdicion) CreateRenderer() fyne.WidgetRenderer { return &zonaRenderer{z: z} }

type zonaRenderer struct {
	z       *zonaEdicion
	objetos []fyne.CanvasObject
}

func (r *zonaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.z.altoPara(max(r.z.Size().Width, r.z.ancho)))
}
func (r *zonaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *zonaRenderer) Destroy()                     {}

func (r *zonaRenderer) Layout(t fyne.Size) {
	z := r.z
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	z.ancho = t.Width
	z.maq = z.maquetar(t.Width, v)
	var objs []fyne.CanvasObject
	if z.enfocado {
		// `.sytue-focus-ring`: 2 px de `focus-ring` por fuera de la caja. La esquina es la
		// que la zona tiene enfocada en la web: 4 arriba (la del aro global) y 6 abajo
		// (`rounded-b-md`). Medido en producción: «4px 4px 6px 6px».
		aro := ui.NuevoAro(0, ui.SinHueco, v).Abierto().ConEsquinas(tema.RadioSM, tema.RadioMD)
		aro.Colocar(t)
		aro.Ver(true)
		objs = append(objs, aro.Objetos()...)
	}
	objs = append(objs, r.seleccion(v)...)
	dentro := PintarProsa(z.maq, v)
	for _, o := range dentro {
		o.Move(o.Position().Add(fyne.NewPos(PXZonaEd, PYZonaEd)))
	}
	objs = append(objs, dentro...)
	if z.e.doc.Vacio() && z.e.marca != "" {
		// La marca de agua (`top-2 left-3`), mientras no hay nada escrito.
		m := ui.Texto(z.e.marca, escalonCuerpo, ui.Normal, tema.Color("text-subtle", v))
		ui.ColocarTexto(m, escalonCuerpo, PXZonaEd, PYZonaEd)
		objs = append(objs, m)
	}
	if z.enfocado && !z.e.haySeleccion() {
		objs = append(objs, r.cursor(v)...)
	}
	r.objetos = objs
}

// cursor es la raya de 1 px del alto de la letra, en la tinta del texto.
func (r *zonaRenderer) cursor(v tema.Variante) []fyne.CanvasObject {
	x, arriba, alto := r.z.cajaCursor()
	return []fyne.CanvasObject{rect(tema.Color("text", v), PXZonaEd+x, PYZonaEd+arriba, 1, alto, 0)}
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
	if !z.e.haySeleccion() {
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

// ---- Botones de la barra ----

// botonBarra es BOTON_DE_BARRA: 24,5 × 24,5, plano, `rounded-sm`, el glifo de 16 en
// `text-muted` que pasa a `text` al pasar, `bg-bg-subtle` encima y `bg-selected` pulsado
// (`aria-pressed`), y al 40 % apagado.
//
// NO es enfocable, a propósito: en la web el botón hace `preventDefault` en el
// `mousedown` para que el cuerpo no pierda el foco ni la selección, y aquí eso es no
// aceptar el foco.
type botonBarra struct {
	widget.BaseWidget
	glifo  string
	icono  iconos.Icono
	Rotulo string
	// ConEstado es `aria-pressed`: Activo pinta el hundido.
	ConEstado, Activo, Inerte bool
	AlPulsar                  func()
	// Glifo y Radio cambian el glifo de 16 y el `rounded-sm` (cero: esos).
	Glifo, Radio float32
	encima       bool
}

var (
	_ fyne.Tappable     = (*botonBarra)(nil)
	_ desktop.Hoverable = (*botonBarra)(nil)
)

func nuevoBotonBarra(glifo string, icono iconos.Icono, rotulo string, f func()) *botonBarra {
	b := &botonBarra{glifo: glifo, icono: icono, Rotulo: rotulo, AlPulsar: f}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonBarra) AccessibilityLabel() string { return b.Rotulo }
func (b *botonBarra) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

func (b *botonBarra) Tapped(*fyne.PointEvent) {
	if !b.Inerte {
		llamar0(b.AlPulsar)
	}
}

func (b *botonBarra) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonBarra) MouseMoved(*desktop.MouseEvent) {}
func (b *botonBarra) MouseOut()                      { b.encima = false; b.Refresh() }

func (b *botonBarra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonBarraRenderer{b: b, fondo: canvas.NewRectangle(color.NRGBA{})}
	r.fondo.CornerRadius = cmp.Or(b.Radio, tema.RadioSM)
	if b.glifo != "" {
		r.dibujo = iconos.TrazoDeRuta(b.glifo, b.lado(), tema.Color("text-muted", v))
	} else {
		r.dibujo = iconos.Trazo(b.icono, b.lado(), "text-muted", v)
	}
	r.Refresh()
	return r
}

type botonBarraRenderer struct {
	b      *botonBarra
	fondo  *canvas.Rectangle
	dibujo *iconos.Dibujo
}

func (r *botonBarraRenderer) MinSize() fyne.Size { return fyne.NewSize(LadoBotonEd, LadoBotonEd) }
func (r *botonBarraRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.dibujo.Objeto()}
}
func (r *botonBarraRenderer) Destroy() {}
func (r *botonBarraRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	g := r.b.lado()
	r.dibujo.Colocar((t.Width-g)/2, (t.Height-g)/2, g)
}

func (b *botonBarra) lado() float32 { return cmp.Or(b.Glifo, GlifoEd) }

func (r *botonBarraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := r.b
	tinta, fondo := tema.Color("text-muted", v), color.NRGBA{}
	switch {
	case b.Inerte:
	case b.ConEstado && b.Activo:
		tinta, fondo = tema.Color("text", v), tema.Color("selected", v)
	case b.encima:
		tinta, fondo = tema.Color("text", v), tema.Color("bg-subtle", v)
	}
	if b.Inerte {
		tinta = velo(tinta, 0.4)
	}
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.dibujo.TenirCon(tinta)
}

// botonBloque es el desplegable de «Estilo del texto»: `w-auto gap-1 px-1.5 text-sm` con
// el glifo `heading` de 14, el rótulo (`max-w-24 truncate`) y el chevron de 12.
type botonBloque struct {
	botonBarra
	Texto string
}

func nuevoBotonBloque(rotulo string, f func()) *botonBloque {
	b := &botonBloque{}
	b.Rotulo, b.AlPulsar = rotulo, f
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonBloque) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonBloqueRenderer{
		b:      b,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(TituloD, 14, tema.Color("text-muted", v)),
		flecha: iconos.Trazo(iconos.ChevronAbajo, 12, "text-muted", v),
		texto:  ui.Texto(b.Texto, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
	}
	r.fondo.CornerRadius = tema.RadioSM
	r.Refresh()
	return r
}

type botonBloqueRenderer struct {
	b             *botonBloque
	fondo         *canvas.Rectangle
	glifo, flecha *iconos.Dibujo
	texto         *canvas.Text
}

func (r *botonBloqueRenderer) anchoTexto() float32 {
	return min(ui.Medir(r.b.Texto, tema.TextoSM, ui.Normal).Width, AnchoMaxBloqueEd)
}

func (r *botonBloqueRenderer) MinSize() fyne.Size {
	hueco := float32(0.25 * tema.Rem)
	return fyne.NewSize(2*PXBarraEd+14+hueco+r.anchoTexto()+hueco+12, LadoBotonEd)
}

func (r *botonBloqueRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.glifo.Objeto(), r.texto, r.flecha.Objeto()}
}
func (r *botonBloqueRenderer) Destroy() {}

func (r *botonBloqueRenderer) Layout(t fyne.Size) {
	hueco := float32(0.25 * tema.Rem)
	r.fondo.Resize(t)
	x := float32(PXBarraEd)
	r.glifo.Colocar(x, (t.Height-14)/2, 14)
	x += 14 + hueco
	ui.Truncar(r.texto, r.b.Texto, tema.TextoSM, ui.Normal, AnchoMaxBloqueEd)
	ui.ColocarTexto(r.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	x += r.anchoTexto() + hueco
	r.flecha.Colocar(x, (t.Height-12)/2, 12)
}

func (r *botonBloqueRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, fondo := tema.Color("text-muted", v), color.NRGBA{}
	if r.b.encima {
		tinta, fondo = tema.Color("text", v), tema.Color("bg-subtle", v)
	}
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.glifo.TenirCon(tinta)
	r.flecha.TenirCon(tinta)
	r.texto.Text, r.texto.Color = r.b.Texto, tinta
	r.texto.Refresh()
}

// separadorEd es SEPARADOR_DE_BARRA: un pelo de 1 × 17,5 con `mx-1`.
type separadorEd struct{ widget.BaseWidget }

func nuevoSeparadorEd() *separadorEd {
	s := &separadorEd{}
	s.ExtendBaseWidget(s)
	return s
}

func (s *separadorEd) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &separadorRenderer{r: canvas.NewRectangle(tema.Color("border", v))}
}

type separadorRenderer struct{ r *canvas.Rectangle }

func (s *separadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(tema.Borde+2*MargenSeparadorE, AltoSeparadorEd)
}
func (s *separadorRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{s.r} }
func (s *separadorRenderer) Destroy()                     {}
func (s *separadorRenderer) Layout(t fyne.Size) {
	s.r.Move(fyne.NewPos(MargenSeparadorE, (t.Height-AltoSeparadorEd)/2))
	s.r.Resize(fyne.NewSize(tema.Borde, AltoSeparadorEd))
}

func (s *separadorRenderer) Refresh() {
	s.r.FillColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	s.r.Refresh()
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
