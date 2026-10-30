// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package editor es el CROMO del editor ofimático de la suite, compartido por Docus,
// Calcus y Diapos.
//
// # Por qué es un paquete y no tres
//
// En la web los tres productos son tres apps que montan el mismo editor de
// `@sytue/office` con un `kind` distinto (`vistas/Edicion.astro`,
// `tipos-de-documento.test.tsx`): comparten `EditorApp`, la cinta de
// trece pestañas, la cabecera, el carril, la barra de estado y los ocho paneles. Lo
// ÚNICO que cambia es el lienzo. Copiar el cromo tres veces es exactamente la avería
// que `docs/CONVENTIONS.md` §13.2 documenta con números: el endpoint de auditoría de
// Mail y Drive salió de una copia y acabó contestando en dos idiomas.
//
// En NATIVO, en cambio, cada producto es su propio binario (`docs/CONVENTIONS.md`
// §13.6): `cmd/docus`, `cmd/calcus`, `cmd/diapos`, cada uno con su `FyneApp.toml` y su
// esquema de URL. El cromo es un paquete importado, no un binario común.
//
// # La costura, en dos piezas
//
//  1. [Lienzo] — lo que el cromo aloja en el centro. Lo implementa cada producto.
//  2. [Pestana] + [Registro] — cada producto añade SUS pestañas a la cinta; las
//     genéricas (Inicio, Revisar, Vista) las pone este paquete. Cuáles hay, dónde se ven
//     y en qué orden lo fija la taxonomía común con la web (`office.Pestanas`).
//
// Nada más. El cromo no sabe qué hay dentro del lienzo y el lienzo no sabe qué hay
// alrededor: hablan por [Contexto].
package editor

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// Tipo es el `DocKind` de la web: qué clase de documento se está editando. Decide qué
// pestañas de la cinta se ven y qué frase usa la barra de estado.
type Tipo uint8

// Los tres tipos, en el orden de `CONTENT_KEY` en `editor/types.ts`.
const (
	Documento    Tipo = iota // `document` — Docus
	Hoja                     // `spreadsheet` — Calcus
	Presentacion             // `presentation` — Diapos
)

// ClaveContenido es la clave del registro LWW donde vive TODO el contenido, tal cual
// la declara `CONTENT_KEY` en la web. Es el mismo documento que abre el navegador, así
// que la clave no se puede inventar.
func (t Tipo) ClaveContenido() string {
	switch t {
	case Hoja:
		return "doc:sheet"
	case Presentacion:
		return "doc:slides"
	case Documento:
	}
	return "doc:html"
}

// Guardado es el `SaveState` de la web: lo que anuncia el chip de la cabecera.
type Guardado uint8

// Los cuatro estados, en el orden de `SaveState`.
const (
	Limpio         Guardado = iota // `saved`
	Guardando                      // `saving`
	Sucio                          // `dirty`
	FalloAlGuardar                 // `error`
)

// Estado son los SEIS estados que toda pantalla de la suite tiene que saber pintar.
// No es una lista de cortesía: el barrido de pantallas los exige uno a uno.
type Estado uint8

// Los seis.
const (
	Cargando Estado = iota
	Vacio
	Error
	SinConexion
	SinPermiso
	Listo
	// SinSesion es el 401: la web nunca lo pinta —redirige al inicio de sesión—, y en
	// nativo es un estado porque abrir el navegador por cuenta propia al arrancar sería
	// peor que ofrecer el botón. Va el último para no renumerar los seis.
	SinSesion
)

