// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fmt"
	"image"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El formato nativo es HTML, y lo que no se entiende no se toca
//
// Lo mismo que se decidió para Notepads: el HTML no es una exportación, es EL modelo. La
// web lo dice con todas las letras en `editor/types.ts` (`document: 'doc:html' — HTML del
// lienzo (el HTML ES el modelo)`), así que el mismo documento se abre en el navegador y
// aquí, y guardar desde cualquiera de los dos tiene que dejarlo igual.
//
// De eso se encarga `rico`: edita el árbol del HTML tocando SOLO el bloque editado, y
// declara editable un documento únicamente si entiende cada etiqueta, cada atributo y
// cada propiedad de estilo que trae, y si escribirlo y volverlo a leer da el mismo árbol.
// Lo demás —una imagen, una tabla, el control de cambios— se ABRE y se lee, en solo
// lectura, y la franja lo dice.

// Las medidas del folio, de `canvas/metrics.ts` y `editor.css`. Van en PÍXELES en la hoja,
// así que el rem de 14 no las toca.
const (
	// AnchoPagina y AltoPagina son el A4 a 96 dpi y zoom 100 (210 × 297 mm).
	AnchoPagina float32 = 794
	AltoPagina  float32 = 1123
	// MargenPagina es el `Normal` de los tres de Disposición (48 / 96 / 128).
	MargenPagina float32 = 96
	// AireFolio es el `PAGE_GAP` de 32 del lienzo, arriba y abajo. En la web se divide
	// por el zoom dentro de la caja escalada para que en pantalla mida siempre 32.
	AireFolio float32 = 32
)

// Los colores de CONTENIDO del folio. No son tokens de la suite y no deben serlo: son
// locales de `editor.css` porque el papel es BLANCO en los dos temas.
var (
	// PapelBlanco es `--editor-page-bg`.
	PapelBlanco = rico.Papel
	// TintaContenido es el `color: #0a0a0a` de `.sytue-page`.
	TintaContenido = rico.Tinta
	// LienzoClaro y LienzoOscuro son `--editor-canvas-bg` en los dos temas.
	LienzoClaro  = color.NRGBA{R: 0xE9, G: 0xE6, B: 0xDD, A: 0xFF}
	LienzoOscuro = color.NRGBA{R: 0x18, G: 0x1A, B: 0x1C, A: 0xFF}
	// LienzoSepia es `--editor-canvas-bg-sepia`, el «Sepia» de Vista. Solo está en el
	// `:root` claro de la hoja, así que en oscuro vale lo mismo.
	LienzoSepia = color.NRGBA{R: 0xEC, G: 0xE4, B: 0xD2, A: 0xFF}
	// CortePagina es `--editor-page-break`: rgba(15, 23, 42, 0.3), la raya de dos
	// píxeles que marca cada alto de A4 cuando el documento pasa de una página.
	CortePagina = color.NRGBA{R: 15, G: 23, B: 42, A: 77}
)

// anchoContenido es lo que queda del A4 vertical entre sus dos márgenes `Normal`.
const anchoContenido = AnchoPagina - 2*MargenPagina

// Pagina es el `PageSetup` de la web más la decoración de «Disposición»: márgenes,
// orientación, tamaño, color del papel y numeración. Todo salvo la numeración se guarda en
// el documento, en la capa de la página ([rico.PaginaDoc]), como en la web.
type Pagina struct {
	Margen   float32 // 48, 96 o 128
	Apaisada bool
	Tamano   string      // «a4», «carta», «legal»
	Papel    color.NRGBA // A == 0: el blanco de siempre
	Numerar  bool
	// Ancho, Alto y Margenes son los de un documento que trae su página (un .docx importado):
	// mandan sobre Tamano, Apaisada y Margen mientras no se elija otro en la cinta.
	Ancho, Alto float32
	Margenes    [4]float32 // arriba, derecha, abajo, izquierda; todo 0 es Margen en los cuatro
}

// margenes son los cuatro márgenes: arriba, derecha, abajo, izquierda.
func (p Pagina) margenes() [4]float32 {
	if p.Margenes != ([4]float32{}) {
		return p.Margenes
	}
	return [4]float32{p.Margen, p.Margen, p.Margen, p.Margen}
}

// paginaDe es la página que guarda el documento, sobre la de ahora (de ella queda la numeración).
func paginaDe(d *rico.PaginaDoc, ahora Pagina) Pagina {
	p := PaginaPorDefecto()
	p.Numerar = ahora.Numerar
	if d == nil {
		return p
	}
	p.Ancho, p.Alto, p.Margenes, p.Apaisada = d.Ancho, d.Alto, d.Margenes, d.Ancho > d.Alto
	if m := d.Margenes; m[0] == m[1] && m[1] == m[2] && m[2] == m[3] {
		p.Margen, p.Margenes = m[0], [4]float32{}
	}
	cerca := func(a, b float32) bool { return a-b < 2 && b-a < 2 }
	for nombre, t := range TamanosPagina {
		if (cerca(t[0], d.Ancho) && cerca(t[1], d.Alto)) || (cerca(t[1], d.Ancho) && cerca(t[0], d.Alto)) {
			p.Tamano, p.Ancho, p.Alto = nombre, 0, 0
		}
	}
	if c, ok := editor.Hex(d.Color); ok {
		p.Papel = c
	}
	return p
}

// paginaDoc es lo que se guarda de p: la página entera, o nil si es la de por defecto.
func paginaDoc(p Pagina) *rico.PaginaDoc {
	p.Numerar = false
	if p == PaginaPorDefecto() {
		return nil
	}
	ancho, alto := p.medidas()
	d := &rico.PaginaDoc{Ancho: ancho, Alto: alto, Margenes: p.margenes()}
	if p.Papel.A > 0 {
		d.Color = hexDe(p.Papel)
	}
	return d
}

// TamanosPagina son los de `PAGE_SIZES`, en px a 96 dpi y vertical.
var TamanosPagina = map[string][2]float32{"a4": {AnchoPagina, AltoPagina}, "carta": {816, 1056}, "legal": {816, 1344}}

