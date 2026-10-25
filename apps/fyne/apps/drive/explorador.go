// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Canalones de la zona de contenido, medidos: la lista va con `px-3` (10,5) y la
// rejilla con `px-6 py-4` (21 / 14).
const (
	CanalonLista   float32 = 3 * 0.25 * tema.Rem
	CanalonRejilla float32 = 6 * 0.25 * tema.Rem
	RellenoRejilla float32 = 4 * 0.25 * tema.Rem
	// HuecoRejilla es el `gap-4` entre tarjetas.
	HuecoRejilla float32 = 4 * 0.25 * tema.Rem
)

// CortesRejillaDrive es `grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5` de
// la vista de tarjetas. El 0 hereda del corte anterior, como una variante no declarada.
var CortesRejillaDrive = ui.Cortes{2, 3, 0, 4, 5, 0}

// Explorador es la pantalla del explorador de ficheros: la de FileBrowser.tsx.
//
// El estado es público y la pantalla es una FUNCIÓN de él: `Objeto` no pide nada a la
// red. Así el guardián de píxel monta cualquiera de las siete fases sin servicio, y la
// carga vive en `Cargar`, que es lo único que habla con Sytue Drive.
type Explorador struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Fase es en cuál de los estados está.
	Fase Fase
	// Ruta son las carpetas desde la raíz hasta la actual, sin incluir «Mi unidad»,
	// que la pinta la primera miga.
	Ruta []Elemento
	// Elementos es lo que se enseña: carpetas primero y ficheros después.
	Elementos []Elemento
	// Seleccion son los ID marcados.
	Seleccion map[string]bool
	// Rejilla es la vista de tarjetas (`view === 'grid'`).
	Rejilla bool
	// Consulta es lo que hay escrito en el buscador, y EnBusqueda si los elementos
	// que se enseñan son su resultado.
	Consulta   string
	EnBusqueda bool
	// Subida es el progreso de la subida en curso; nil, no hay ninguna.
	Subida *Progreso
	// Ventana es el ancho de la VENTANA en px de CSS: decide los cortes `sm:`/`md:`.
	Ventana float32

	// HayMas dice si el servicio tiene otra página de esta carpeta (`nextCursor`), y
	// CargandoMas si ya se está pidiendo.
	HayMas, CargandoMas bool
	// Ocupado es el `busy` de la web: hay una mutación en curso y «Nuevo» y «Subir» se
	// apagan hasta que acabe.
	Ocupado bool
	// Desconectado es que el monitor de red dice que no hay: lo cargado se sigue viendo
	// (el contrato offline), pero las mutaciones se apagan igual que en SinConexion.
	Desconectado bool

	// Enganches. Los rellena quien monta la app; nil es «esta acción no está».
	AlAbrir                               func(Elemento)
	AlNavegar                             func(nivel int) // -1 es la raíz
	AlBuscar                              func(string)
	AlNuevaCarpeta, AlSubir, AlReintentar func()
	AlCambiarVista                        func(bool)
	AlSeleccionar                         func(id string)
	AlSeleccionarTodo                     func()
	AlDescargarLote, AlPapeleraLote       func()
	AlMoverLote                           func()
	// AlCargarMas pide la página siguiente. Lo llama la lista al pintar una de sus
	// últimas filas y el botón «Cargar más» de la rejilla.
	AlCargarMas func()
	Refresco    ui.RefrescoLista
	// AlMoverA mueve lo arrastrado a una carpeta soltándolo encima; nil, no se arrastra.
	AlMoverA func(origenes []Elemento, destino Elemento)
	// AlMenu abre el menú de acciones de un elemento —el «⋯» de la fila o de la
	// tarjeta— colgado de `disparador`. AlNuevo abre el de «Nuevo», colgado del botón.
	AlMenu  func(el Elemento, disparador fyne.CanvasObject)
	AlNuevo func(disparador fyne.CanvasObject)
	// Miniaturas son las de las imágenes ya traídas, por id; un nil guardado es que
	// no la hay (el servicio dijo que no), y no se vuelve a pedir. AlMiniatura la pide
	// cuando una tarjeta de imagen se pinta sin ella: la web la pide `lazy`, solo de lo
	// que se ve.
	Miniaturas  map[string]image.Image
	AlMiniatura func(el Elemento)

	// Las piezas que SOBREVIVEN al repintado. La pantalla se rehace entera con cada
	// cambio de estado, y rehacer estas tres rompía lo que la persona tenía entre
	// manos: el buscador perdía el foco en la primera letra (cada pulsación repinta),
	// y la lista volvía arriba al marcar una casilla o cada 250 ms durante una subida.
	buscador     *ui.Entrada
	filas        *widget.List
	filasObj     fyne.CanvasObject
	filasCasilla bool
	// filasLineas son las líneas bajo el nombre con las que se creó la lista: una fila
	// con dos crece, y `widget.List` toma el alto de la plantilla.
	filasLineas  int
	desplazable  *container.Scroll
	anchoRejilla float32
	// deLista es lo que enseña la LISTA en este pintado: todo, o lo que no se ha ido a
	// la franja de carpetas (ver Partes). franjaScroll es el desplazable de la franja,
	// el mismo de un pintado a otro, y anchoFranja el último ancho de su rejilla.
	deLista      []Elemento
	franjaScroll *container.Scroll
	anchoFranja  float32
	// rueda es la de la franja de progreso: la MISMA de un repintado a otro, que se
	// repinta cuatro veces por segundo y una rueda nueva cada vez no llegaría a girar.
	rueda *ui.Rueda
	// marcados es Marcados() de este pintado, para no recontar en cada fila.
	marcados int
	// arrastrando es lo que viaja en un arrastre y destino la carpeta sobre la que se
	// soltaría (su id; vacío, ninguna que lo acepte).
	arrastrando []Elemento
	destino     string
	// tarjetas son las de la rejilla de este pintado, en su orden, con lo que está en el
	// lienzo (`objeto`: la tarjeta o la que la embebe para arrastrarse) para saber dónde
	// cae el puntero, y la tarjeta en sí para marcar el destino sin repintar la rejilla.
	tarjetas []tarjetaPintada
	// arriba pide volver al principio en el próximo pintado: una carpeta NUEVA
	// empieza arriba, y la misma recargada o ampliada se queda donde estaba.
	arriba bool
}

// FilasAntesDelFinal es a cuántas filas del final se pide la página siguiente: los
// 600 px de margen de la web (`scrollHeight - 600`) entre los 51 de una fila.
const FilasAntesDelFinal = 12

// Partes reparte lo que se enseña entre la franja de teselas y la lista: es el
// `partirEnTeselas` de la web (`apps/drive/src/lib/teselas.ts`). En la forma Marco, en la
// vista de lista y sin búsqueda, las carpetas salen ARRIBA como una franja de teselas de
// color pleno y lo demás sigue en la lista; cada parte conserva el orden en que llegó.
// En Carril, en la cuadrícula, en una búsqueda —sus resultados vienen de todo el árbol—
// o sin carpetas no se parte nada.
func (e *Explorador) Partes() (carpetas, resto []Elemento) {
	if !tema.EsMarco() || e.Rejilla || e.EnBusqueda {
		return nil, e.Elementos
	}
	resto = []Elemento{} // vacía y no nil: una carpeta con solo carpetas deja la lista sin filas
	for _, el := range e.Elementos {
		if el.Carpeta {
			carpetas = append(carpetas, el)
		} else {
			resto = append(resto, el)
		}
	}
	if len(carpetas) == 0 {
		return nil, e.Elementos
	}
	return carpetas, resto
}