// Lienzo es LA COSTURA: lo que el cromo aloja en el centro y cada producto escribe.
//
// Es deliberadamente corta. Todo lo que un lienzo necesita saber del mundo (el
// documento, el zoom, la traducción, si se puede escribir) llega por [Contexto], y
// todo lo que el cromo necesita saber del lienzo está aquí.
//
// Un lienzo es un `fyne.CanvasObject` porque el cromo lo coloca sin envolverlo: el
// área central es suya entera y su desplazamiento es cosa suya —una rejilla de celdas y
// un folio A4 no se desplazan igual—.
type Lienzo interface {
	// Objeto es lo que se pinta en el área central. Se llama UNA vez, al montar.
	Objeto() fyne.CanvasObject

	// Cargar recibe el contenido tal cual vino del servicio (el valor del registro
	// LWW de [Tipo.ClaveContenido]) y devuelve si lo entiende ENTERO.
	//
	// Devolver false NO es un fallo: es la promesa de que los documentos de la gente
	// no se corrompen. El cromo enseña la franja de solo lectura, bloquea los
	// comandos de edición y no deja guardar. Un lienzo que devuelve true sobre algo
	// que no entiende es el único fallo grave que este paquete puede cometer.
	Cargar(contenido string) (entendido bool)

	// Serializar devuelve lo que hay que guardar. El cromo solo lo llama si el
	// último Cargar devolvió true, así que un lienzo en solo lectura no tiene que
	// defenderse: no se le va a preguntar.
	Serializar() string

	// Estadisticas son las tres cifras de la barra de estado: la unidad actual, el
	// total de unidades y las palabras. `unidad` y `total` son páginas en Docus,
	// hojas en Calcus y diapositivas en Diapos; `palabras` es 0 fuera de Docus, como
	// en la web.
	Estadisticas() (unidad, total, palabras int)

	// Mandar ejecuta un comando de la cinta sobre la selección. Lo que el lienzo no
	// conozca lo ignora: la cinta es común y no todos los comandos aplican a todos
	// los lienzos.
	Mandar(c Comando, valor string)

	// Formato es el estado del formato bajo el cursor, para que la cinta pinte sus
	// botones pulsados. Un lienzo sin selección devuelve el cero.
	Formato() Formato
}

// ComandosCompactos permite trasladar los mandos propios del lienzo a la misma fila
// de la cinta cuando el teléfono apaisado necesita la altura para editar. El lienzo
// conserva sus callbacks y permisos; no se crea una segunda barra de comandos.
type ComandosCompactos interface {
	ComandosCompactos() []fyne.CanvasObject
}

// Comando es un comando de edición de la cinta. Son los mismos identificadores que
// `EditorCommand` en `editor/types.ts`, porque el mismo documento se edita en las dos y
// dos vocabularios distintos son dos comportamientos distintos.
type Comando string