// PaginaPorDefecto es la de la web al abrir: A4 vertical con el margen `Normal`.
func PaginaPorDefecto() Pagina { return Pagina{Margen: MargenPagina, Tamano: "a4"} }

// medidas son el ancho y el alto del folio con esa orientación.
func (p Pagina) medidas() (ancho, alto float32) {
	if p.Ancho > 0 && p.Alto > 0 {
		return p.Ancho, p.Alto
	}
	t, ok := TamanosPagina[p.Tamano]
	if !ok {
		t = TamanosPagina["a4"]
	}
	if p.Apaisada {
		return t[1], t[0]
	}
	return t[0], t[1]
}

// Folio es el lienzo de Docus: la página A4 sobre el fondo greige, con su regla, y el
// editor de texto enriquecido que la web tiene en su contenteditable.
//
// Implementa [editor.Lienzo], que es la única costura con el cromo.
type Folio struct {
	widget.BaseWidget

	ctx *editor.Contexto
	doc *rico.Documento
	// crudo es el HTML tal cual llegó: lo que se devuelve si el documento no se entiende.
	crudo string

	bloques []rico.Bloque
	cajas   []caja
	alto    float32 // el del contenido, sin los márgenes de la página

	// cur es donde está el cursor y anc donde empezó la selección; iguales, no hay.
	cur, anc punto
	// ed es el bloque del cursor abierto para editar. Se conserva entre teclas porque
	// guarda la marca pedida con el cursor quieto («negrita» y a escribir).
	ed *rico.Edicion
	// xObjetivo es la columna a la que vuelven las flechas arriba y abajo.
	xObjetivo    float32
	hayObjetivo  bool
	mayus        bool // Mayús pulsada: las flechas y el clic extienden la selección
	enfocado     bool
	conCursor    bool // alguien ha puesto el cursor: hasta entonces la cinta no refleja nada
	caretVisible bool
	parpadeo     *fyne.Animation
	porRaton     bool // el último clic ya colocó el cursor en MouseDown

	hist    historial
	cambios int

	// pagina es la disposición de la página; ver [Pagina].
	pagina Pagina
	// caja, si no es nil, es el folio SIN página: el texto de una caja de otro lienzo
	// (ver [Caja]). Manda sobre `pagina`.
	caja *Caja
	// zoomPuesto es el último zoom que puso el ajuste al ancho: si `ctx.Zoom` ya no es
	// este, lo ha cambiado la persona y el ajuste se retira. -1 marca una elección
	// explícita incluso cuando coincide con el último porcentaje automático.
	zoomPuesto int
	// ajustandoZoom distingue la actualización automática de una elección manual
	// del mismo porcentaje. La elección manual siempre retira el ajuste al ancho.
	ajustandoZoom bool

	// resaltes son las coincidencias de «Buscar» y activa la que se está viendo;
	// ocultos, que el panel está cerrado y no se pintan, y buscado, la última consulta,
	// para repintarlas al volver a abrirlo.
	resaltes []coincidencia
	activa   int
	ocultos  bool
	buscado  struct {
		q              string
		mayus, palabra bool
	}
	// ancla es lo anclado al comentario que se señala en el panel, y revisado el cambio
	// en foco del panel «Revisión».
	ancla    []coincidencia
	revisado []coincidencia
	buscador *editor.Buscador
	revisor  *revisor

	// Autor es quien firma las marcas del control de cambios: el de la sesión. Vacío, la
	// reserva de la web.
	Autor rico.Autor

	// sel es la imagen seleccionada (su bloque y su runa) y arrastre, el de uno de sus
	// tiradores o el de moverla (imagen.go).
	sel      *punto
	arrastre *arrastreImagen
	// tiradorTexto es el de la selección que lleva el dedo en un táctil (`tactil.go`).
	tiradorTexto int
	// previa es el registro de la combinación que se está viendo (la vista previa), o nil.
	previa *int

	// laminas son las imágenes pintadas la última vez, por su src (y cuántas van antes
	// con el mismo): ver [Folio.lamina]. Con su cerrojo: en las pruebas se repinta desde
	// más de una rutina.
	laminas   map[string]*canvas.Image
	laminasMu sync.Mutex

	pag        *pagina
	scroll     *container.Scroll
	envoltorio *container.ThemeOverride

	// AlEditar se llama tras CADA cambio del documento: quien lo pone programa el
	// guardado. nil en solo lectura.
	AlEditar func()
	// AlGuardarYa es Ctrl/Cmd+S: adelantar el guardado que ya está programado.
	AlGuardarYa func()
	// Programar corre f dentro de d en el hilo de la interfaz. Las pruebas lo cambian
	// para no depender del reloj.
	Programar        func(d time.Duration, f func())
	formatoPendiente bool
	ultimoFormato    editor.Formato
	// campoPuesto es el texto que lleva ahora el número de página, o "" si no hay.
	campoPuesto string
}

var (
	_ fyne.Widget   = (*Folio)(nil)
	_ editor.Lienzo = (*Folio)(nil)
)

// NuevoFolio crea el lienzo. El contexto se guarda para leer el zoom, la regla y el
// permiso; puede ser nil en un guardián.
func NuevoFolio(ctx *editor.Contexto) *Folio {
	f := &Folio{ctx: ctx, doc: rico.Parse(""), caretVisible: true, pagina: PaginaPorDefecto(), zoomPuesto: 100}
	f.ExtendBaseWidget(f)
	f.pag = &pagina{f: f}
	f.pag.ExtendBaseWidget(f.pag)
	f.scroll = container.NewScroll(f.pag)
	// Sin las sombras que `container.Scroll` pinta en el borde por donde sigue el
	// contenido: la web no las tiene, y sobre el greige se ven como una raya oscura.
	f.envoltorio = container.NewThemeOverride(f.scroll, tema.SinSombras{})
	f.scroll.OnScrolled = func(fyne.Position) {
		if f.campoPuesto != "" && f.textoCampo() != f.campoPuesto {
			f.remaquetar() // el número de página sigue a la que se está viendo
		}
		if f.ctx != nil && f.ctx.RefrescarEstado != nil {
			f.ctx.RefrescarEstado()
		}
	}
	f.Programar = func(d time.Duration, fn func()) {
		time.AfterFunc(d, func() { fyne.Do(fn) })
	}
	f.remaquetar()
	return f
}