// filasDeLista es lo que enseña la lista: lo que repartió el último pintado o, si aún no
// ha pintado, todo.
func (e *Explorador) filasDeLista() []Elemento {
	if e.deLista == nil {
		return e.Elementos
	}
	return e.deLista
}

// EnOrden es todo en el orden en que se VE: primero la franja y después la lista. Son
// dos piezas en pantalla y UNA sola serie para lo que cuenta posiciones: el rango de
// Mayús + clic y «seleccionar todo».
func (e *Explorador) EnOrden() []Elemento {
	carpetas, resto := e.Partes()
	if len(carpetas) == 0 {
		return resto
	}
	return append(carpetas, resto...)
}

// AlPrincipio hace que el próximo pintado enseñe la lista desde arriba. Lo llama quien
// cambia de carpeta o de búsqueda, no quien recarga la misma.
func (e *Explorador) AlPrincipio() { e.arriba = true }

// NuevoExplorador crea la pantalla en su estado inicial: cargando.
func NuevoExplorador(tr *i18n.Translator) *Explorador {
	return &Explorador{tr: tr, loc: tr.Locale(), Seleccion: map[string]bool{}}
}

func (e *Explorador) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return e.tr.T(clave, v)
}

// SinRed dice si la pantalla está en la fase de sin conexión, que es la que apaga
// TODAS las mutaciones: subir, crear, mover, renombrar, papelera y favoritos.
//
// Es el contrato de `docs/roadmap.md` §«Contrato offline»: se navega por lo ya
// consultado y se abre lo descargado, y nada se encola. Una subida a medias que se
// reintenta sola es como se duplica un fichero.
func (e *Explorador) SinRed() bool { return e.Fase == SinConexion || e.Desconectado }

// Subiendo dice si hay una SUBIDA en curso (una descarga también pinta la franja, pero
// no hace girar «Subir»).
func (e *Explorador) Subiendo() bool { return e.Subida != nil && !e.Subida.Bajada }

// Marcados es cuántos elementos hay seleccionados.
func (e *Explorador) Marcados() int {
	n := 0
	for _, s := range e.Seleccion {
		if s {
			n++
		}
	}
	return n
}

// TodoMarcado y AlgoMarcado son el `allSelected`/`someSelected` de la web: el segundo
// solo cuenta si el primero es falso, que es lo que hace la casilla indeterminada.
func (e *Explorador) TodoMarcado() bool {
	return len(e.Elementos) > 0 && e.Marcados() == len(e.Elementos)
}

// AlgoMarcado es el tercer estado de la casilla de la cabecera.
func (e *Explorador) AlgoMarcado() bool { return e.Marcados() > 0 && !e.TodoMarcado() }

// Objeto arma la pantalla entera: cabecera, barras y cuerpo, en una columna donde el
// cuerpo es el que crece.
func (e *Explorador) Objeto(v tema.Variante) fyne.CanvasObject {
	e.marcados = e.Marcados()
	cab := e.cabecera(v)
	franjas := []fyne.CanvasObject{cab}
	elastico := 1
	if n := e.Marcados(); n > 0 && !e.SinRed() {
		franjas = append(franjas, e.barraSeleccion(v, n))
		elastico++
	}
	if e.Subida != nil {
		franjas = append(franjas, e.barraSubida(v))
		elastico++
	}
	franjas = append(franjas, e.cuerpo(v))
	pila := container.New(Pila{Elastico: elastico}, franjas...)
	if c, ok := cab.(*fyne.Container); ok {
		if cv, ok := c.Layout.(*CabeceraVista); ok {
			cv.AlCambiarAlto = pila.Refresh
		}
	}
	return pila
}

// ---- La cabecera ----