// Los comandos que esta ola cablea. La lista de la web es más larga (bibliografía,
// correspondencia, macros, notas al final, campos de formulario); lo que no está aquí
// no está en la cinta nativa todavía.
const (
	Negrita      Comando = "bold"
	Cursiva      Comando = "italic"
	Subrayado    Comando = "underline"
	Tachado      Comando = "strike"
	SinFormato   Comando = "clearFormat"
	AlinearIzq   Comando = "alignLeft"
	AlinearCen   Comando = "alignCenter"
	AlinearDer   Comando = "alignRight"
	Justificar   Comando = "alignJustify"
	ListaPuntos  Comando = "listBullet"
	ListaNumero  Comando = "listNumber"
	MasSangria   Comando = "indent"
	MenosSangria Comando = "outdent"
	Deshacer     Comando = "undo"
	Rehacer      Comando = "redo"
	Titulo       Comando = "heading" // valor: "p", "h1", "h2", "h3", "blockquote"
	Superindice  Comando = "superscript"
	Subindice    Comando = "subscript"
	Fuente       Comando = "font"       // valor: la familia, «Arial»
	Tamano       Comando = "size"       // valor: el peldaño de `fontSize`, «1»…«7»
	ColorTexto   Comando = "color"      // valor: `#rrggbb` de CONTENT_PALETTE
	Resaltado    Comando = "highlight"  // valor: `#rrggbb` o «transparent»
	Interlineado Comando = "lineHeight" // valor: «1», «1.15», «1.5», «2»

	// Los de la pestaña «Insertar».
	InsertarTabla   Comando = "insertTable" // valor: «FxC», con «h» detrás si lleva fila de encabezado
	InsertarImagen  Comando = "insertImage" // valor: la data: URL
	InsertarEnlace  Comando = "insertLink"  // valor: la URL, un salto de línea y el texto visible
	InsertarRegla   Comando = "insertHr"
	SaltoPagina     Comando = "pageBreak"
	InsertarTexto   Comando = "insertText" // valor: el texto, un símbolo
	TablaFilaEncima Comando = "tableRowAbove"
	TablaFilaDebajo Comando = "tableRowBelow"
	TablaColIzq     Comando = "tableColLeft"
	TablaColDer     Comando = "tableColRight"
	TablaBorrarFila Comando = "tableDelRow"
	TablaBorrarCol  Comando = "tableDelCol"
	TablaBorrar     Comando = "tableDelete"
	TablaCombinar   Comando = "tableMergeCells"
	TablaDividir    Comando = "tableSplitCell"
	TablaFondo      Comando = "tableCellBg"     // valor: `#rrggbb` o «transparent»
	TablaVertical   Comando = "tableCellValign" // valor: «top», «middle» o «bottom»

	// Los de la pestaña «Referencias».
	InsertarIndice     Comando = "insertToc" // valor: el título del índice, ya traducido
	ActualizarIndice   Comando = "updateToc" // valor: el título del índice, ya traducido
	NotaPie            Comando = "footnote"  // valor: el texto provisional de la nota
	NotaFinal          Comando = "endnote"
	ActualizarFinal    Comando = "endnotesUpdate"
	MarcarIndice       Comando = "indexMark" // valor: «término» o «término:subtérmino»
	GenerarIndice      Comando = "indexBuild"
	InsertarLeyenda    Comando = "caption" // valor: «tabla» o «figura», un salto de línea y el texto
	ActualizarLeyendas Comando = "captionsUpdate"
	ReferenciaCruzada  Comando = "crossReference" // valor: el id de la leyenda

	// Los de «Encabezado y pie» de Insertar. En la web no pasan por `exec` sino por
	// funciones de la pestaña (`ensureRegion`, `insertPageField`…); los nombres son los
	// de sus iconos.
	InsertarEncabezado    Comando = "header"
	InsertarPie           Comando = "footer"
	InsertarNumeroPagina  Comando = "pageNumber" // valor: el texto al insertarlo, `status.pageOf` ya traducido
	PrimeraPaginaDistinta Comando = "differentFirstPage"
)

// Alineacion es la de un párrafo.
type Alineacion uint8

// Las cuatro, en el orden de `FormatState.align`.
const (
	Izquierda Alineacion = iota
	Centro
	Derecha
	Justificado
)

// Formato es el `FormatState` de la web recortado a lo que la cinta nativa pinta hoy.
// El cero es «sin selección», que es lo que devuelve un lienzo recién cargado.
type Formato struct {
	Negrita, Cursiva, Subrayado, Tachado bool
	Alineacion                           Alineacion
	Puntos, Numeros                      bool
	// Titulo es "p", "h1", "h2", "h3" o "blockquote". Vacío cuenta como "p".
	Titulo                      string
	PuedeDeshacer, PuedeRehacer bool
	// Fuente es la familia bajo el cursor tal cual está escrita («Georgia»); vacía es la
	// del folio. Tamano es su peldaño de `fontSize` («1»…«7»), o vacío si no es ninguno.
	Fuente, Tamano string
	// EnTabla enciende la sección de tabla del panel de ajustes.
	EnTabla bool
	// Imagen es `imageSelected`: hay una imagen seleccionada, y la cinta enseña sus
	// herramientas.
	Imagen bool
}