// Objeto es el propio folio: el cromo lo coloca sin envolverlo.
func (f *Folio) Objeto() fyne.CanvasObject { return f }

// Cargar lee el HTML y dice si lo entiende ENTERO. Uno que no se entiende se abre y se
// lee, pero en solo lectura: el cromo enseña la franja y no deja guardar.
func (f *Folio) Cargar(contenido string) bool {
	f.crudo = contenido
	f.doc = rico.ParseCon(contenido, f.hoja())
	entendido := f.doc.Editable()
	if entendido {
		f.doc.Asegurar()
	} else if f.ctx != nil {
		f.ctx.MotivoSoloLectura = "editor.readOnly"
	}
	f.cur, f.anc, f.ed, f.conCursor = punto{}, punto{}, nil, false
	f.resaltes, f.revisado, f.sel, f.arrastre, f.previa = nil, nil, nil, nil, nil
	if f.revisor != nil {
		f.revisor.activa = -1
		f.revisor.pintar()
	}
	f.hist = historial{}
	f.cambios = 0
	f.pagina = paginaDe(f.doc.PaginaDe(), f.pagina)
	f.remaquetar()
	return entendido
}

// Fusionar pone el documento que deja la coedición sin perder el sitio: el cursor y la
// selección se quedan en su bloque y su posición, acotados a lo que haya, y la cuenta de
// cambios no se mueve. El historial sí se vacía: deshacer por encima de lo que acaba de
// escribir otro se lo llevaría por delante.
func (f *Folio) Fusionar(contenido string) bool {
	cur, anc, con, cambios := f.cur, f.anc, f.conCursor, f.cambios
	ok := f.Cargar(contenido)
	f.cambios = cambios
	acotar := func(p punto) punto {
		p.b = max(0, min(p.b, len(f.bloques)-1))
		p.p = max(0, min(p.p, f.largo(p.b)))
		return p
	}
	f.cur, f.anc, f.conCursor = acotar(cur), acotar(anc), con
	f.Refresh()
	return ok
}

// Serializar devuelve el HTML. Con un documento que no se entiende —o que tras editar ya
// no se leería igual— devuelve lo que llegó, sin tocarlo.
func (f *Folio) Serializar() string {
	if f.doc == nil || !f.doc.Guardable() {
		return f.crudo
	}
	return f.doc.HTML()
}

// Cambios cuenta las ediciones desde que se cargó: quien guarda compara con lo guardado.
func (f *Folio) Cambios() int { return f.cambios }

// Documento es el modelo, para los guardianes.
func (f *Folio) Documento() *rico.Documento { return f.doc }

// Estadisticas son la página actual, el total y las palabras, como las cuenta la web:
// las páginas son el alto del folio continuo entre el de un A4, y la actual sale del
// desplazamiento.
func (f *Folio) Estadisticas() (unidad, total, palabras int) {
	unidad, total = f.paginas()
	return unidad, total, Palabras(f.Serializar())
}

// paginas son la página que se está viendo y el total.
func (f *Folio) paginas() (unidad, total int) {
	papel := f.altoPapel()
	_, altoPag := f.pagina.medidas()
	total = max(int((papel+altoPag-1)/altoPag), 1)
	z := f.escala()
	unidad = int((f.scroll.Offset.Y-AireFolio)/(altoPag*z)) + 1
	return min(max(unidad, 1), total), total
}

// Palabras cuenta las palabras EXACTAMENTE como la web (`countWords` de
// `state/useDocument.ts`): fuera los `<script>`, cada etiqueta por un ESPACIO, y a partir
// por blancos. Cambiar la etiqueta por un espacio no es lo mismo que quitarla: en
// «…<strong>dos fines de semana</strong>, sin…» la coma queda suelta y cuenta como una
// palabra más. Se replica y no se mejora: la cifra sale en la barra de estado de los dos
// sitios, y dos números distintos para el mismo documento es peor que uno discutible.
func Palabras(html string) int {
	html = reScript.ReplaceAllString(html, " ")
	return len(strings.Fields(reEtiqueta.ReplaceAllString(html, " ")))
}

var (
	reEtiqueta = regexp.MustCompile(`<[^>]+>`)
	reScript   = regexp.MustCompile(`(?is)<script.*?</script>`)
)

func (f *Folio) escala() float32 {
	if f.caja != nil {
		return f.caja.Escala
	}
	if f.ctx == nil || f.ctx.Zoom <= 0 {
		return 1
	}
	return float32(f.ctx.Zoom) / 100
}

func (f *Folio) conRegla() bool { return f.ctx != nil && f.ctx.Regla }

// editable dice si se puede escribir: el documento se entiende y el permiso lo deja.
func (f *Folio) editable() bool {
	return f.doc != nil && f.doc.Editable() && (f.ctx == nil || !f.ctx.SoloLectura) && f.previa == nil
}

// altoPapel es el folio continuo: el contenido con sus dos márgenes, y un A4 como mínimo.
func (f *Folio) altoPapel() float32 {
	if f.caja != nil {
		return f.caja.Alto
	}
	_, alto := f.pagina.medidas()
	m := f.pagina.margenes()
	return max(alto, f.alto+m[0]+m[2])
}

// anchoTexto es lo que queda del folio entre sus dos márgenes.
func (f *Folio) anchoTexto() float32 {
	if f.caja != nil {
		return max(f.caja.Ancho-2*f.caja.Relleno, 1)
	}
	ancho, _ := f.pagina.medidas()
	m := f.pagina.margenes()
	return ancho - m[1] - m[3]
}

// Pagina es la disposición de ahora.
func (f *Folio) Pagina() Pagina { return f.pagina }

// GuardarPagina pone la página y la guarda en el documento, en su capa, con su paso de
// deshacer: es lo que hace la cinta. La numeración no se guarda: es de la vista.
func (f *Folio) GuardarPagina(p Pagina) {
	antes := paginaDoc(f.pagina)
	f.PonerPagina(p)
	ahora := paginaDoc(p)
	if !f.editable() || (antes == nil && ahora == nil) || (antes != nil && ahora != nil && *antes == *ahora) {
		return
	}
	f.antesDeCambiar(false)
	f.doc.PonerPaginaDoc(ahora)
	f.reestructurado()
}