func (e *Explorador) cabecera(v tema.Variante) fyne.CanvasObject {
	// La raíz es el PRIMER segmento de la ruta, no el nombre del `<nav>`: «Mi unidad»
	// se pulsa para volver al principio. El nombre que recibe `NuevasMigas` es el
	// `aria-label`, que en la web es «Ruta». Pasarle la ruta sin la raíz dejaba la
	// cabecera sin ningún segmento cuando se está en el primer nivel —o sea, casi
	// siempre—, y se vio mirando el PNG.
	nombres := make([]string, 0, len(e.Ruta)+1)
	nombres = append(nombres, e.t("nav.myDrive"))
	for _, c := range e.Ruta {
		nombres = append(nombres, c.Nombre)
	}
	migas := ui.NuevasMigas(e.t("files.breadcrumb"), nombres, func(i int) {
		if e.AlNavegar != nil {
			// El índice 0 es la raíz; los demás, la carpeta i-1 de la ruta.
			e.AlNavegar(i - 1)
		}
	})

	buscador := e.campoBusqueda()

	// «Nuevo» es el disparador de un menú —carpeta, documento, hoja, presentación—, con
	// su `chevronDown` de 14 detrás y el `gap-1.5` de su clase escrita a mano: 76,25
	// medidos en producción.
	var nuevo *ui.Boton
	nuevo = ui.NuevoBoton(e.t("files.new"), func() {
		if e.AlNuevo != nil {
			e.AlNuevo(nuevo)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	nuevo.IconoDetras, nuevo.TamIcono, nuevo.Hueco = iconos.ChevronAbajo, LadoGalonNuevo, HuecoNuevo
	nuevo.Inerte = e.SinRed() || e.Ocupado
	// Medido: 71 de ancho con la flecha, y sin ella salía de 50. La rueda solo gira al
	// SUBIR; lo demás que ocupa (crear, copiar, borrar) solo lo apaga.
	subir := ui.NuevoBoton(e.t("files.upload"), e.AlSubir).Con(ui.Primario, ui.TallaSM)
	subir.Inerte = e.SinRed() || (e.Ocupado && !e.Subiendo())
	subir.Cargando = e.Subiendo()
	conGlifo(subir, glifoSubir, v)

	conmutador := NuevoConmutadorVista(e.t("files.viewList"), e.t("files.viewGrid"), e.AlCambiarVista)
	conmutador.Rejilla = e.Rejilla

	c := &CabeceraVista{
		Ruta:       migas,
		Acciones:   []fyne.CanvasObject{buscador, nuevo, subir},
		Conmutador: conmutador,
		Ventana:    e.Ventana,
	}
	return c.Objeto(v)
}

// campoBusqueda es el buscador de la cabecera, el MISMO de un pintado a otro: cada
// letra repinta la pantalla, y con un campo nuevo en cada vuelta el foco se perdía en
// la primera.
func (e *Explorador) campoBusqueda() *ui.Entrada {
	if e.buscador != nil {
		return e.buscador
	}
	b := ui.NuevaEntrada(e.t("files.search")).Con(ui.CampoCromo)
	b.Ancho = AnchoBuscador
	b.Icono = iconos.Buscar
	b.PonerTexto(e.Consulta)
	// Por el campo y no con `e.AlBuscar` directamente: el enganche se pone DESPUÉS de
	// crear la pantalla, y el valor que se copiara aquí sería nil.
	b.AlCambiar(func(s string) {
		if e.AlBuscar != nil {
			e.AlBuscar(s)
		}
	})
	e.buscador = b
	return b
}

// LadoGalonNuevo es el `chevronDown` de 14 de «Nuevo» y HuecoNuevo su `gap-1.5`.
const (
	LadoGalonNuevo float32 = 14
	HuecoNuevo     float32 = 1.5 * 0.25 * tema.Rem
)

// conGlifo le pone a un botón su `leadingIcon={<Glyph … size={16} />}`.
//
// El glifo es de Drive y no del catálogo, así que el botón no lo tiñe: se le da ya con
// la tinta de su rótulo —`on-primary` en los rellenos, `text` en los demás— y a media
// fuerza si está apagado, como el rótulo. Se llama DESPUÉS de fijar `Inerte`.
func conGlifo(b *ui.Boton, cuerpo string, v tema.Variante) *ui.Boton {
	token := "text"
	if b.Variante == ui.Primario || b.Variante == ui.Peligro {
		token = "on-primary"
	}
	tinta := tema.Color(token, v)
	if b.Inerte {
		tinta.A /= 2
	}
	b.TamIcono = iconos.TallaSM
	b.Glifo = glifoCon(cuerpo, iconos.TallaSM, tinta)
	return b
}

// barraSeleccion es la franja de acciones en lote: `ui.BarraDeLote` con las dos
// acciones de Drive y su canalón (`px-4 sm:px-6`).
//
// Lleva «Mover», «Descargar» y «Papelera», en el orden de la web.
//
// ponytail: falta «Combinar PDF» (dos o más PDF y nada más), que pide la operación de
// páginas del servicio de PDF y entra con el organizador de páginas.
func (e *Explorador) barraSeleccion(v tema.Variante, n int) fyne.CanvasObject {
	acciones := []*ui.Boton{
		conGlifo(ui.NuevoBoton(e.t("files.selection.move"), e.AlMoverLote).Con(ui.Secundario, ui.TallaSM), glifoMover, v),
		conGlifo(ui.NuevoBoton(e.t("files.selection.download"), e.AlDescargarLote).Con(ui.Secundario, ui.TallaSM), glifoDescargar, v),
		conGlifo(ui.NuevoBoton(e.t("files.selection.trash"), e.AlPapeleraLote).Con(ui.Peligro, ui.TallaSM), glifoPapelera, v),
	}
	recuento := e.t("files.selection.count", i18n.Vars{"count": n})
	return ui.NuevaBarraDeLote(recuento, e.t("ui.bulk.region"), e.t("files.selection.clear"),
		acciones, e.limpiarSeleccion).Con(CanalonCabecera(e.Ventana))
}

func (e *Explorador) limpiarSeleccion() {
	clear(e.Seleccion)
	if e.AlSeleccionar != nil {
		e.AlSeleccionar("")
	}
}

// barraSubida es el progreso: `border-b bg-bg-subtle px-4 py-2 sm:px-6`, la rueda de
// 16 y el texto en `text-sm` con `gap-3`, y la barra de `h-1` con `mt-1.5`, redondeada.
//
// Una DESCARGA pinta la misma franja con el nombre del fichero y nada más: en la web
// la descarga la enseña el navegador —con el nombre y su barra—, y aquí no hay
// navegador que lo haga. Escribir «Subiendo 1/1» encima de una descarga, que es lo que
// hacía, es mentir sobre lo que pasa.
func (e *Explorador) barraSubida(v tema.Variante) fyne.CanvasObject {
	p := e.Subida.Instantanea()
	escrito := p.Nombre
	if !e.Subida.Bajada {
		escrito = e.t("files.uploadProgress.text", i18n.Vars{
			"current": p.Actual, "total": p.Total, "name": p.Nombre,
		})
	}
	texto := ui.Texto(escrito, tema.TextoSM, ui.Normal, tema.Color("text", v))
	canal := canvas.NewRectangle(tema.Color("border", v))
	canal.CornerRadius = tema.RadioPill
	relleno := canvas.NewRectangle(tema.Color("primary", v))
	relleno.CornerRadius = tema.RadioPill
	if e.rueda == nil {
		e.rueda = ui.NuevaRueda(LadoRuedaSubida, "text", v)
	}
	return container.New(&filaSubida{
		ventana: e.Ventana, fraccion: p.Fraccion(), texto: texto, rueda: e.rueda, canal: canal, relleno: relleno,
	},
		canvas.NewRectangle(tema.Color("bg-subtle", v)),
		canvas.NewRectangle(tema.Color("border", v)),
		e.rueda, texto, canal, relleno)
}

// LadoRuedaSubida es el `<Spinner size={16} />` de la franja de progreso y HuecoRuedaSubida
// su `gap-3` con el texto.
const (
	LadoRuedaSubida  float32 = 16
	HuecoRuedaSubida float32 = 3 * 0.25 * tema.Rem
)

// filaSubida coloca la franja de progreso.
type filaSubida struct {
	ventana, fraccion float32
	texto             fyne.CanvasObject
	rueda             fyne.CanvasObject
	canal, relleno    *canvas.Rectangle
}

var _ fyne.Layout = (*filaSubida)(nil)

// Medidas de la franja: `py-2`, el texto de `text-sm`, `mt-1.5` y la barra de `h-1`.
const (
	rellenoSubida float32 = 2 * 0.25 * tema.Rem
	separaBarra   float32 = 1.5 * 0.25 * tema.Rem
	altoBarra     float32 = 1 * 0.25 * tema.Rem
)

// AltoBarraSubida es lo que ocupa la franja de progreso, con su `border-b` dentro.
const AltoBarraSubida float32 = 2*rellenoSubida + 1.125*tema.Rem + separaBarra + altoBarra + tema.Borde

func (f *filaSubida) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, AltoBarraSubida)
}

func (f *filaSubida) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) >= 2 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
		objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	}
	canalon := CanalonCabecera(f.ventana)
	ancho := max(t.Width-2*canalon, 0)
	// `flex items-center gap-3`: la rueda de 16 centrada en la línea de 17,5 del texto.
	f.rueda.Move(fyne.NewPos(canalon, rellenoSubida+(tema.TextoSM.Linea-LadoRuedaSubida)/2))
	f.rueda.Resize(fyne.NewSize(LadoRuedaSubida, LadoRuedaSubida))
	x := canalon + LadoRuedaSubida + HuecoRuedaSubida
	if txt, ok := f.texto.(*canvas.Text); ok {
		ui.Truncar(txt, txt.Text, tema.TextoSM, ui.Normal, max(canalon+ancho-x, 0))
		ui.ColocarTexto(txt, tema.TextoSM, x, rellenoSubida)
	}
	y := rellenoSubida + tema.TextoSM.Linea + separaBarra
	f.canal.Move(fyne.NewPos(canalon, y))
	f.canal.Resize(fyne.NewSize(ancho, altoBarra))
	f.relleno.Move(fyne.NewPos(canalon, y))
	f.relleno.Resize(fyne.NewSize(ancho*min(max(f.fraccion, 0), 1), altoBarra))
}

// ---- El cuerpo ----

func (e *Explorador) cuerpo(v tema.Variante) fyne.CanvasObject {
	switch e.Fase {
	case Cargando:
		return e.esqueletoCuerpo(v)
	case Vacia:
		return e.vacio(v)
	case Fallo, SinSesion, Prohibida, SinConexion:
		return e.problema(v)
	case Exito:
	}
	if e.Rejilla {
		return e.rejilla(v)
	}
	return e.lista(v)
}

// vacio es el estado sin elementos. La búsqueda tiene su propio texto: decirle «esta
// carpeta está vacía» a quien acaba de buscar en TODO el árbol es mentirle.
func (e *Explorador) vacio(v tema.Variante) fyne.CanvasObject {
	if e.EnBusqueda {
		x := ui.NuevoVacio(
			e.t("files.search.emptyTitle", i18n.Vars{"query": strings.TrimSpace(e.Consulta)}),
			e.t("files.search.emptyBody"))
		x.Ilustracion = ilustracion(iconos.VacioBusqueda, v)
		return centrar(x, v)
	}
	x := ui.NuevoVacio(e.t("files.empty.title"), e.t("files.empty.body"))
	x.Ilustracion = ilustracion(iconos.VacioDrive, v)
	// La web lo pinta también sin red, apagado (`disabled={offline}`): el botón dice
	// qué se haría aquí aunque ahora no se pueda.
	cta := ui.NuevoBoton(e.t("files.empty.cta"), e.AlSubir).Con(ui.Primario, ui.TallaSM)
	cta.Inerte = e.SinRed()
	x.Accion = conGlifo(cta, glifoSubir, v)
	return centrar(x, v)
}