// Pestana es una pestaña de la cinta: el rótulo de la tira de arriba y los grupos de
// controles de la banda de abajo.
//
// El cuerpo se construye con [Grupo] y [SeparadorGrupos]; nada impide devolver
// cualquier `fyne.CanvasObject`, pero entonces la banda no mide lo que la web.
type Pestana struct {
	// ID es el identificador estable de la pestaña (`ribbon-tab-<id>` en la web). Lo
	// usa el registro para ordenar y para recordar cuál estaba abierta.
	ID string
	// Rotulo es la CLAVE de catálogo del rótulo, no el rótulo. La traducción la hace
	// el cromo con el traductor del [Contexto].
	Rotulo string
	// Tipos son los tipos de documento en los que esta pestaña se ve. Vacío = todos.
	Tipos []Tipo
	// Cuerpo construye la banda de grupos. Se llama en cada cambio de pestaña y en
	// cada refresco del formato, así que no debe guardar estado propio.
	Cuerpo func(*Contexto) fyne.CanvasObject
}

// Visible dice si esta pestaña se ve con ese tipo de documento.
func (p Pestana) Visible(t Tipo) bool {
	if len(p.Tipos) == 0 {
		return true
	}
	for _, x := range p.Tipos {
		if x == t {
			return true
		}
	}
	return false
}

// Registro es la lista de pestañas de la cinta, en el orden en que se pintan.
//
// QUÉ pestañas hay, en qué tipos de documento se ven y en qué ORDEN lo dice la taxonomía
// común (`apps/nucleo/office/pestanas.json`, la misma que lee la cinta web): Inicio y
// Revisar siempre, Insertar y Diseño en documento y presentación, Datos y Fórmulas en la
// hoja, Presentar en las diapositivas. El cromo trae las genéricas con [Genericas]; cada
// producto AÑADE las suyas con [Registro.Anadir], y ni el rótulo, ni los tipos, ni el
// sitio se escriben aquí: salen de la taxonomía.
type Registro struct{ pestanas []Pestana }

// tipoDe es el [Tipo] de un `DocKind` de la web.
func tipoDe(kind string) (Tipo, bool) {
	switch kind {
	case office.Documento:
		return Documento, true
	case office.Hoja:
		return Hoja, true
	case office.Presentacion:
		return Presentacion, true
	}
	return 0, false
}

// deTaxonomia es la pestaña con el rótulo y los tipos que la taxonomía común le da.
func deTaxonomia(p office.Pestana, cuerpo func(*Contexto) fyne.CanvasObject) Pestana {
	tipos := make([]Tipo, 0, len(p.Kinds))
	for _, k := range p.Kinds {
		if t, ok := tipoDe(k); ok {
			tipos = append(tipos, t)
		}
	}
	return Pestana{ID: p.ID, Rotulo: p.LabelKey, Tipos: tipos, Cuerpo: cuerpo}
}

// Genericas es el registro de partida: las pestañas que los tres productos comparten
// (Inicio, Revisar y Vista), con el orden, el rótulo y los tipos de la taxonomía. Se
// llama una vez por aplicación y se le añaden las propias.
func Genericas() *Registro {
	cuerpos := map[string]func(*Contexto) fyne.CanvasObject{
		"inicio": cuerpoInicio, "revisar": cuerpoColaboracion, "vista": cuerpoVista,
	}
	r := &Registro{}
	for _, p := range office.Pestanas() {
		if cuerpo, ok := cuerpos[p.ID]; ok {
			r.pestanas = append(r.pestanas, deTaxonomia(p, cuerpo))
		}
	}
	return r
}

// Anadir mete la pestaña de ese id de la taxonomía en su sitio: con su rótulo y sus tipos,
// y delante de la primera ya presente que la taxonomía pone detrás. Un id que la taxonomía
// no conoce es un error de programación —un producto inventándose una pestaña— y para.
func (r *Registro) Anadir(id string, cuerpo func(*Contexto) fyne.CanvasObject) {
	def, pos, ok := office.BuscarPestana(id)
	if !ok {
		panic("editor: la pestaña " + id + " no está en la taxonomía (apps/nucleo/office/pestanas.json)")
	}
	nueva := deTaxonomia(def, cuerpo)
	for i, q := range r.pestanas {
		if _, otra, ok := office.BuscarPestana(q.ID); ok && otra > pos {
			r.pestanas = append(r.pestanas[:i], append([]Pestana{nueva}, r.pestanas[i:]...)...)
			return
		}
	}
	r.pestanas = append(r.pestanas, nueva)
}