// PonerPagina cambia la disposición y remaqueta: el texto se reparte al ancho nuevo.
func (f *Folio) PonerPagina(p Pagina) {
	f.pagina = p
	f.remaquetar()
	f.pag.Refresh()
	f.scroll.Refresh()
	if f.ctx != nil && f.ctx.RefrescarEstado != nil {
		f.ctx.RefrescarEstado()
	}
}

// remaquetar vuelve a leer los bloques y a repartirlos en líneas, y repinta.
//
// ponytail: maqueta el documento ENTERO en cada tecla. Con la caché de medidas cuesta
// poco hasta decenas de páginas; si un documento largo se nota al escribir, se remaqueta
// solo el bloque tocado y se desplazan los de debajo.
func (f *Folio) remaquetar() {
	f.bloques = f.doc.Bloques()
	campo := f.rotularCampos()
	f.cajas, f.alto = maquetar(f.bloques, f.anchoTexto())
	if campo != "" && f.textoCampo() != campo {
		// El documento ha cambiado de páginas al maquetarlo: el número se rehace una vez.
		f.rotularCampos()
		f.cajas, f.alto = maquetar(f.bloques, f.anchoTexto())
	}
	f.colocarRegiones()
	f.pag.Refresh()
}

// traducir es la traducción de una clave, o la clave sin contexto (las pruebas).
func (f *Folio) traducir(clave string) string {
	if f.ctx == nil {
		return clave
	}
	return f.ctx.Traducir(clave)
}

// textoCampo es `pageFieldText`: «Página X de Y» con la página que se está viendo, que
// es lo que la web escribe en cada `[data-page-field]` al desplazarse.
func (f *Folio) textoCampo() string {
	unidad, total := f.paginas()
	if f.ctx == nil {
		return "status.page"
	}
	return f.ctx.Traducir("status.page", i18n.Vars{"page": unidad, "pages": total})
}

// rotularCampos pone el texto de cada número de página en su tramo, y devuelve el que ha
// puesto ("" si el documento no lleva ninguno).
func (f *Folio) rotularCampos() string {
	texto := ""
	for i := range f.bloques {
		for j := range f.bloques[i].Tramos {
			if reg := f.registroPrevia(); reg != nil && f.bloques[i].Tramos[j].Campo != "" {
				f.bloques[i].Tramos[j].Rotulo = reg[f.bloques[i].Tramos[j].Campo]
			}
			if f.bloques[i].Tramos[j].CampoPagina {
				if texto == "" {
					texto = f.textoCampo()
				}
				f.bloques[i].Tramos[j].Rotulo = texto
			}
		}
	}
	f.campoPuesto = texto
	return texto
}

// colocarRegiones lleva el pie a su sitio: pegado al fondo del papel, que es donde lo pone
// su `bottom`, se alargue lo que se alargue el documento.
func (f *Folio) colocarRegiones() {
	for i := range f.cajas {
		if c := &f.cajas[i]; c.b.Region == rico.Pie {
			m := f.pagina.margenes()
			c.y = f.altoPapel() - m[0] - m[2] + asomaRegion - c.alto
		}
	}
}

// MinSize deja que el cromo decida: el folio se desplaza por dentro.
func (f *Folio) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(0, 0)
}

// CreateRenderer pone el fondo del lienzo y, encima, el desplazamiento con la página.
func (f *Folio) CreateRenderer() fyne.WidgetRenderer {
	return &folioRenderer{f: f, fondo: canvas.NewRectangle(f.lienzo())}
}

// lienzo es el fondo de detrás del folio: el greige, el de oscuro o el sepia de Vista.
func (f *Folio) lienzo() color.NRGBA {
	switch {
	case f.caja != nil:
		return color.NRGBA{} // la caja va sobre su diapositiva: sin fondo propio
	case f.ctx != nil && f.ctx.Sepia:
		return LienzoSepia
	case tema.De(fyne.CurrentApp().Settings().ThemeVariant()) == tema.Oscuro:
		return LienzoOscuro
	}
	return LienzoClaro
}

// HuecoAjuste es el `FIT_GAP` de «Ajustar al ancho»: lo que se deja libre a los lados.
const HuecoAjuste = 48

// AjustarAlAncho es «Ajustar al ancho»: el zoom que hace caber el folio en el ancho
// del lienzo, en pasos de 5 (`Math.round(avail / pageW * 20) * 5`).
func (f *Folio) AjustarAlAncho() {
	disponible := f.scroll.Size().Width - HuecoAjuste
	ancho, _ := f.pagina.medidas()
	if f.ctx == nil || disponible <= 0 {
		return
	}
	f.ctx.PonerZoom(int(disponible/ancho*20+0.5) * 5)
}

// ajusteInicial encuadra el folio al abrir. Con el dedo no baja de 75%: el cuerpo
// normal de 16 conserva 12 dp, y el papel A4 real se alcanza con el scroll en dos ejes.
//
// La web lo hace UNA vez, cuando el lienzo ya mide lo que va a medir. Aquí el primer
// Layout llega con la ventana aún en su tamaño mínimo (SetContent va antes que Resize),
// así que no vale la primera medida: el ajuste sigue al ancho MIENTRAS nadie haya
// tocado el zoom a mano —que es cuando `ctx.Zoom` deja de ser el último que puso él—, y
// vuelve a 100 si luego el folio cabe.
func (f *Folio) ajusteInicial(ancho float32) {
	if ancho <= 0 || f.caja != nil || f.ctx == nil || f.ctx.Zoom != f.zoomPuesto {
		return
	}
	disponible := ancho - AireFolio
	pag, _ := f.pagina.medidas()
	z := 100
	if disponible < pag {
		z = max(int(disponible/pag*100), editor.ZoomMin)
	}
	if ui.EsTactil() {
		z = max(z, 75)
	}
	if z == f.ctx.Zoom {
		return
	}
	f.zoomPuesto = z
	// Fuera del Layout: cambiar el zoom recompone el marco, que es quien está colocando.
	fyne.Do(func() {
		if f.ctx.Zoom != z && f.zoomPuesto == z {
			seguirCursor := f.cursorEnVista()
			f.ajustandoZoom = true
			f.ctx.PonerZoom(z)
			f.ajustandoZoom = false
			if seguirCursor {
				f.asegurarVisible()
			}
		}
	})
}