// AnchoIlustracion es el `size = 160` por defecto de Illustration.tsx: el tope de
// ancho del dibujo, que conserva su proporción.
const AnchoIlustracion float32 = 160

// ilustracion es `<Illustration name=… />` a su ancho por defecto.
func ilustracion(i iconos.Ilustracion, v tema.Variante) fyne.CanvasObject {
	return iconos.Pintar(i, v).A(AnchoIlustracion).Objeto()
}

// problema pinta las cuatro fases que no son datos: fallo de red, sesión caducada,
// permiso denegado y sin conexión. Cada una dice qué hacer, que es lo que las separa.
func (e *Explorador) problema(v tema.Variante) fyne.CanvasObject {
	titulo, cuerpo, rotulo := e.textoProblema()
	x := ui.NuevoVacio(titulo, cuerpo)
	x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
	if e.Fase == SinConexion {
		// Sin red no se ofrece reintentar: el botón fallaría igual y enseña a
		// desconfiar de los botones.
		x.Ilustracion = iconos.Trazo(iconos.Nube, 40, "text-subtle", v).Objeto()
		return centrar(x, v)
	}
	if rotulo != "" && e.AlReintentar != nil {
		variante := ui.Secundario
		if e.Fase == SinSesion {
			variante = ui.Primario
		}
		x.Accion = ui.NuevoBoton(rotulo, e.AlReintentar).Con(variante, ui.TallaSM)
	}
	return centrar(x, v)
}

func (e *Explorador) textoProblema() (titulo, cuerpo, accion string) {
	switch e.Fase {
	case SinSesion:
		return e.t("files.error.sessionTitle"), e.t("files.error.sessionBody"), e.t("files.error.sessionCta")
	case Prohibida:
		// Del catálogo COMÚN: los diez servicios contestan lo mismo y la suite lo
		// dice una vez, no diez veces distinto.
		return e.tr.T("forbidden.title", nil), e.tr.T("forbidden.description", nil), ""
	case SinConexion:
		return e.tr.T("offline.status", nil), e.tr.T("offline.description", nil), ""
	case Cargando, Exito, Vacia, Fallo:
	}
	if e.EnBusqueda {
		return e.t("files.search.errorTitle"), e.t("files.error.network"), e.t("files.retry")
	}
	return e.t("files.error.loadTitle"), e.t("files.error.network"), e.t("files.retry")
}

// centrar es el `grid h-full place-items-center p-6` con el que la web centra los
// estados vacíos en el hueco que queda.
func centrar(x *ui.Vacio, v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("bg", v))
	return container.New(&centrado{hijo: x}, fondo, x)
}

type centrado struct{ hijo *ui.Vacio }

var _ fyne.Layout = (*centrado)(nil)

func (c *centrado) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (c *centrado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	ancho := max(t.Width-2*tema.Espacio(6), 0)
	alto := c.hijo.MedidaVacio(ancho)
	c.hijo.Move(fyne.NewPos(tema.Espacio(6), max((t.Height-alto)/2, 0)))
	c.hijo.Resize(fyne.NewSize(ancho, alto))
}

// ---- La lista ----

// lista es la cabecera de columnas más el listado virtualizado.
func (e *Explorador) lista(v tema.Variante) fyne.CanvasObject {
	cab := ui.NuevaCabeceraDrive(e.t("files.col.name"), e.t("files.col.size"), e.t("files.col.modified"))
	cab.ConCasilla = !e.SinRed()
	cab.Todas, cab.Algunas = e.TodoMarcado(), e.AlgoMarcado()
	cab.Ventana = e.Ventana
	cab.AlMarcarTodas = e.AlSeleccionarTodo
	var carpetas []Elemento
	carpetas, e.deLista = e.Partes()

	// `ConCasilla` se decide al CREAR la fila y no al rellenarla: el renderer monta la
	// casilla en `CreateRenderer`, así que ponerla en `UpdateItem` no la añade nunca y
	// la fila se queda 24,5 px a la izquierda de donde la pinta la web. Sale del mismo
	// sitio para todas porque depende de la conexión, no del elemento.
	//
	// La casilla se pinta SIEMPRE, y en la web solo aparece al pasar por encima
	// (`opacity-0 group-hover:opacity-100`) aunque ocupe su sitio desde el principio.
	// Se elige reservar el hueco y pintarla: la geometría es el mandato, y una fila
	// sin el hueco desplaza el nombre, la tesela y las dos columnas. Anotado en el
	// informe como hueco de `ui.Eleccion`, que no tiene opacidad.
	conCasilla := !e.SinRed()
	lineas := 0
	if e.EnBusqueda {
		lineas++
	}
	if e.Ventana > 0 && e.Ventana < ui.CorteSM {
		lineas++
	}
	if e.filas == nil || e.filasCasilla != conCasilla || e.filasLineas != lineas {
		e.filas = widget.NewList(
			func() int { return len(e.filasDeLista()) },
			func() fyne.CanvasObject {
				// Con ratón la fila se arrastra a una carpeta; con el dedo, no: una fila
				// que se queda el arrastre no deja desplazar la lista.
				var o fyne.CanvasObject
				var f *ui.FilaDrive
				if punteroGrueso() || e.AlMoverA == nil {
					f = ui.NuevaFilaDrive("", "")
					o = f
				} else {
					a := &filaArrastrable{e: e}
					a.ExtendBaseWidget(a)
					f, o = &a.FilaDrive, a
				}
				f.ConCasilla = conCasilla
				f.Ventana = e.Ventana
				if e.EnBusqueda {
					f.Ubicacion = " " // la plantilla solo necesita saber que hay línea
				}
				// El glifo va al CREAR la fila: el renderer lo monta una vez y la
				// fila se recicla. Se tiñe en cada Refresh con su estado.
				f.GlifoMas = glifoCon(glifoMas, iconos.TallaSM, tema.Color("text-muted", v))
				f.SiempreMenu = punteroGrueso()
				return o
			},
			e.rellenarFila,
		)
		e.filas.HideSeparators = true
		// El `widget.List` separa sus filas con el relleno del TEMA —7 px con el de la
		// suite, y también con HideSeparators—: 35.000 px de desvío en 5.000 filas.
		// Con esto van pegadas, que es lo que hace la web.
		e.Refresco.Arriba = func() bool { return e.filas.GetScrollOffset() <= 0.5 }
		e.filasObj = ui.RefrescarAlTirar(container.NewThemeOverride(e.filas, SinRelleno{}), &e.Refresco)
		e.filasCasilla, e.filasLineas = conCasilla, lineas
	} else {
		e.filas.Refresh()
	}
	if e.arriba {
		e.filas.ScrollToTop()
		e.arriba = false
	}

	fondo := canvas.NewRectangle(tema.Color("bg", v))
	col := &columnaLista{cab: cab, filas: e.filasObj}
	hijos := []fyne.CanvasObject{fondo, cab, e.filasObj}
	e.tarjetas = e.tarjetas[:0]
	if len(carpetas) > 0 {
		col.franja, col.altoFranja = e.franja(carpetas)
		hijos = append(hijos, col.franja)
	}
	if e.CargandoMas {
		col.pie = e.pieCargando(v)
		hijos = append(hijos, col.pie)
	}
	return container.New(col, hijos...)
}