// Insertar mete una pestaña JUSTO ANTES de la de ese id. Es para las que NO están en la
// taxonomía (las de las pruebas); las de los productos van con [Registro.Anadir]. Con un
// id que no existe, la añade al final.
func (r *Registro) Insertar(antesDe string, p Pestana) {
	for i, q := range r.pestanas {
		if q.ID == antesDe {
			r.pestanas = append(r.pestanas[:i], append([]Pestana{p}, r.pestanas[i:]...)...)
			return
		}
	}
	r.pestanas = append(r.pestanas, p)
}

// Sustituir cambia el cuerpo de una pestaña genérica sin cambiar su sitio ni su
// rótulo. Es lo que necesita Calcus, cuya pestaña «Inicio» no es la de Docus (la web
// tiene `InicioSheets.tsx` aparte). Con un id que no existe no hace nada.
func (r *Registro) Sustituir(id string, cuerpo func(*Contexto) fyne.CanvasObject) {
	for i := range r.pestanas {
		if r.pestanas[i].ID == id {
			r.pestanas[i].Cuerpo = cuerpo
			return
		}
	}
}

// Visibles son las pestañas que se ven con ese tipo de documento, en orden.
func (r *Registro) Visibles(t Tipo) []Pestana {
	out := make([]Pestana, 0, len(r.pestanas))
	for _, p := range r.pestanas {
		if p.Visible(t) {
			out = append(out, p)
		}
	}
	return out
}