// ZoomExplicito retira el encuadre automático cuando la persona elige un porcentaje,
// aunque sea el mismo que éste había elegido. No cambia la página ni el documento.
func (f *Folio) ZoomExplicito() {
	if !f.ajustandoZoom {
		f.zoomPuesto = -1
	}
}

// cursorEnVista distingue editar de leer otra zona con el foco conservado. Un
// cambio de viewport sólo sigue el cursor que ya se estaba viendo.
func (f *Folio) cursorEnVista() bool {
	if !f.enfocado || !f.conCursor {
		return false
	}
	x, y, alto, ok := f.caretEnPagina()
	off, vista := f.scroll.Offset, f.scroll.Size()
	return ok && vista.Width > 0 && vista.Height > 0 &&
		x >= off.X && x <= off.X+vista.Width && y >= off.Y && y+alto <= off.Y+vista.Height
}

type folioRenderer struct {
	f     *Folio
	fondo *canvas.Rectangle
}

func (r *folioRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *folioRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.f.envoltorio}
}
func (r *folioRenderer) Destroy() {}

func (r *folioRenderer) Layout(t fyne.Size) {
	seguirCursor := r.f.cursorEnVista()
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.f.envoltorio.Move(fyne.NewPos(0, 0))
	r.f.envoltorio.Resize(t)
	r.f.ajusteInicial(t.Width)
	if seguirCursor {
		r.f.asegurarVisible()
	}
}

func (r *folioRenderer) Refresh() {
	r.fondo.FillColor = r.f.lienzo()
	r.fondo.Refresh()
	r.f.pag.Refresh()
	r.f.scroll.Refresh()
}

// ---- la página ----

// pagina es lo que se desplaza: el aire de arriba, la regla, el papel con el texto, y el
// aire de abajo. Es la que recibe el teclado y el ratón (escribir.go).
type pagina struct {
	widget.BaseWidget
	f *Folio
}

// origen es la esquina de la caja de CONTENIDO del papel dentro de la página, en px de
// pantalla: el folio centrado, más la regla, más el margen.
func (p *pagina) origen() (x, y float32) {
	f := p.f
	z := f.escala()
	if f.caja != nil {
		return f.caja.Relleno * z, 0
	}
	ancho, _ := f.pagina.medidas()
	izq := max((p.Size().Width-ancho*z)/2, 0)
	arr := AireFolio
	if f.conRegla() {
		arr += AltoRegla * z
	}
	m := f.pagina.margenes()
	return izq + m[3]*z, arr + m[0]*z
}

func (p *pagina) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	f := p.f
	z := f.escala()
	if f.caja != nil {
		return fyne.NewSize(f.caja.Ancho*z, f.caja.Alto*z)
	}
	alto := 2*AireFolio + f.altoPapel()*z
	if f.conRegla() {
		alto += AltoRegla * z
	}
	ancho, _ := f.pagina.medidas()
	return fyne.NewSize(ancho*z, alto)
}

func (p *pagina) CreateRenderer() fyne.WidgetRenderer {
	return &paginaRenderer{
		p:      p,
		sombra: ui.NuevaSombraCon(editor.CapasSombraPagina, 0),
		papel:  canvas.NewRectangle(PapelBlanco),
		regla:  NuevaRegla(),
		caret:  canvas.NewRectangle(color.NRGBA{}),
	}
}

type paginaRenderer struct {
	p      *pagina
	sombra *ui.Sombra
	papel  *canvas.Rectangle
	regla  *Regla
	caret  *canvas.Rectangle
	dyn    []fyne.CanvasObject
}

func (r *paginaRenderer) MinSize() fyne.Size { return r.p.MinSize() }

func (r *paginaRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.dyn)+5)
	if r.p.f.caja != nil {
		return append(append(objs, r.dyn...), r.caret)
	}
	objs = append(objs, r.sombra.Objeto(), r.papel)
	if r.p.f.conRegla() {
		objs = append(objs, r.regla)
	}
	objs = append(objs, r.dyn...)
	return append(objs, r.caret)
}

func (r *paginaRenderer) Destroy() {}

func (r *paginaRenderer) Layout(t fyne.Size) {
	f := r.p.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if f.caja != nil {
		r.dyn = pintar(f, fyne.NewPos(0, 0), v)
		r.colocarCaret(v)
		return
	}
	z := f.escala()
	anchoPag, _ := f.pagina.medidas()
	izq := max((t.Width-anchoPag*z)/2, 0)
	arr := AireFolio
	if f.conRegla() {
		// La regla va pegada encima del folio y se desplaza con él, como en la web
		// (allí `sticky` no convive con el transform del zoom).
		r.regla.Move(fyne.NewPos(izq, arr))
		r.regla.Resize(fyne.NewSize(anchoPag*z, AltoRegla*z))
		// ponytail: la regla tiene un margen para los dos lados; el izquierdo.
		if m := f.pagina.margenes()[3]; r.regla.Margen != m || r.regla.Ancho != anchoPag {
			r.regla.Margen, r.regla.Ancho = m, anchoPag
			r.regla.Refresh()
		}
		arr += AltoRegla * z
		// Los marcadores siguen al párrafo del cursor (`selectedBlock()`).
		var ind [3]float32
		if f.deTexto(f.cur.b) {
			b := f.bloques[f.cur.b].Base
			ind = [3]float32{b.MargenIzq, b.Sangria1, b.MargenDer}
		}
		if ind != [3]float32{r.regla.Izq, r.regla.Primera, r.regla.Der} {
			r.regla.Izq, r.regla.Primera, r.regla.Der = ind[0], ind[1], ind[2]
			r.regla.Refresh()
		}
	}
	pos := fyne.NewPos(izq, arr)
	tam := fyne.NewSize(anchoPag*z, f.altoPapel()*z)
	r.sombra.Ajustar(pos, tam, v)
	r.papel.Move(pos)
	r.papel.Resize(tam)
	r.dyn = pintar(f, pos, v)
	r.colocarCaret(v)
}