// rellenarFila es el `UpdateItem` de la lista. Además de poner los datos, pide la
// página siguiente cuando se pinta una de las últimas filas: es el `onScroll` de la
// web, que la pide a 600 px del final.
func (e *Explorador) rellenarFila(i widget.ListItemID, o fyne.CanvasObject) {
	if i < 0 || i >= len(e.filasDeLista()) {
		return
	}
	f, ok := o.(*ui.FilaDrive)
	if a, es := o.(*filaArrastrable); es {
		f, ok, a.el = &a.FilaDrive, true, e.filasDeLista()[i]
	}
	if !ok {
		return
	}
	f.Destino = e.destino != "" && e.destino == e.filasDeLista()[i].ID
	e.ponerFila(f, e.filasDeLista()[i])
	if e.HayMas && !e.CargandoMas && i >= len(e.filasDeLista())-FilasAntesDelFinal && e.AlCargarMas != nil {
		e.AlCargarMas()
	}
}

// pieCargando es la franja de «Cargando más…» debajo de la lista: `border-t
// border-border/60 bg-bg sytue-meta py-1.5`, con la rueda de 14 y `gap-2`.
func (e *Explorador) pieCargando(v tema.Variante) fyne.CanvasObject {
	texto := ui.Texto(e.t("files.loadingMore"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	rueda := ui.NuevaRueda(LadoRuedaPie, "text-subtle", v)
	return container.New(&pieLista{texto: texto, rueda: rueda},
		canvas.NewRectangle(tema.Color("bg", v)), canvas.NewRectangle(conAlfaBorde(v)), rueda, texto)
}

// LadoRuedaPie es el `<Spinner size={14} />` del pie de carga, y AltoPieCargando lo
// que ocupa la franja: `py-1.5` a cada lado, la caja de `text-xs` y el `border-t`.
const (
	LadoRuedaPie    float32 = 14
	AltoPieCargando         = 2*1.5*0.25*tema.Rem + 14 + tema.Borde
)

type pieLista struct {
	texto *canvas.Text
	rueda fyne.CanvasObject
}

var _ fyne.Layout = (*pieLista)(nil)

func (p *pieLista) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoPieCargando) }

func (p *pieLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	// `justify-center gap-2`: rueda y texto se centran JUNTOS.
	ancho := LadoRuedaPie + tema.Espacio(2) + ui.Medir(p.texto.Text, tema.TextoXS, ui.Normal).Width
	x := (t.Width - ancho) / 2
	medio := tema.Borde + (t.Height-tema.Borde)/2
	p.rueda.Move(fyne.NewPos(x, medio-LadoRuedaPie/2))
	p.rueda.Resize(fyne.NewSize(LadoRuedaPie, LadoRuedaPie))
	ui.ColocarTexto(p.texto, tema.TextoXS, x+LadoRuedaPie+tema.Espacio(2), medio-tema.TextoXS.Linea/2)
}

func (e *Explorador) ponerFila(f *ui.FilaDrive, el Elemento) {
	f.Nombre, f.Carpeta, f.Extension = el.Nombre, el.Carpeta, Extension(el.Nombre)
	f.Tamano, f.Fecha = Tamano(e.loc, el.Tamano), Fecha(e.loc, el.Modificado)
	f.Ventana = e.Ventana
	f.Ubicacion = ""
	if e.EnBusqueda {
		f.Ubicacion = UbicacionDe(e.tr, el.Ruta)
	}
	f.Meta = e.metaFila(el)
	f.Seleccionada = e.Seleccion[el.ID]
	f.SeleccionActiva = e.marcados > 0
	id, elemento := el.ID, el
	f.AlAbrir = func() {
		if e.AlAbrir != nil {
			e.AlAbrir(elemento)
		}
	}
	f.AlMarcar = func() {
		if e.AlSeleccionar != nil {
			e.AlSeleccionar(id)
		}
	}
	f.AlMenu = nil
	if e.AlMenu != nil {
		f.AlMenu = func(d fyne.CanvasObject) { e.AlMenu(elemento, d) }
	}
	f.Refresh()
}

// punteroGrueso es el `pointer: coarse` de la hoja: en un teléfono no hay «por encima»,
// así que lo que la web destapa al pasar el ratón se enseña siempre.
func punteroGrueso() bool {
	return ui.EsTactil()
}

// columnaLista mete la cabecera de columnas y el listado en el hueco, con el `mx-3` de
// la cabecera y el `px-3 pb-6` del listado, y debajo el pie de carga si lo hay.
type columnaLista struct {
	cab   *ui.CabeceraDrive
	filas fyne.CanvasObject
	pie   fyne.CanvasObject
	// franja es la de las carpetas, encima de la cabecera, y altoFranja lo que pide a un
	// ancho. Sin carpetas en la franja, nil.
	franja     fyne.CanvasObject
	altoFranja func(ancho float32) float32
}

var _ fyne.Layout = (*columnaLista)(nil)

// PieLista es el `pb-6` de debajo del listado.
const PieLista float32 = 6 * 0.25 * tema.Rem

func (c *columnaLista) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, ui.AltoCabeceraDrive)
}

func (c *columnaLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	// La franja de carpetas, arriba: lo que pida, hasta el 45 % del alto (`max-h-[45%]`);
	// con más carpetas desplaza por dentro y la lista conserva más de la mitad.
	arriba := float32(0)
	if c.franja != nil {
		arriba = min(c.altoFranja(t.Width), TopeFranja*t.Height)
		c.franja.Move(fyne.NewPos(0, 0))
		c.franja.Resize(fyne.NewSize(t.Width, arriba))
	}
	ancho := max(t.Width-2*CanalonLista, 0)
	c.cab.Move(fyne.NewPos(CanalonLista, arriba))
	c.cab.Resize(fyne.NewSize(ancho, ui.AltoCabeceraDrive))
	abajo := t.Height
	if c.pie != nil {
		// El pie va a TODO el ancho (no lleva `mx-3`) y es hermano del listado, así
		// que el `pb-6` de este queda por encima de él.
		abajo -= AltoPieCargando
		c.pie.Move(fyne.NewPos(0, abajo))
		c.pie.Resize(fyne.NewSize(t.Width, AltoPieCargando))
	}
	c.filas.Move(fyne.NewPos(CanalonLista, arriba+ui.AltoCabeceraDrive))
	c.filas.Resize(fyne.NewSize(ancho, max(abajo-arriba-ui.AltoCabeceraDrive-PieLista, 0)))
}

// ---- La franja de carpetas ----

// La franja de teselas de carpetas sobre la lista (forma Marco): `max-h-[45%] shrink-0
// overflow-y-auto px-6 pt-4 pb-1` con la rejilla de `FileGrid` en `gap-2.5`.
const (
	TopeFranja         float32 = 0.45
	HuecoFranja        float32 = 2.5 * 0.25 * tema.Rem
	CanalonFranja      float32 = 6 * 0.25 * tema.Rem
	RellenoFranja      float32 = 4 * 0.25 * tema.Rem
	RellenoFranjaAbajo float32 = 1 * 0.25 * tema.Rem
)