// Contexto es lo que TODA pieza del editor recibe: el `EditorContext` de la web, sin
// lo que allí es del navegador.
//
// Es una estructura y no una interfaz a propósito: tiene catorce campos y ninguna
// implementación alternativa, así que una interfaz solo añadiría una indirección y
// catorce métodos que escribir dos veces (`docs/CONVENTIONS.md` §13.2).
type Contexto struct {
	// Tipo decide las pestañas visibles y la frase de la barra de estado.
	Tipo Tipo
	// Titulo es el nombre del documento; lo edita la cabecera.
	Titulo string
	// Guardado es lo que anuncia el chip de la cabecera.
	Guardado Guardado
	// Estado es cuál de los seis estados pinta la pantalla.
	Estado Estado
	// SoloLectura lo enciende el cromo cuando el lienzo NO entendió el documento, y
	// también cuando el permiso es de lectura. Mientras esté encendido, la cinta no
	// manda comandos de edición y no se guarda.
	SoloLectura bool
	// MotivoSoloLectura es la clave de catálogo que explica por qué. Vacío = permiso.
	MotivoSoloLectura string
	// Peticion es la salida de la franja (petición del dueño, 2026-09-25): «Pedir
	// acceso de edición» cuando el motivo es el permiso y, para el dueño, quién se lo
	// pide. Lo rellena EngancharPeticionEdicion.
	Peticion peticion.Estado
	// BloqueoPropio es el caso (a) de la franja: un bloqueo que quien edita puede quitar
	// ella misma (la hoja protegida de Calcus). Devuelve el aviso ya traducido y con qué
	// quitarlo; ("", nil) si no hay. nil = la app no tiene bloqueos propios.
	BloqueoPropio func() (aviso string, desbloquear func())

	// Zoom es el porcentaje, de [ZoomMin] a [ZoomMax] en pasos de 10.
	Zoom int
	// Regla enseña la regla horizontal (solo en Docus).
	Regla bool
	// Panel es el panel del carril abierto, o [SinPanel].
	Panel Panel
	// Ajustes es el panel derecho.
	Ajustes bool
	// IA dice si hay Sytue AI detrás (`ctx.aiEnabled`): sin ella, «Asistente» no se ofrece
	// en la cinta.
	IA bool

	// T traduce una clave de catálogo. Nunca se escribe texto a pelo en el editor.
	//
	// La firma es la de `i18n.Translator.T` tal cual, para que se pueda pasar el
	// método directamente (`ctx.T = tr.T`) sin un adaptador que convierta variables
	// posicionales en nombradas: «Página 2 de 7» es UNA frase con `{page}` y
	// `{pages}`, y en alemán y en francés cambian el orden y la preposición.
	T func(clave string, vars i18n.Vars) string

	// Lienzo es el del producto. El cromo no lo construye.
	Lienzo Lienzo

	// Refrescar lo llama cualquier pieza que cambie algo del contexto. Lo pone el
	// [Marco]; una pieza suelta puede encontrárselo nil y tiene que aguantarlo.
	Refrescar func()

	// RefrescarEstado repinta SOLO la barra de estado: es lo que pide el lienzo al
	// desplazarse o al escribir, donde rehacer la cinta entera en cada tecla sobra.
	// Lo pone el [Marco]; puede ser nil.
	RefrescarEstado func()

	// AlRenombrar recibe el título en cuanto se teclea; la pausa antes de mandarlo es
	// de quien lo pone. nil: el título no se puede cambiar (sin permiso de escritura, o
	// sin servicio), y el campo se queda inerte.
	AlRenombrar func(string)

	// Lo que ofrecen el menú «Archivo» de la cinta y «Descargar» de la cabecera. Cada
	// uno lo pone el producto; nil deja la entrada inerte, que es más honesto que un
	// botón que no hace nada.
	//
	// Nuevo y Abrir son «Nuevo documento» y «Abrir un archivo», las dos primeras
	// entradas del menú Archivo en nativo (abrir.go). nil: la entrada no sale.
	Nuevo func()
	Abrir func()
	// Exportar recibe uno de `EXPORT_FORMATS` («pdf», «html», «markdown», «odf»,
	// «ooxml»); Exportando es el que se está generando ahora («» si ninguno).
	Exportar func(formato string)
	// ExportarPlantilla pide ooxml/odf con template:true, conservando el guardado previo.
	// nil deja las dos descargas de plantillas inertes.
	ExportarPlantilla func(formato string)
	Exportando        string
	// Imprimir es Ctrl/Cmd+P: el PDF del documento, con lo pendiente ya guardado, al
	// diálogo de impresión del sistema. En la web lo hace el navegador; aquí lo pone el
	// producto (`Exportar(FormatoImprimir)`). nil: el atajo no hace nada.
	Imprimir func()
	// Temas es el grupo «Tema» de la cinta (temas comunes y marca de la organización) de
	// Calcus y Diapos; nil, sin él (marca.go).
	Temas *TemasDoc
	// Bloques es el grupo «Bloques» de la cinta (bloques reutilizables); nil, sin él.
	Bloques *BloquesDoc
	// GuardarVersion guarda una versión con esa etiqueta, que puede ir vacía. La
	// pregunta —opcional, como en la web— la hace la cinta (`preguntarVersion`).
	GuardarVersion func(etiqueta string)
	// GuardarEnDrive devuelve los cambios al fichero de Drive del que salió el
	// documento; nil si no salió de ninguno, y entonces la entrada no se enseña.
	GuardarEnDrive func()
	// SinRed apaga lo que necesita el servicio (descargar, compartir, versiones).
	SinRed bool
	// Paneles da el cuerpo de un panel del carril que el producto sabe llenar (Docus:
	// buscar en el folio, el esquema, los comentarios). nil, o nil para un panel,
	// deja el de por defecto. Se llama en cada recomposición: si el cuerpo tiene estado
	// —lo tecleado en «Buscar»— tiene que devolver SIEMPRE el mismo objeto.
	Paneles func(Panel) fyne.CanvasObject
	// Colaboración: comentar, el historial de versiones y compartir. Versiones pide
	// la lista y llama a `listo` en el hilo de la interfaz; RestaurarVersion ya viene
	// confirmado por quien pregunta. nil: el botón sale inerte.
	NuevoComentario  func()
	Versiones        func(listo func([]api.DocVersion))
	RestaurarVersion func(api.DocVersion)
	// Actividad pide la auditoría del documento (quién cambió qué y cuándo) y llama a
	// `listo` en el hilo de la interfaz. Solo el propietario la tiene —el servicio da 403
	// al resto—, así que sin ella el chip «Actividad» no se enseña.
	Actividad func(listo func([]api.DocActividad, error))
	// Compartir abre el panel de compartir colgado de `desde`, el botón pulsado.
	Compartir func(desde fyne.CanvasObject)
	// El control de cambios, el grupo «Revisión» de Colaboración (solo en Docus, como en
	// la web): EnRastreo dice si está encendido, AlternarRastreo lo enciende o lo apaga, y
	// ResolverCambios acepta —o rechaza— todos los cambios y dice si había alguno. nil: el
	// botón sale inerte.
	EnRastreo       func() bool
	AlternarRastreo func()
	ResolverCambios func(aceptar bool) bool
	// Notificar enseña un aviso de ese tono en la pila de la ventana (`toast.notify`).
	Notificar func(tono ui.TonoAviso, titulo string)
	// Vista: «Ajustar al ancho» (lo pone el lienzo que sabe medirse), la pantalla
	// completa (la pone quien tiene la ventana) y el tema del lienzo, `Sepia` el
	// `--editor-canvas-bg-sepia` de la web.
	AjustarAlAncho     func()
	PantallaCompleta   func()
	EnPantallaCompleta func() bool
	Sepia              bool
	// Avisar enseña un aviso en la pila de la ventana (el `toast` de la web). Nil, nada.
	Avisar func(titulo, texto string)
	// Loc es el idioma de las fechas (el historial).
	Loc i18n.Locale
	// Volver y EnfocarTitulo los pone el [Marco]: «Volver» del menú y «Renombrar».
	Volver        func()
	EnfocarTitulo func()
}