// colocarCaret pone el cursor: una raya de 1 px (a zoom 100) en el `caret-color` de la
// página, que es el primario de la suite, del alto de la letra donde está.
func (r *paginaRenderer) colocarCaret(v tema.Variante) {
	f := r.p.f
	if !f.enfocado || !f.caretVisible || f.haySeleccion() || !f.editable() {
		r.caret.Hide()
		return
	}
	x, y, alto, ok := f.caretEnPagina()
	if !ok {
		r.caret.Hide()
		return
	}
	r.caret.FillColor = tema.Color("primary", v)
	r.caret.Move(fyne.NewPos(x, y))
	r.caret.Resize(fyne.NewSize(max(f.escala(), 1), alto))
	r.caret.Show()
	r.caret.Refresh()
}

func (r *paginaRenderer) Refresh() {
	r.papel.FillColor = PapelBlanco
	if c := r.p.f.pagina.Papel; c.A > 0 {
		r.papel.FillColor = c
	}
	r.Layout(r.p.Size())
	canvas.Refresh(r.p)
}

// ---- pintar ----

// pintar construye los objetos de la página: selección, resaltes, texto, subrayados,
// marcas de lista, barras de cita, reglas, bordes de tabla y cortes de página.
//
// ponytail: rehace todos los objetos en cada repintado. Fyne recorta lo que no se ve al
// pintar, así que lo que cuesta es crearlos; si un documento de cien páginas se nota al
// escribir, se reutilizan por línea.
func pintar(f *Folio, papel fyne.Position, v tema.Variante) []fyne.CanvasObject {
	z := f.escala()
	var objs []fyne.CanvasObject
	m := f.pagina.margenes()
	ox, oy := papel.X+m[3]*z, papel.Y+m[0]*z
	anchoPag, altoPag := f.pagina.medidas()
	n := int((f.altoPapel() + altoPag - 1) / altoPag)
	if f.caja != nil {
		ox, oy = f.pag.origen()
		n = 0 // una caja no tiene páginas: ni cortes ni número
	}

	// La marca de agua y el borde, detrás de todo lo demás, sobre el papel.
	objs = f.pintarDecoracion(objs, papel, z)
	// Los cortes: una raya de 2 px al final de cada alto de página, si hay más de una.
	for i := 1; i < n; i++ {
		r := canvas.NewRectangle(CortePagina)
		r.Move(fyne.NewPos(papel.X, papel.Y+(float32(i)*altoPag-2)*z))
		r.Resize(fyne.NewSize(anchoPag*z, 2*z))
		objs = append(objs, r)
	}
	// La numeración de Disposición: un solo pie con el total, centrado a 18 px del
	// borde de abajo, en `font-size: 11px`, `letter-spacing: 0.05em` y al 50 %.
	// ponytail: como en la web, el folio es continuo y hay UN pie, no uno por página.
	if f.pagina.Numerar && f.caja == nil {
		tinta := TintaContenido
		tinta.A = 128
		num := canvas.NewText(strconv.Itoa(max(n, 1)), tinta)
		num.TextSize, num.FontSource = 11*z, tema.Regular
		w := num.MinSize().Width
		num.Move(fyne.NewPos(papel.X+(anchoPag*z-w)/2, papel.Y+(f.altoPapel()-18-11*cajaFuente)*z))
		objs = append(objs, num)
	}

	// Las coincidencias de «Buscar», con los dos amarillos de la web.
	for i, m := range f.resaltes {
		if f.ocultos {
			break
		}
		col := editor.ResalteBuscar
		if i == f.activa {
			col = editor.ResalteActivo
		}
		for _, rc := range f.rectangulosEntre(punto{m.b, m.desde}, punto{m.b, m.hasta}) {
			r := canvas.NewRectangle(col)
			r.Move(fyne.NewPos(ox+rc.x*z, oy+rc.y*z))
			r.Resize(fyne.NewSize(rc.w*z, rc.h*z))
			objs = append(objs, r)
		}
	}
	for _, lista := range []struct {
		ms  []coincidencia
		col color.NRGBA
	}{{f.ancla, color.NRGBA{R: 0xbf, G: 0xdb, B: 0xfe, A: 0xff}}, {f.revisado, ColorRevisado}} {
		for _, m := range lista.ms {
			for _, rc := range f.rectangulosEntre(punto{m.b, m.desde}, punto{m.b, m.hasta}) {
				r := canvas.NewRectangle(lista.col)
				r.Move(fyne.NewPos(ox+rc.x*z, oy+rc.y*z))
				r.Resize(fyne.NewSize(rc.w*z, rc.h*z))
				objs = append(objs, r)
			}
		}
	}
	// La selección va DEBAJO del texto, como la del navegador.
	if f.haySeleccion() {
		sel := tema.Color("primary", v)
		sel.A = 64 // `color-mix(in srgb, var(--sytue-primary) 25%, transparent)`
		for _, rc := range f.rectangulosSeleccion() {
			r := canvas.NewRectangle(sel)
			r.Move(fyne.NewPos(ox+rc.x*z, oy+rc.y*z))
			r.Resize(fyne.NewSize(rc.w*z, rc.h*z))
			objs = append(objs, r)
		}
	}
	return f.pintarTiradoresTexto(f.pintarSeleccion(f.pintarContenido(objs, papel, ox, oy, z), ox, oy, z, v), v)
}