// franja monta las teselas de las carpetas y dice cuánto alto piden a un ancho. Cada una
// es la tarjeta de la cuadrícula en su modo de franja, con el tono por turno, y cuenta
// para el arrastre como las de la rejilla: se suelta encima de una carpeta.
func (e *Explorador) franja(carpetas []Elemento) (fyne.CanvasObject, func(ancho float32) float32) {
	tonos := ui.TonosTarjeta()
	teselas := make([]fyne.CanvasObject, 0, len(carpetas))
	for i, el := range carpetas {
		t, o := e.nuevaTarjeta(el)
		t.Franja, t.Tono = true, tonos[i%len(tonos)]
		e.tarjetas = append(e.tarjetas, tarjetaPintada{el: el, objeto: o, t: t})
		teselas = append(teselas, o)
	}
	rej := ui.RejillaTarjetas(e.Ventana, HuecoFranja, CortesRejillaDrive)
	alto := func(float32) float32 { return AltoTeselaFranja }
	interior := container.New(&marcoFranja{}, container.New(&rejillaDrive{rej: rej, ancho: &e.anchoFranja, alto: alto}, teselas...))
	// El desplazable es el MISMO de un pintado a otro: marcar una casilla repinta, y con
	// uno nuevo la franja volvía arriba.
	if e.franjaScroll == nil {
		e.franjaScroll = container.NewVScroll(interior)
	} else {
		e.franjaScroll.Content = interior
		e.franjaScroll.Refresh()
	}
	n := len(carpetas)
	refresco := e.Refresco
	refresco.Arriba = func() bool { return e.franjaScroll.Offset.Y <= 0.5 }
	return ui.RefrescarAlTirar(e.franjaScroll, &refresco), func(ancho float32) float32 {
		cols := rej.Columnas(max(ancho-2*CanalonFranja, 0))
		filas := float32((n + cols - 1) / cols)
		return RellenoFranja + filas*AltoTeselaFranja + (filas-1)*HuecoFranja + RellenoFranjaAbajo
	}
}

// marcoFranja es el relleno de la franja: `px-6 pt-4 pb-1`.
type marcoFranja struct{}

var _ fyne.Layout = marcoFranja{}

func (marcoFranja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, RellenoFranja+objs[0].MinSize().Height+RellenoFranjaAbajo)
}

func (marcoFranja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(CanalonFranja, RellenoFranja))
	objs[0].Resize(fyne.NewSize(max(t.Width-2*CanalonFranja, 0), max(t.Height-RellenoFranja-RellenoFranjaAbajo, 0)))
}

// ---- El esqueleto de carga ----

// FilasEsqueleto son las nueve filas que la web pinta mientras carga la lista.
const FilasEsqueleto = 9

// esqueletoCuerpo es el placeholder de carga con la geometría de la vista que toca, que
// es lo que evita el salto de layout al llegar los datos.
//
// Solo se porta el de la LISTA. En rejilla la web pinta diez tarjetas y aquí se pintan
// las mismas nueve filas: es la misma promesa —«esto está llegando»— y la única
// consecuencia es un salto de layout al cambiar a tarjetas mientras carga, que dura lo
// que tarde la respuesta. ponytail: se cierra copiando la geometría de la tarjeta en
// cuanto haya una medida de la tarjeta de carga.
func (e *Explorador) esqueletoCuerpo(v tema.Variante) fyne.CanvasObject {
	hijos := make([]fyne.CanvasObject, 0, 4+4*FilasEsqueleto)
	hijos = append(hijos, canvas.NewRectangle(tema.Color("bg", v)))
	s := &esqueletoLista{}
	// La cabecera: tres rayas de `0.6rem` donde van los tres rótulos de columna.
	for _, ancho := range []float32{56, 48, 72} {
		b := ui.NuevoEsqueleto(ancho, 0.6*tema.Rem)
		s.cabecera = append(s.cabecera, b)
		hijos = append(hijos, b)
	}
	for i := range FilasEsqueleto {
		// Los anchos del nombre son los de la web: `38 + (i*11)%42` por ciento. Sin
		// esa variación el esqueleto parece una tabla de rayas iguales.
		fila := filaEsqueleto{
			tesela: ui.NuevoEsqueleto(ui.TeselaDrive, ui.TeselaDrive),
			nombre: ui.NuevoEsqueleto(0, 0.85*tema.Rem),
			pct:    float32(38+(i*11)%42) / 100,
			tamano: ui.NuevoEsqueleto(44, 0.7*tema.Rem),
			fecha:  ui.NuevoEsqueleto(76, 0.7*tema.Rem),
		}
		s.filas = append(s.filas, fila)
		hijos = append(hijos, fila.tesela, fila.nombre, fila.tamano, fila.fecha)
	}
	s.ventana = e.Ventana
	return container.New(s, hijos...)
}

type filaEsqueleto struct {
	tesela, nombre, tamano, fecha *ui.Esqueleto
	pct                           float32
}

// esqueletoLista coloca el placeholder con la geometría real: la cabecera con `px-3
// py-2.5` y las filas con `px-3 py-2` y la tesela de 36, dentro del `px-3` del
// contenedor.
type esqueletoLista struct {
	ventana  float32
	cabecera []*ui.Esqueleto
	filas    []filaEsqueleto
}

var _ fyne.Layout = (*esqueletoLista)(nil)

func (s *esqueletoLista) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (s *esqueletoLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	izq := 2 * CanalonLista // el `px-3` del contenedor más el de la fila
	der := t.Width - izq
	ancho := max(der-izq, 0)
	haySM := s.ventana == 0 || s.ventana >= ui.CorteSM
	hayMD := s.ventana == 0 || s.ventana >= ui.CorteMD
	poner := func(r *ui.Esqueleto, x, y, w, h float32, visible bool) {
		if !visible {
			r.Hide()
			return
		}
		r.Show()
		r.Move(fyne.NewPos(x, y))
		r.Resize(fyne.NewSize(w, h))
	}

	// Cabecera: `py-2.5` y su `border-b`, o sea 2×8,75 + 8,4 + 1.
	altoCab := 2*(2.5*0.25*tema.Rem) + 0.6*tema.Rem + tema.Borde
	y := (altoCab - tema.Borde - 0.6*tema.Rem) / 2
	if len(s.cabecera) == 3 {
		poner(s.cabecera[0], izq, y, 56, 0.6*tema.Rem, true)
		poner(s.cabecera[2], der-72, y, 72, 0.6*tema.Rem, hayMD)
		x := der - 72 - 2*tema.Espacio(4)
		if !hayMD {
			x = der
		}
		poner(s.cabecera[1], x-48, y, 48, 0.6*tema.Rem, haySM)
	}

	y = altoCab
	for _, f := range s.filas {
		centro := func(alto float32) float32 { return y + (ui.AltoFilaDrive-alto)/2 }
		poner(f.tesela, izq, centro(ui.TeselaDrive), ui.TeselaDrive, ui.TeselaDrive, true)
		x := izq + ui.TeselaDrive + ui.HuecoFila
		poner(f.fecha, der-76, centro(0.7*tema.Rem), 76, 0.7*tema.Rem, hayMD)
		derTam := der - 76 - 2*tema.Espacio(4)
		if !hayMD {
			derTam = der
		}
		poner(f.tamano, derTam-44, centro(0.7*tema.Rem), 44, 0.7*tema.Rem, haySM)
		poner(f.nombre, x, centro(0.85*tema.Rem), max(ancho*f.pct, 0), 0.85*tema.Rem, true)
		y += ui.AltoFilaDrive
	}
}

// ---- La rejilla ----