// Producto es la marca del conmutador que corresponde a este tipo (`TESELA_POR_KIND`).
func (t Tipo) Producto() iconos.Producto {
	switch t {
	case Hoja:
		return iconos.Sheets
	case Presentacion:
		return iconos.Slides
	case Documento:
	}
	return iconos.Docs
}

// Traducir es T con el hueco tapado: una clave sin traductor se devuelve tal cual, que
// es más útil que un pánico en una prueba.
func (c *Contexto) Traducir(clave string, vars ...i18n.Vars) string {
	if c == nil || c.T == nil {
		return clave
	}
	if len(vars) == 0 {
		return c.T(clave, nil)
	}
	return c.T(clave, vars[0])
}

// Mandar pasa un comando al lienzo, salvo en solo lectura. El cortafuegos está AQUÍ y
// no en cada botón: un botón que se olvide no puede corromper nada.
func (c *Contexto) Mandar(cmd Comando, valor string) {
	if c.SoloLectura || c.Lienzo == nil {
		return
	}
	c.Lienzo.Mandar(cmd, valor)
	grabadora.registrar(c.Tipo, cmd, valor) // no-op salvo grabación de macro activa
	if c.Refrescar != nil {
		c.Refrescar()
	}
}

// Panel es el panel del carril izquierdo.
type Panel uint8

// Los paneles del carril, en el orden de la web.
const (
	SinPanel Panel = iota
	PanelBuscar
	PanelEsquema
	PanelComentarios
	PanelRevision
	PanelIA
)

// carril es la fila del carril: su icono y su clave de rótulo.
type carrilItem struct {
	p      Panel
	icono  iconos.Ed
	rotulo string
}