// pintarContenido añade las cajas del folio con su contenido en (ox, oy) de pantalla y el
// papel en `papel`. Las imágenes detrás del texto van sobre el papel y bajo todo lo demás;
// las de delante, encima de todo; las dos, en X e Y desde la esquina del papel. Lo usa
// también [Lectura] y [PintarCaja] (con sus propias `cajas`, sin tabla ni recuadro).
func (f *Folio) pintarContenido(objs []fyne.CanvasObject, papel fyne.Position, ox, oy, z float32) []fyne.CanvasObject {
	usadas := map[string]*canvas.Image{}
	objs = f.pintarAbsolutas(objs, papel, z, false, usadas)
	objs = f.pintarCajas(objs, f.cajas, ox, oy, z, usadas)
	objs = f.pintarAbsolutas(objs, papel, z, true, usadas)
	f.laminasMu.Lock()
	f.laminas = usadas
	f.laminasMu.Unlock()
	return objs
}

// pintarCajas añade los bloques ya maquetados, con sus tablas y recuadros, con el
// contenido en (ox, oy).
func (f *Folio) pintarCajas(objs []fyne.CanvasObject, cajas []caja, ox, oy, z float32, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	var tabla *tablaMaq
	var recuadro *recuadroMaq
	for _, c := range cajas {
		if c.tabla != nil && c.tabla != tabla {
			tabla = c.tabla
			objs = pintarTabla(objs, tabla, ox, oy, z)
		}
		if c.recuadro != nil && c.recuadro != recuadro {
			recuadro = c.recuadro
			objs = pintarRecuadro(objs, recuadro, ox, oy, z)
		}
		objs = f.pintarCaja(objs, c, ox, oy, z, usadas)
	}
	return objs
}

// pintarAbsolutas pinta las imágenes detrás (`delante` false) o delante del texto.
func (f *Folio) pintarAbsolutas(objs []fyne.CanvasObject, papel fyne.Position, z float32, delante bool, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	for _, c := range f.cajas {
		for _, l := range c.lineas {
			for _, p := range l.piezas {
				if p.img == nil || !p.img.Absoluta() || (p.img.Ajuste == "front") != delante {
					continue
				}
				// `left`/`top` son los de la caja de margen, y el recorte se come con márgenes
				// negativos: lo que se ve empieza justo ahí.
				w, h := visibleImagen(p.img, 0)
				objs = append(objs, f.lamina(usadas, p.img, papel.X+p.img.X*z, papel.Y+p.img.Y*z, w*z, h*z))
			}
		}
	}
	return objs
}

// lamina es lo que se ve de la imagen —recortada, si lo está— pintado en (x, y) de pantalla
// con ese tamaño. Se reutiliza de un repintado a otro —por su src y su recorte— para que
// Fyne no vuelva a subir la textura en cada tecla. Una que no se puede leer (de fuera, o
// rota) es una caja con el borde de la página.
func (f *Folio) lamina(usadas map[string]*canvas.Image, im *rico.Imagen, x, y, w, h float32) fyne.CanvasObject {
	var o fyne.CanvasObject
	if img := imagenRecortada(im); img != nil {
		o = f.reutilizar(usadas, im.Src+"\x00"+fmt.Sprint(im.Recorte), func() image.Image { return img })
	} else {
		r := canvas.NewRectangle(color.NRGBA{})
		r.StrokeColor, r.StrokeWidth = rico.TintaLinea, 1
		o = r
	}
	o.Move(fyne.NewPos(x, y))
	o.Resize(fyne.NewSize(w, h))
	return o
}

// pintarCaja añade los objetos de un bloque con su contenido en (ox, oy) de pantalla.
func (f *Folio) pintarCaja(objs []fyne.CanvasObject, c caja, ox, oy, z float32, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	rect := func(col color.Color, x, y, w, h float32) {
		r := canvas.NewRectangle(col)
		r.Move(fyne.NewPos(ox+x*z, oy+y*z))
		r.Resize(fyne.NewSize(w*z, h*z))
		objs = append(objs, r)
	}
	switch c.b.Tipo {
	case rico.Regla:
		tinta := color.Color(rico.TintaLinea)
		if c.b.Base.DeHoja() {
			tinta = c.b.Base.Color // el reset de Tailwind: `border-top: 1px`, en `currentColor`
		}
		rect(tinta, c.x+c.b.Base.MargenIzq, c.y, c.ancho+c.izq-c.x-c.b.Base.MargenIzq, 1)
		return objs
	case rico.SaltoPagina:
		// `border-top: 2px dashed #cbd5e1`: Chromium pinta trazos de tres veces el grosor
		// con el mismo hueco.
		ancho := c.ancho + c.izq - c.x
		for x := float32(0); x < ancho; x += 12 {
			rect(color.NRGBA{R: 0xcb, G: 0xd5, B: 0xe1, A: 0xff}, c.x+x, c.y, min(6, ancho-x), 2)
		}
		return objs
	case rico.Figura:
		return f.pintarFigura(objs, c, ox, oy, z, usadas)
	case rico.Cita:
		rect(rico.TintaLinea, c.izq-barraCita-aireCita, c.y, barraCita, c.alto)
	case rico.Parrafo, rico.Titulo:
	}
	if c.b.Region != "" {
		if c.b.Oculta {
			return objs // «Primera página distinta»: `display: none`
		}
		objs = f.pintarRegion(objs, c, ox, oy, z)
	}
	if c.b.Marca == "•" && len(c.lineas) > 0 {
		objs = append(objs, pintarVineta(c, ox, oy, z))
	}
	if c.marcaTexto != "" && len(c.lineas) > 0 {
		l := c.lineas[0]
		t := textoContenido(c.marcaTexto, c.marcaEstilo, z)
		base := c.y + c.arribaTexto + l.base
		t.Move(fyne.NewPos(ox+(c.izq-c.marcaAncho)*z, oy+(base-ascFuente*c.marcaEstilo.Tamano)*z))
		objs = append(objs, t)
	}
	return f.pintarLineas(objs, c, ox, oy, z, usadas)
}