// rejilla es la vista de tarjetas: `grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4
// xl:grid-cols-5` dentro de un `px-6 py-4`.
func (e *Explorador) rejilla(v tema.Variante) fyne.CanvasObject {
	tarjetas := make([]fyne.CanvasObject, 0, len(e.Elementos))
	e.tarjetas = e.tarjetas[:0]
	tonos := ui.TonosTarjeta()
	for i, el := range e.Elementos {
		t, o := e.nuevaTarjeta(el)
		// Una carpeta es una tarjeta de recurso, con el tono por turno
		// (`CARD_TONES[index % CARD_TONES.length]`): solo pinta en la forma Marco.
		if el.Carpeta {
			t.Tono = tonos[i%len(tonos)]
		}
		e.tarjetas = append(e.tarjetas, tarjetaPintada{el: el, objeto: o, t: t})
		tarjetas = append(tarjetas, o)
	}
	rej := &rejillaDrive{rej: ui.RejillaTarjetas(e.Ventana, HuecoRejilla, CortesRejillaDrive), ancho: &e.anchoRejilla}
	var interior fyne.CanvasObject = container.New(rej, tarjetas...)
	if e.HayMas {
		// La rejilla no pagina sola: `flex justify-center pt-5` con «Cargar más».
		mas := ui.NuevoBoton(e.t("files.loadMore"), e.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
		mas.Cargando = e.CargandoMas
		interior = container.New(&conPie{separa: SeparaCargarMas}, interior, mas)
	}
	// El desplazable es el MISMO de un pintado a otro: con uno nuevo, cada repintado
	// devolvía la rejilla arriba.
	if e.desplazable == nil {
		e.desplazable = container.NewVScroll(interior)
	} else {
		e.desplazable.Content = interior
		e.desplazable.Refresh()
	}
	if e.arriba {
		e.desplazable.ScrollToTop()
		e.arriba = false
	}
	fondo := canvas.NewRectangle(tema.Color("bg", v))
	refresco := e.Refresco
	refresco.Arriba = func() bool { return e.desplazable.Offset.Y <= 0.5 }
	cuerpo := ui.RefrescarAlTirar(e.desplazable, &refresco)
	return container.New(&marcoRejilla{hijo: cuerpo}, fondo, cuerpo)
}

// nuevaTarjeta crea la tarjeta de un elemento con sus datos puestos, y lo que va al
// lienzo. Con ratón la tarjeta se arrastra a una carpeta; con el dedo, no: una tarjeta que
// se queda el arrastre no deja desplazar la rejilla. Igual que la fila.
func (e *Explorador) nuevaTarjeta(el Elemento) (*TarjetaDrive, fyne.CanvasObject) {
	var t *TarjetaDrive
	var o fyne.CanvasObject
	if punteroGrueso() || e.AlMoverA == nil {
		t = &TarjetaDrive{}
		t.ExtendBaseWidget(t)
		o = t
	} else {
		a := &tarjetaArrastrable{e: e, el: el}
		a.yo = a
		a.ExtendBaseWidget(a)
		t, o = &a.TarjetaDrive, a
	}
	e.ponerTarjeta(t, el)
	return t, o
}

// rejillaDrive es `ui.Rejilla` con el alto de las filas sacado de la MEDIDA de la
// tarjeta al ancho de columna de verdad.
//
// `fyne.Layout.MinSize` no recibe el ancho, y sin él la rejilla contaba las filas con
// la tarjeta a su ancho de reserva (126), no a los ~180 que mide en una ventana de
// 1280: el desplazable creía el contenido un tercio más corto de lo que es y las
// últimas filas no se alcanzaban. El ancho se toma del último `Layout`, que el
// Explorador guarda de un pintado a otro.
type rejillaDrive struct {
	rej   ui.Rejilla
	ancho *float32
	// alto es el de una tarjeta a un ancho de columna; nil, el de la tarjeta de la
	// cuadrícula. La franja de carpetas pone el suyo, que no depende del ancho.
	alto func(col float32) float32
}

func (r *rejillaDrive) altoTarjeta(col float32) float32 {
	if r.alto != nil {
		return r.alto(col)
	}
	return MedidaTarjetaDrive(col).Height
}

var _ fyne.Layout = (*rejillaDrive)(nil)

func (r *rejillaDrive) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := *r.ancho
	if ancho <= 0 || len(objs) == 0 {
		return r.rej.MinSize(objs)
	}
	cols := r.rej.Columnas(ancho)
	filas := (len(objs) + cols - 1) / cols
	alto := r.altoTarjeta(r.rej.AnchoColumna(ancho))
	return fyne.NewSize(ancho, float32(filas)*alto+float32(filas-1)*r.rej.Hueco)
}

// Layout coloca las tarjetas con el alto de su MEDIDA y no con su `MinSize`, que antes
// de su primer `Layout` todavía cree medir la columna de reserva.
func (r *rejillaDrive) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	*r.ancho = t.Width
	cols := r.rej.Columnas(t.Width)
	col := r.rej.AnchoColumna(t.Width)
	alto := r.altoTarjeta(col)
	for i, o := range objs {
		o.Move(fyne.NewPos(r.rej.X(t.Width, i%cols), float32(i/cols)*(alto+r.rej.Hueco)))
		o.Resize(fyne.NewSize(col, alto))
	}
}

// SeparaCargarMas es el `pt-5` sobre el botón «Cargar más» de la rejilla.
const SeparaCargarMas float32 = 5 * 0.25 * tema.Rem

// conPie apila un contenido y, debajo y centrado, un control a su medida.
type conPie struct{ separa float32 }

var _ fyne.Layout = (*conPie)(nil)

func (c *conPie) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(max(a.Width, b.Width), a.Height+c.separa+b.Height)
}

func (c *conPie) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, a.Height))
	objs[1].Move(fyne.NewPos((t.Width-b.Width)/2, a.Height+c.separa))
	objs[1].Resize(b)
}

// ponerTarjeta rellena la tarjeta de un elemento. La construye quien llama, que es quien
// sabe si además se arrastra.
func (e *Explorador) ponerTarjeta(t *TarjetaDrive, el Elemento) {
	t.Nombre, t.Extension, t.Carpeta = el.Nombre, Extension(el.Nombre), el.Carpeta
	t.Meta = e.metaTarjeta(el)
	t.Seleccionada = e.Seleccion[el.ID]
	t.Destino = e.destino != "" && e.destino == el.ID
	t.SeleccionActiva, t.Grueso = e.marcados > 0, punteroGrueso()
	clave := "files.open"
	if el.Carpeta {
		clave = "files.openFolder"
	}
	t.Rotulo = e.t(clave, i18n.Vars{"name": el.Nombre})
	elemento := el
	t.AlAbrir = func() {
		if e.AlAbrir != nil {
			e.AlAbrir(elemento)
		}
	}
	if !e.SinRed() && el.ID != "" && e.AlSeleccionar != nil {
		t.AlMarcar = func() { e.AlSeleccionar(elemento.ID) }
	}
	if e.AlMenu != nil {
		t.AlMenu = func(d fyne.CanvasObject) { e.AlMenu(elemento, d) }
	}
	if ConMiniatura(el) {
		if img, ya := e.Miniaturas[el.ID]; ya {
			t.Miniatura = img
		} else if e.AlMiniatura != nil {
			e.AlMiniatura(el)
		}
	}
}

// ConMiniatura es el `thumbable` de la web: el servicio solo hace miniaturas de JPEG,
// PNG y GIF.
func ConMiniatura(el Elemento) bool {
	if el.Carpeta || el.ID == "" {
		return false
	}
	switch Extension(el.Nombre) {
	case "jpg", "jpeg", "png", "gif":
		return true
	}
	return false
}

// metaTarjeta es el pie de la tarjeta: «Carpeta», o el tamaño y la fecha separados por
// un punto medio, que es como lo compone la web.
func (e *Explorador) metaTarjeta(el Elemento) string {
	meta := e.t("files.folder")
	if !el.Carpeta {
		meta = Tamano(e.loc, el.Tamano)
		if f := Fecha(e.loc, el.Modificado); f != "" {
			meta += " · " + f
		}
	}
	// En una búsqueda, la carpeta donde vive: los resultados vienen de todo el árbol y
	// el nombre solo no basta para reconocerlo.
	if e.EnBusqueda {
		meta += " · " + UbicacionDe(e.tr, el.Ruta)
	}
	return meta
}

// metaFila es la línea de debajo del nombre en una ventana estrecha (`sm:hidden`):
// «Carpeta» o el tamaño, y la fecha —también en una carpeta, a diferencia de la tarjeta—.
func (e *Explorador) metaFila(el Elemento) string {
	meta := Tamano(e.loc, el.Tamano)
	if el.Carpeta {
		meta = e.t("files.folder")
	}
	if f := Fecha(e.loc, el.Modificado); f != "" {
		meta += " · " + f
	}
	return meta
}

// marcoRejilla aplica el `px-6 py-4` de la zona de tarjetas.
type marcoRejilla struct{ hijo fyne.CanvasObject }

var _ fyne.Layout = (*marcoRejilla)(nil)

func (m *marcoRejilla) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (m *marcoRejilla) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	m.hijo.Move(fyne.NewPos(CanalonRejilla, RellenoRejilla))
	m.hijo.Resize(fyne.NewSize(max(t.Width-2*CanalonRejilla, 0), max(t.Height-2*RellenoRejilla, 0)))
}

// filaArrastrable es la fila del explorador que además se arrastra: `draggable` con
// `onDragStart`/`onDrop` de FileRow. Es la MISMA fila —se embebe por valor y se extiende,
// como manda Fyne—; lo único que añade es el arrastre.
//
// ponytail: sin la imagen fantasma que pinta el navegador bajo el puntero: se ve la
// carpeta de destino resaltada, que es lo que dice dónde va a caer.
type filaArrastrable struct {
	ui.FilaDrive
	e  *Explorador
	el Elemento
}

var _ fyne.Draggable = (*filaArrastrable)(nil)

// Dragged empieza o sigue el arrastre.
func (a *filaArrastrable) Dragged(ev *fyne.DragEvent) { a.e.arrastrar(a.el, ev.AbsolutePosition) }

// DragEnd suelta.
func (a *filaArrastrable) DragEnd() { a.e.soltar() }

// tarjetaArrastrable es lo mismo para una tarjeta de la rejilla: el `draggable` del
// `<div role="button">` de la vista de cuadrícula, que en la web se arrastra igual que la
// fila.
type tarjetaArrastrable struct {
	TarjetaDrive
	e  *Explorador
	el Elemento
}

var _ fyne.Draggable = (*tarjetaArrastrable)(nil)

// Dragged empieza o sigue el arrastre.
func (a *tarjetaArrastrable) Dragged(ev *fyne.DragEvent) { a.e.arrastrar(a.el, ev.AbsolutePosition) }

// DragEnd suelta.
func (a *tarjetaArrastrable) DragEnd() { a.e.soltar() }

// tarjetaPintada es una tarjeta de la rejilla con su elemento.
type tarjetaPintada struct {
	el     Elemento
	objeto fyne.CanvasObject
	t      *TarjetaDrive
}

// arrastrar empieza o sigue un arrastre desde `el` con el puntero en `abs`: si `el` forma
// parte de la selección viaja la selección ENTERA —arrastrar uno de doce marcados y
// mover solo ese es lo contrario de lo que acaba de pedir quien los marcó—, y si no, solo
// él. El destino es la carpeta de debajo, si acepta algo de lo que viaja.
func (e *Explorador) arrastrar(el Elemento, abs fyne.Position) {
	if e.arrastrando == nil {
		e.arrastrando = []Elemento{el}
		if e.Seleccion[el.ID] {
			e.arrastrando = e.seleccionados()
		}
	}
	destino := ""
	if d, ok := e.elementoEn(abs); ok && d.Carpeta && SePuedeSoltar(e.arrastrando, d) {
		destino = d.ID
	}
	if destino != e.destino {
		antes := e.destino
		e.destino = destino
		e.marcarDestino(antes)
	}
}

// soltar termina el arrastre: si hay carpeta de destino, mueve; si no, no pasa nada.
func (e *Explorador) soltar() {
	origenes, destino := e.arrastrando, e.destino
	e.arrastrando, e.destino = nil, ""
	e.marcarDestino(destino)
	if destino == "" || e.AlMoverA == nil {
		return
	}
	for _, el := range e.Elementos {
		if el.ID == destino {
			e.AlMoverA(origenes, el)
			return
		}
	}
}

// marcarDestino repinta el resalte: en la lista, las filas visibles; en la rejilla, solo
// la tarjeta que lo pierde y la que lo gana, sin rehacer la rejilla —rehacerla a cada
// movimiento del puntero destruiría la tarjeta que se está arrastrando—.
func (e *Explorador) marcarDestino(antes string) {
	if !e.Rejilla && e.filas != nil {
		e.filas.Refresh()
	}
	// Las tarjetas: las de la rejilla o, en la lista, las teselas de la franja.
	for _, tp := range e.tarjetas {
		if tp.el.ID == antes || tp.el.ID == e.destino {
			tp.t.Destino = e.destino != "" && tp.el.ID == e.destino
			tp.t.Refresh()
		}
	}
}

// elementoEn es el elemento que hay bajo un punto de la ventana: la fila de la lista o
// la tarjeta de la rejilla.
func (e *Explorador) elementoEn(abs fyne.Position) (Elemento, bool) {
	// Las tarjetas primero: las de la rejilla o, en la lista, las teselas de la franja.
	d := fyne.CurrentApp().Driver()
	for _, tp := range e.tarjetas {
		o, t := d.AbsolutePositionForObject(tp.objeto), tp.objeto.Size()
		if abs.X >= o.X && abs.X < o.X+t.Width && abs.Y >= o.Y && abs.Y < o.Y+t.Height {
			return tp.el, true
		}
	}
	if e.Rejilla {
		return Elemento{}, false
	}
	if e.filas == nil {
		return Elemento{}, false
	}
	origen := fyne.CurrentApp().Driver().AbsolutePositionForObject(e.filas)
	y := abs.Y - origen.Y + e.filas.GetScrollOffset()
	if abs.X < origen.X || abs.X > origen.X+e.filas.Size().Width || y < 0 {
		return Elemento{}, false
	}
	i := int(y / ui.AltoFilaDriveCon(e.filasLineas))
	if i >= len(e.filasDeLista()) {
		return Elemento{}, false
	}
	return e.filasDeLista()[i], true
}

// seleccionados son los marcados, en el orden de la lista.
func (e *Explorador) seleccionados() []Elemento {
	var out []Elemento
	for _, el := range e.Elementos {
		if e.Seleccion[el.ID] {
			out = append(out, el)
		}
	}
	return out
}

// SePuedeSoltar es el `sePuedeSoltar` de `arrastrar.ts` para un destino y lo que viaja:
// se acepta si ALGO de lo que viaja cambiaría de sitio. No cambia de sitio lo que ya está
// en esa carpeta, ni una carpeta que se suelta en sí misma o en una de sus hijas.
func SePuedeSoltar(origenes []Elemento, destino Elemento) bool {
	return len(Movibles(origenes, destino)) > 0
}

// Movibles es lo que de verdad se movería (`movibles` de la web).
func Movibles(origenes []Elemento, destino Elemento) []Elemento {
	var out []Elemento
	for _, o := range origenes {
		if o.ID == destino.ID {
			continue
		}
		if o.Ruta != "" && destino.Ruta != "" {
			if strings.HasPrefix(destino.Ruta, o.Ruta+"/") || carpetaDeRuta(o.Ruta) == destino.Ruta {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

// carpetaDeRuta es la carpeta que contiene una ruta (`carpetaDe` de la web).
func carpetaDeRuta(ruta string) string {
	if i := strings.LastIndexByte(ruta, '/'); i > 0 {
		return ruta[:i]
	}
	return ""
}