// pintarLineas añade el texto y las imágenes de las líneas de un bloque, con sus fondos,
// subrayados y tachados.
func (f *Folio) pintarLineas(objs []fyne.CanvasObject, c caja, ox, oy, z float32, usadas map[string]*canvas.Image) []fyne.CanvasObject {
	rect := func(col color.Color, x, y, w, h float32) {
		r := canvas.NewRectangle(col)
		r.Move(fyne.NewPos(ox+x*z, oy+y*z))
		r.Resize(fyne.NewSize(w*z, h*z))
		objs = append(objs, r)
	}
	for _, l := range c.lineas {
		top := c.y + c.arribaTexto + l.y
		for _, fm := range l.flotas {
			objs = append(objs, f.lamina(usadas, fm.img, ox+(c.izq+fm.x)*z, oy+(c.y+c.arribaTexto+fm.y)*z, fm.w*z, fm.h*z))
		}
		for _, p := range l.piezas {
			x := c.izq + l.x0 + p.x
			if p.img != nil {
				if !p.img.Absoluta() && !p.img.Rodeada() {
					objs = append(objs, f.lamina(usadas, p.img, ox+x*z, oy+(top+p.arriba)*z, p.ancho*z, p.alto*z))
				}
				continue
			}
			base := top + l.base + desplazamientoVertical(p.est)
			e := p.est
			if e.Fondo.A > 0 {
				// El fondo de un tramo en línea cubre la caja de su LETRA, no la línea.
				rect(e.Fondo, x, base-ascFuente*e.Tamano, p.ancho, cajaFuente*e.Tamano)
			}
			if strings.TrimSpace(p.texto) != "" {
				t := textoContenido(p.texto, e, z)
				t.Move(fyne.NewPos(ox+x*z, oy+(base-ascFuente*e.Tamano)*z))
				objs = append(objs, t)
			}
			grosor := max(e.Tamano/16, 1)
			if e.Subrayado {
				rect(e.Color, x, base+max(e.Tamano*0.1, 1), p.ancho, grosor)
			}
			if e.Tachado {
				rect(e.Color, x, base-e.Tamano*0.3, p.ancho, grosor)
			}
		}
	}
	return objs
}

// pintarVineta dibuja la viñeta de un elemento de lista como Chromium: un disco en el
// primer nivel, un círculo en el segundo y un cuadrado del tercero en adelante
// (`disc`, `circle`, `square`), de diámetro `(ascendente·2/3 + 1)/2` —5,57 px a 16 px,
// medido 5,6 en producción—, centrado 0,85 em a la izquierda del texto y 0,36 em por
// encima de su línea base (medido: 14,5 y 5,75 px con la letra de 16).
func pintarVineta(c caja, ox, oy, z float32) fyne.CanvasObject {
	e := c.marcaEstilo
	d := (ascFuente*e.Tamano*2/3 + 1) / 2
	l := c.lineas[0]
	cx := c.izq - 0.85*e.Tamano
	cy := c.y + c.arribaTexto + l.base - 0.36*e.Tamano
	pos := fyne.NewPos(ox+(cx-d/2)*z, oy+(cy-d/2)*z)
	tam := fyne.NewSize(d*z, d*z)
	var o fyne.CanvasObject
	switch {
	case c.b.Sangria >= 3:
		o = canvas.NewRectangle(e.Color)
	case c.b.Sangria == 2:
		k := canvas.NewCircle(color.NRGBA{})
		k.StrokeColor, k.StrokeWidth = e.Color, max(z, 1)
		o = k
	default:
		o = canvas.NewCircle(e.Color)
	}
	o.Move(pos)
	o.Resize(tam)
	return o
}

// pintarTabla dibuja el fondo de cada celda y la rejilla, con los bordes de 1 px de
// `border-collapse`: cada celda pone el de su izquierda y el de arriba, y la tabla cierra
// la derecha y abajo. El texto de las celdas lo pintan sus cajas, encima.
func pintarTabla(objs []fyne.CanvasObject, tm *tablaMaq, ox, oy, z float32) []fyne.CanvasObject {
	rect := func(col color.Color, x, y, w, h float32) {
		r := canvas.NewRectangle(col)
		r.Move(fyne.NewPos(ox+x*z, oy+y*z))
		r.Resize(fyne.NewSize(max(w*z, 1), max(h*z, 1)))
		objs = append(objs, r)
	}
	for _, cm := range tm.celdas {
		if cm.c.Fondo.A > 0 {
			rect(cm.c.Fondo, cm.x, cm.y, cm.w, cm.h)
		}
	}
	if tm.sinBordes {
		return objs
	}
	for _, cm := range tm.celdas {
		rect(rico.TintaLinea, cm.x, cm.y, cm.w, 1)
		rect(rico.TintaLinea, cm.x, cm.y, 1, cm.h)
	}
	rect(rico.TintaLinea, tm.ancho, tm.y, 1, tm.alto)
	rect(rico.TintaLinea, 0, tm.y+tm.alto-1, tm.ancho, 1)
	return objs
}

// pintarRecuadro dibuja el `border: 1px dashed var(--page-rule)` del índice. Chromium
// pinta los trazos de un borde discontinuo de tres veces su grosor, con el mismo hueco.
func pintarRecuadro(objs []fyne.CanvasObject, r *recuadroMaq, ox, oy, z float32) []fyne.CanvasObject {
	const trazo = 3
	rect := func(x, y, w, h float32) {
		o := canvas.NewRectangle(rico.TintaLinea)
		o.Move(fyne.NewPos(ox+x*z, oy+y*z))
		o.Resize(fyne.NewSize(max(w*z, 1), max(h*z, 1)))
		objs = append(objs, o)
	}
	for x := float32(0); x < r.ancho; x += 2 * trazo {
		rect(x, r.y, min(trazo, r.ancho-x), 1)
		rect(x, r.y+r.alto-1, min(trazo, r.ancho-x), 1)
	}
	for y := float32(0); y < r.alto; y += 2 * trazo {
		rect(0, r.y+y, 1, min(trazo, r.alto-y))
		rect(r.ancho-1, r.y+y, 1, min(trazo, r.alto-y))
	}
	return objs
}

// textoContenido es un `canvas.Text` con la letra del CONTENIDO: Figtree en su cara,
// al tamaño del tramo por el zoom, en su color.
func textoContenido(s string, e rico.Estilo, z float32) *canvas.Text {
	t := canvas.NewText(s, e.Color)
	t.TextSize = e.Tamano * z
	t.FontSource = caraDe(e)
	t.Resize(t.MinSize())
	return t
}
