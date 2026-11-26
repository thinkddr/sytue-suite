// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image"
	"image/color"
	"strconv"
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
	// Orden es en el que llegan los Elementos (lo aplica el servicio), y AlOrdenar lo
	// cambia por una columna: la misma invierte el sentido. Nil: no se ordena.
	Orden     Orden
	AlOrdenar func(CampoOrden)
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
	// Arrastrando es que hay ficheros del sistema arrastrándose por encima de la ventana
	// (el `dragging` de la web): el cuerpo se tapa con «Suelta para subir aquí». Sin red
	// no se enseña, porque soltar no subiría nada.
	Arrastrando bool
	// AVista es el id que llevar a la vista en el próximo pintado («Mostrar en su
	// carpeta»); se consume al pintarlo.
	AVista string

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
	// AlCombinarPDF une en un PDF nuevo los PDF marcados; nil, la acción no está (no
	// hay cliente de PDF). Combinando es que hay una combinación en curso: el botón
	// gira hasta que acaba.
	AlCombinarPDF func()
	Combinando    bool
	// AlCargarMas pide la página siguiente. Lo llama la lista al pintar una de sus
	// últimas filas y el botón «Cargar más» de la rejilla.
	AlCargarMas func()
	Refresco    ui.RefrescoLista
	// AlMoverA mueve lo arrastrado a una carpeta soltándolo encima; nil, no se arrastra.
	AlMoverA func(origenes []Elemento, destino Elemento)
	// AlMenu abre el menú de acciones de un elemento —el «⋯» de la fila o de la
	// tarjeta— colgado de `disparador`. El de «Nuevo» no es suyo: cuelga del «+» flotante
	// de la pantalla.
	AlMenu func(el Elemento, disparador fyne.CanvasObject)
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
	filas        *listaFilas
	filasObj     fyne.CanvasObject
	filasCasilla bool
	// filasLineas son las líneas bajo el nombre con las que se creó la lista: una fila
	// con dos crece, y `widget.List` toma el alto de la plantilla.
	filasLineas  int
	desplazable  *container.Scroll
	anchoRejilla float32
	// rejillaViva es la cuadrícula del último pintado: la que hace las tarjetas que
	// entran al desplazar (ver rejillaVirtual).
	rejillaViva *rejillaVirtual
	// deLista es lo que enseña la LISTA en este pintado: todo, o lo que no se ha ido a
	// la franja de carpetas (ver Partes). franjaScroll es el desplazable de la franja,
	// el mismo de un pintado a otro, y anchoFranja el último ancho de su rejilla.
	deLista      []Elemento
	franjaScroll *container.Scroll
	anchoFranja  float32
	// listaFranja desplaza juntas las carpetas y la lista en una ventana baja.
	// La lista conserva su propia virtualización y su instancia al repintar.
	listaFranja *container.Scroll
	// reservaFilas es la fila vacía del final de `filas` para el botón flotante.
	reservaFilas *ui.ReservaEnLista
	// vista es la única raíz que puede colocar las piezas conservadas. Al
	// repintar se retira antes de prestarlas a la vista nueva.
	vista *fyne.Container
	// rueda es la de la franja de progreso: la MISMA de un repintado a otro, que se
	// repinta cuatro veces por segundo y una rueda nueva cada vez no llegaría a girar.
	rueda *ui.Rueda
	// marcados es Marcados() de este pintado, para no recontar en cada fila.
	marcados int
	// arrastrando es lo que viaja en un arrastre y destino la carpeta sobre la que se
	// soltaría (su id; vacío, ninguna que lo acepte).
	arrastrando  []Elemento
	autoArrastre ui.AutoScrollArrastre
	destino      string
	// tarjetas son las de la rejilla de este pintado, en su orden, con lo que está en el
	// lienzo (`objeto`: la tarjeta o la que la embebe para arrastrarse) para saber dónde
	// cae el puntero, y la tarjeta en sí para marcar el destino sin repintar la rejilla.
	tarjetas []tarjetaPintada
	// arriba pide volver al principio en el próximo pintado: una carpeta NUEVA
	// empieza arriba, y la misma recargada o ampliada se queda donde estaba.
	arriba bool
	// franjas son las piezas de la vista montada que no dependen de la selección, y
	// cabLista la cabecera de columnas de la lista: ActualizarSeleccion las conserva.
	franjas  struct{ cab, subida, cuerpo fyne.CanvasObject }
	cabLista *ui.CabeceraDrive
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
	return &Explorador{tr: tr, loc: tr.Locale(), Seleccion: map[string]bool{}, Orden: OrdenDelServicio}
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
// Es el contrato de `docs/architecture.md` §«Contrato offline»: se navega por lo ya
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

// PDFMarcados son los PDF marcados, en el orden en que se VEN: es el orden en que se
// combinan, como en la web (`selectedPdfs`), porque es el que la persona tiene delante.
func (e *Explorador) PDFMarcados() []Elemento {
	var out []Elemento
	for _, el := range e.EnOrden() {
		if e.Seleccion[el.ID] && !el.Carpeta && Extension(el.Nombre) == "pdf" {
			out = append(out, el)
		}
	}
	return out
}

// PuedeCombinar es el `puedeCombinar` de la web: dos o más PDF marcados y NADA más.
// Con una carpeta o un .docx en la selección la acción no tendría sentido, y ofrecerla
// para fallar después es peor que ocultarla. Sin red tampoco: el resultado se sube.
func (e *Explorador) PuedeCombinar() bool {
	if e.AlCombinarPDF == nil || e.SinRed() {
		return false
	}
	n := len(e.PDFMarcados())
	return n >= 2 && n == e.Marcados()
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
	if e.vista != nil {
		// Un árbol retirado no puede recolocar el scroll, la lista o el buscador
		// que ahora usa la vista nueva. Hay que retirar layout y descendientes.
		e.vista.Layout = nil
		e.vista.Objects = nil
		e.vista = nil
	}
	e.marcados = e.Marcados()
	e.franjas.cab, e.franjas.subida, e.cabLista = e.cabecera(v), nil, nil
	if e.Subida != nil {
		e.franjas.subida = e.barraSubida(v)
	}
	e.franjas.cuerpo = e.conVeloSoltar(e.cuerpo(v), v)
	// Pila mide AltoCon con el ancho actual antes de colocar la cabecera.
	// No se encola su Refresh: un repintado retira esta pila pero conserva
	// listaFranja, y una notificación antigua redimensionaría el scroll vivo.
	pila, franjas := e.apilarFranjas(v)
	e.vista = container.New(pila, franjas...)
	return e.vista
}

// apilarFranjas son la cabecera, la franja de lote si hay algo marcado, la de la subida
// y el cuerpo, en ese orden, en la pila que las coloca: el cuerpo es el que crece.
func (e *Explorador) apilarFranjas(v tema.Variante) (Pila, []fyne.CanvasObject) {
	franjas := []fyne.CanvasObject{e.franjas.cab}
	if n := e.marcados; n > 0 && !e.SinRed() {
		franjas = append(franjas, e.barraSeleccion(v, n))
	}
	if e.franjas.subida != nil {
		franjas = append(franjas, e.franjas.subida)
	}
	return Pila{Elastico: len(franjas)}, append(franjas, e.franjas.cuerpo)
}

// ActualizarSeleccion pone la selección de ahora en lo que ya está montado: la casilla
// de cada tarjeta y fila, la de «seleccionar todo» y la franja de lote, que es lo único
// que se rehace. Marcar un fichero rehacía la pantalla entera (~1 ms con la rejilla
// virtual, y el árbol viejo retenido un minuto). Devuelve false si no hay vista
// montada en la que ponerla: entonces toca repintar.
func (e *Explorador) ActualizarSeleccion(v tema.Variante) bool {
	if e.vista == nil || e.Fase != Exito || e.franjas.cab == nil || e.franjas.cuerpo == nil {
		return false
	}
	e.marcados = e.Marcados()
	// Las tarjetas montadas: las de la rejilla (virtual: las que no están se hacen ya
	// con la selección de ahora) o las de la franja de carpetas de la lista.
	for _, tp := range e.tarjetas {
		tp.t.Seleccionada, tp.t.SeleccionActiva = e.Seleccion[tp.el.ID], e.marcados > 0
		tp.t.Refresh()
	}
	if !e.Rejilla && e.filas != nil {
		if e.cabLista != nil {
			e.cabLista.Todas, e.cabLista.Algunas = e.TodoMarcado(), e.AlgoMarcado()
			e.cabLista.Refresh()
		}
		e.filas.Refresh() // cada fila vuelve a leer la selección al rellenarse
	}
	viejas := e.vista.Objects
	e.vista.Layout, e.vista.Objects = e.apilarFranjas(v)
	e.vista.Refresh()
	soltarArboles(viejas, e.vista) // la franja de lote de antes
	return true
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

	// «Nuevo» y «Subir» ya no van aquí: crear es el «+» flotante de la pantalla, en todos
	// los anchos, que abre el menú de «Nuevo» con «Subir» delante (`capaNuevo`). Lo que se
	// sube lo sigue diciendo la franja de progreso (`barraSubida`).
	conmutador := NuevoConmutadorVista(e.t("files.viewLabel"), e.t("files.viewList"), e.t("files.viewGrid"), e.Rejilla, e.AlCambiarVista)

	c := &CabeceraVista{
		Ruta:       migas,
		Acciones:   []fyne.CanvasObject{buscador},
		Conmutador: conmutador,
		Ventana:    e.Ventana,
	}
	if e.conSelectorOrden() {
		var orden *ui.Boton
		orden = ui.NuevoBoton(e.t("files.sort.label"), func() { e.menuOrden(orden) }).Con(ui.Secundario, ui.TallaSM)
		orden.IconoDetras, orden.TamIcono, orden.Hueco = iconos.ChevronAbajo, LadoGalonNuevo, HuecoNuevo
		c.Orden = orden
	}
	return c.Objeto(v)
}

// ordenable dice si lo que se ve se puede ordenar: en una búsqueda manda la relevancia.
func (e *Explorador) ordenable() bool { return e.AlOrdenar != nil && !e.EnBusqueda }

// conSelectorOrden dice si la cabecera lleva «Ordenar»: donde no hay cabeceras de
// columna que pulsar, que es la cuadrícula, y la lista por debajo de 768, donde «Tamaño»
// y «Modificado» no se pintan y solo quedaría ordenar por nombre.
func (e *Explorador) conSelectorOrden() bool {
	return e.ordenable() && (e.Rejilla || (e.Ventana > 0 && e.Ventana < ui.Estrecho))
}

// menuOrden despliega las tres columnas bajo «Ordenar», como `SelectorOrden`: la vigente
// con la flecha de su sentido, y las otras con el hueco de la
// flecha para que los rótulos queden alineados. Elegir la vigente invierte el sentido.
func (e *Explorador) menuOrden(disparador fyne.CanvasObject) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	elementos := make([]fyne.CanvasObject, 0, len(CamposOrden))
	for _, campo := range CamposOrden {
		rotulo := e.t("files.col." + string(campo))
		el := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() {
			if e.AlOrdenar != nil {
				e.AlOrdenar(campo)
			}
		})
		if campo == e.Orden.Campo {
			flecha, sentido := iconos.ChevronArriba, "files.sort.asc"
			if e.Orden.Desc {
				flecha, sentido = iconos.ChevronAbajo, "files.sort.desc"
			}
			// `text-text-muted`, el hueco del icono de MenuItem. Sin `Actual`: en la web
			// el `aria-current` solo dice dónde cae el foco al abrir, no pinta fondo.
			el.Glifo = iconos.Trazo(flecha, iconos.TallaSM, "text-muted", v)
			// La flecha es solo visual: el sentido va también en el nombre accesible.
			el.Rotulo = rotulo + " · " + e.t(sentido)
		} else {
			el.Glifo = iconos.TrazoConTinta(iconos.ChevronArriba, iconos.TallaSM, color.NRGBA{})
		}
		elementos = append(elementos, el)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = e.t("files.sort.label")
	m.Mostrar(disparador)
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

// borrarBusqueda vacía el buscador y vuelve a la carpeta. El campo avisa a `AlBuscar`
// al cambiar; si no lo hiciera (un campo aún sin pintar), se le avisa aquí.
func (e *Explorador) borrarBusqueda() {
	e.campoBusqueda().PonerTexto("")
	if e.Consulta != "" && e.AlBuscar != nil {
		e.AlBuscar("")
	}
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

// barraSeleccion es la franja de acciones en lote: `ui.BarraDeLote` con las
// acciones de Drive y su canalón (`px-4 sm:px-6`).
//
// Lleva «Combinar PDF» (solo si se puede, ver PuedeCombinar), «Mover», «Descargar» y
// «Papelera», en el orden de la web.
//
// ponytail: sin el `title` de la web («Se combinan en el orden en que aparecen en la
// lista»): `ui.BarraDeLote` recibe botones sueltos y no los envuelve en una `ui.Pista`.
func (e *Explorador) barraSeleccion(v tema.Variante, n int) fyne.CanvasObject {
	acciones := e.accionesLote(v, false)
	if ui.EsTactil() && e.Ventana > 0 && !accionesLoteCaben(acciones, n, e.Ventana) {
		// En el teléfono las acciones van en una tira deslizable entre el número y el
		// aspa, y con su rótulo no caben: «Papelera», la última, se quedaba en «Papeler»
		// contra el borde. Entonces van solo con su glifo, como la barra contextual de un
		// teléfono; el rótulo sigue siendo su nombre accesible. Son botones nuevos: el
		// botón decide al montarse si pinta el rótulo, y medir ya los ha montado.
		acciones = e.accionesLote(v, true)
	}
	recuento := e.t("files.selection.count", i18n.Vars{"count": n})
	return ui.NuevaBarraDeLote(recuento, e.t("ui.bulk.region"), e.t("files.selection.clear"),
		acciones, e.limpiarSeleccion).Con(CanalonCabecera(e.Ventana)).ConNumero(n)
}

// accionesLote son los botones de la barra de lote, con su rótulo o solo con su glifo.
func (e *Explorador) accionesLote(v tema.Variante, soloIcono bool) []*ui.Boton {
	var acciones []*ui.Boton
	if e.PuedeCombinar() {
		b := ui.NuevoBoton(e.t("pdf.combine"), e.AlCombinarPDF).Con(ui.Secundario, ui.TallaSM)
		b.Inerte = e.Ocupado && !e.Combinando
		b.Cargando = e.Combinando
		acciones = append(acciones, conGlifo(b, glifoCopia, v))
	}
	acciones = append(acciones,
		conGlifo(ui.NuevoBoton(e.t("files.selection.move"), e.AlMoverLote).Con(ui.Secundario, ui.TallaSM), glifoMover, v),
		conGlifo(ui.NuevoBoton(e.t("files.selection.download"), e.AlDescargarLote).Con(ui.Secundario, ui.TallaSM), glifoDescargar, v),
		conGlifo(ui.NuevoBoton(e.t("files.selection.trash"), e.AlPapeleraLote).Con(ui.Peligro, ui.TallaSM), glifoPapelera, v),
	)
	if soloIcono {
		for _, b := range acciones {
			b.Rotulo, b.SoloIcono = b.Texto, true
		}
	}
	return acciones
}

// accionesLoteCaben dice si las acciones con su rótulo caben enteras en la barra de lote
// táctil de una ventana de ese ancho: el canalón a cada lado, el número (`text-base
// font-semibold` con su `mr-1`), el aspa de 44 y los `gap-1.5` entre todo.
func accionesLoteCaben(acciones []*ui.Boton, n int, ventana float32) bool {
	hueco := tema.Espacio(1.5)
	libre := ventana - 2*CanalonCabecera(ventana) -
		ui.Medir(strconv.Itoa(n), tema.TextoBase, ui.SemiNegrita).Width - tema.Espacio(1) -
		ui.SueloTactil(ui.MedidaBoton("", ui.TallaSM, 0).Height) - 2*hueco
	for i, b := range acciones {
		if i > 0 {
			libre -= hueco
		}
		libre -= b.MinSize().Width
	}
	return libre >= 0
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

// MargenVeloSoltar es el `inset-2` del velo de soltar, RadioVeloSoltar su `rounded-xl`,
// GrosorVeloSoltar su `border-2` y LadoGlifoSoltar la caja `h-12 w-12` del glifo.
const (
	GrosorVeloSoltar float32 = 2
	LadoGlifoSoltar  float32 = 12 * 0.25 * tema.Rem
)

var (
	MargenVeloSoltar = tema.Espacio(2)
	RadioVeloSoltar  = tema.RadioXL
)

// conVeloSoltar tapa el cuerpo, mientras se arrastran ficheros del sistema por encima,
// con el velo de FileBrowser.tsx: `bg-bg/90 absolute inset-2 rounded-xl border-2
// border-dashed border-primary`, y en el centro el glifo `upload` de 24 en
// `text-primary-text` sobre «Suelta para subir aquí» en `text-base font-semibold`. En la
// web es `aria-hidden` y `pointer-events-none`: solo avisa, la subida la hace soltar.
func (e *Explorador) conVeloSoltar(cuerpo fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	if !e.Arrastrando || e.SinRed() {
		return cuerpo
	}
	fondo := tema.Color("bg", v)
	fondo.A = uint8(float32(fondo.A) * 0.9)
	placa := canvas.NewRectangle(fondo)
	placa.CornerRadius = RadioVeloSoltar
	glifo := glifoCon(glifoSubir, 24, tema.Color("primary-text", v)).Objeto()
	caja := container.NewGridWrap(fyne.NewSize(LadoGlifoSoltar, LadoGlifoSoltar), container.NewCenter(glifo))
	texto := ui.NuevoRotulo(e.t("files.dropHere"), tema.TextoBase, ui.SemiNegrita, "text")
	centro := container.NewCenter(container.New(ui.Columna{Hueco: tema.Espacio(2), Encaja: true}, caja, texto))
	m := MargenVeloSoltar
	velo := container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, container.NewStack(
		placa, ui.NuevoBordeDiscontinuo(GrosorVeloSoltar, RadioVeloSoltar, "primary"), centro))
	return container.NewStack(cuerpo, velo)
}

// vacio es el estado sin elementos. La búsqueda tiene su propio texto: decirle «esta
// carpeta está vacía» a quien acaba de buscar en TODO el árbol es mentirle.
func (e *Explorador) vacio(v tema.Variante) fyne.CanvasObject {
	if e.EnBusqueda {
		x := ui.NuevoVacio(
			e.t("files.search.emptyTitle", i18n.Vars{"query": strings.TrimSpace(e.Consulta)}),
			e.t("files.search.emptyBody"))
		x.Ilustracion = ilustracion(iconos.VacioBusqueda, v)
		// «Borrar la búsqueda»: volver a la carpeta sin ir a buscar la «×» del campo,
		// como el botón del estado vacío de la web.
		if e.AlBuscar != nil {
			x.Accion = ui.NuevoBoton(e.t("files.search.clear"), e.borrarBusqueda).Con(ui.Secundario, ui.TallaSM)
		}
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
	fondo := canvas.NewRectangle(tema.Color(ui.FondoHoja(), v))
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

// llevarAVista desplaza la lista hasta `AVista` («Mostrar en su carpeta») y lo consume.
// Una carpeta de la franja ya está arriba, a la vista; un fichero se busca en la lista.
func (e *Explorador) llevarAVista() {
	if e.AVista == "" {
		return
	}
	for i, el := range e.deLista {
		if el.ID == e.AVista {
			e.filas.ScrollTo(i)
			break
		}
	}
	e.AVista = ""
}

// lista es la cabecera de columnas más el listado virtualizado.
func (e *Explorador) lista(v tema.Variante) fyne.CanvasObject {
	cab := e.cabeceraLista()
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
	lineas := e.lineasDeFila()
	e.prepararFilas(conCasilla, lineas, v)
	conFranja := len(carpetas) > 0 && ui.EsTactil()
	e.ajustarDesplazamiento(conFranja)
	e.llevarAVista()

	fondo := canvas.NewRectangle(tema.Color(ui.FondoHoja(), v))
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
	return e.montarLista(col, hijos, lineas)
}

// cabeceraLista monta la cabecera de columnas (con su casilla de «todas» y el orden,
// si la vista se deja ordenar) y la recuerda en `cabLista`.
func (e *Explorador) cabeceraLista() *ui.CabeceraDrive {
	cab := ui.NuevaCabeceraDrive(e.t("files.col.name"), e.t("files.col.size"), e.t("files.col.modified"))
	e.cabLista = cab
	cab.ConCasilla = !e.SinRed()
	cab.RotuloCasilla = e.t("files.selectAll")
	cab.Todas, cab.Algunas = e.TodoMarcado(), e.AlgoMarcado()
	cab.Ventana = e.Ventana
	cab.AlMarcarTodas = e.AlSeleccionarTodo
	if e.ordenable() {
		cab.OrdenCampo, cab.OrdenDesc = string(e.Orden.Campo), e.Orden.Desc
		cab.AlOrdenar = func(campo string) { e.AlOrdenar(CampoOrden(campo)) }
	}
	return cab
}

// lineasDeFila cuenta las líneas de `sytue-meta` que cada fila lleva bajo el nombre:
// la ubicación en una búsqueda y, cuando la ventana estrecha esconde sus columnas,
// la del tamaño y la fecha (como `ui.FilaDrive.Lineas`).
func (e *Explorador) lineasDeFila() int {
	lineas := 0
	if e.EnBusqueda {
		lineas++
	}
	if e.Ventana > 0 && e.Ventana < ui.CorteSM {
		lineas++
	}
	return lineas
}

// prepararFilas crea el listado virtualizado cuando cambia la forma de sus filas
// (casilla o líneas) y, si no, lo refresca tal cual.
func (e *Explorador) prepararFilas(conCasilla bool, lineas int, v tema.Variante) {
	if e.filas != nil && e.filasCasilla == conCasilla && e.filasLineas == lineas {
		e.filas.Refresh()
		return
	}
	e.filas = nuevaListaFilas(
		e.abrirFila,
		func() int { return len(e.filasDeLista()) },
		e.plantillaFila(conCasilla, v),
		e.rellenarFila,
	)
	e.filas.HideSeparators = true
	// Una fila vacía al final: el «+» flotante tapaba la última.
	e.reservaFilas = ui.ReservarEnLista(&e.filas.List)
	// El `widget.List` separa sus filas con el relleno del TEMA —7 px con el de la
	// suite, y también con HideSeparators—: 35.000 px de desvío en 5.000 filas.
	// Con esto van pegadas, que es lo que hace la web.
	e.filasObj = ui.RefrescarAlTirar(container.NewThemeOverride(e.filas, SinRelleno{}), &e.Refresco)
	e.filasCasilla, e.filasLineas = conCasilla, lineas
}

// plantillaFila crea cada fila reciclable del listado.
func (e *Explorador) plantillaFila(conCasilla bool, v tema.Variante) func() fyne.CanvasObject {
	return func() fyne.CanvasObject {
		// Con ratón la fila se arrastra a una carpeta; con el dedo, no: una fila
		// que se queda el arrastre no deja desplazar la lista.
		var o fyne.CanvasObject
		var f *ui.FilaDrive
		switch {
		case punteroGrueso() && e.AlMoverA != nil:
			a := e.nuevaFilaTactil()
			f, o = a.fila, a
		case e.AlMoverA == nil:
			f = ui.NuevaFilaDrive("", "")
			o = f
		default:
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
	}
}

// ajustarDesplazamiento pone la reserva del «Subir» flotante, el «arriba del todo»
// del tirar para refrescar y, si se pidió, vuelve el listado al principio.
func (e *Explorador) ajustarDesplazamiento(conFranja bool) {
	// Con la franja de carpetas en el teléfono la lista va dentro de otro desplazamiento
	// (montarLista). Si la franja cabe, ese otro no se mueve y el final de la lista es el
	// de la pantalla: la reserva va en la lista, como sin franja. Si no cabe (una ventana
	// baja: `compacta`), la lista es de dos filas dentro del desplazamiento y la reserva
	// va al final de éste (`columnaLista.reserva`); ahí la lista no la lleva. Lo decide
	// el alto de la ventana, que se sabe al colocar: viewportListaCarpetas lo reajusta.
	e.ajustarReserva(conFranja && e.listaFranja != nil && franjaCompacta(e.listaFranja.Size().Height))
	e.Refresco.Arriba = func() bool {
		return e.filas.GetScrollOffset() <= 0.5 && (!conFranja || e.listaFranja == nil || e.listaFranja.Offset.Y <= 0.5)
	}
	if e.arriba {
		e.filas.ScrollToTop()
		if e.listaFranja != nil {
			e.listaFranja.ScrollToTop()
		}
		e.arriba = false
	}
}

// ajustarReserva pone la fila de la reserva del «+» flotante en la lista, salvo con la
// franja compacta, que la lleva al final de su desplazamiento.
func (e *Explorador) ajustarReserva(compacta bool) {
	if compacta {
		e.reservaFilas.Quitar()
		return
	}
	e.reservaFilas.Ajustar(e.Ventana)
}

func (e *Explorador) montarLista(col *columnaLista, hijos []fyne.CanvasObject, lineas int) fyne.CanvasObject {
	if col.franja == nil || !ui.EsTactil() {
		return container.New(col, hijos...)
	}
	// Conserva el desplazable y su contenido entre repintados.
	col.altoFilas = float32(min(len(e.deLista), 2)) * ui.AltoFilaDriveCon(lineas)
	// El «+» flota en todos los anchos (2026-11-25): la reserva va con cualquier ventana.
	if e.Ventana > 0 {
		col.reserva = ui.ReservaFlotante
	}
	var contenido *fyne.Container
	if e.listaFranja == nil {
		contenido = container.New(col, hijos...)
		e.listaFranja = container.NewVScroll(contenido)
	} else {
		var ok bool
		contenido, ok = e.listaFranja.Content.(*fyne.Container)
		if !ok || contenido == nil {
			contenido = container.New(col, hijos...)
			e.listaFranja.Content = contenido
		}
		contenido.Layout, contenido.Objects = col, hijos
	}
	viewport := &viewportListaCarpetas{
		col: col, contenido: contenido, scroll: e.listaFranja,
		normal: col.franja, natural: e.franjaScroll.Content,
		reserva: e.ajustarReserva,
	}
	viewport.preparar(e.listaFranja.Size())
	refresco := e.Refresco
	refresco.Arriba = func() bool { return e.listaFranja.Offset.Y <= 0.5 && e.filas.GetScrollOffset() <= 0.5 }
	return ui.RefrescarAlTirar(container.New(viewport, e.listaFranja), &refresco)
}

// rellenarFila es el `UpdateItem` de la lista. Además de poner los datos, pide la
// página siguiente cuando se pinta una de las últimas filas: es el `onScroll` de la
// web, que la pide a 600 px del final.
func (e *Explorador) rellenarFila(i widget.ListItemID, o fyne.CanvasObject) {
	if i < 0 || i >= len(e.filasDeLista()) {
		return
	}
	f, ok := o.(*ui.FilaDrive)
	if a, es := o.(*filaTactilDrive); es {
		f, ok, a.el = a.fila, true, e.filasDeLista()[i]
		a.asa.Rotulo = e.t("move.title", i18n.Vars{"name": a.el.Nombre})
	}
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
// border-border/60 bg-[var(--sytue-sheet-bg)] sytue-meta py-1.5`, con la rueda de 14 y
// `gap-2`: sobre la hoja, no sobre el lienzo.
func (e *Explorador) pieCargando(v tema.Variante) fyne.CanvasObject {
	texto := ui.Texto(e.t("files.loadingMore"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	rueda := ui.NuevaRueda(LadoRuedaPie, "text-subtle", v)
	return container.New(&pieLista{texto: texto, rueda: rueda},
		canvas.NewRectangle(tema.Color(ui.FondoHoja(), v)), canvas.NewRectangle(conAlfaBorde(v)), rueda, texto)
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
	f.RotuloCasilla = e.t("files.select", i18n.Vars{"name": el.Nombre})
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
	compacta   bool
	ancho      float32
	altoFilas  float32
	// reserva es el hueco del «+» flotante al final del desplazamiento de la franja
	// compacta (ui.ReservaFlotante): la lista de dos filas queda encima del botón al
	// bajar del todo. Sin compactar, la reserva va en la lista. altoVista
	// es el alto del desplazamiento, que acota la reserva (ver pieCompacta).
	reserva, altoVista float32
}

var _ fyne.Layout = (*columnaLista)(nil)

// PieLista es el `pb-6` de debajo del listado.
const PieLista float32 = 6 * 0.25 * tema.Rem

func (c *columnaLista) MinSize([]fyne.CanvasObject) fyne.Size {
	if c.compacta {
		alto := c.altoFranja(c.ancho) + ui.AltoCabeceraDrive + c.altoFilas + c.pieCompacta()
		if c.pie != nil {
			alto += AltoPieCargando
		}
		return fyne.NewSize(0, alto)
	}
	return fyne.NewSize(0, ui.AltoCabeceraDrive)
}

func (c *columnaLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	// La franja de carpetas, arriba: lo que pida, hasta el 45 % del alto (`max-h-[45%]`)
	// recortado a filas enteras; con más carpetas desplaza por dentro y la lista conserva
	// más de la mitad.
	arriba := float32(0)
	if c.franja != nil {
		arriba = min(c.altoFranja(t.Width), franjaEnFilas(TopeFranja*t.Height))
		if c.compacta {
			arriba = c.altoFranja(t.Width)
		}
		c.franja.Move(fyne.NewPos(0, 0))
		c.franja.Resize(fyne.NewSize(t.Width, arriba))
	}
	ancho := max(t.Width-2*CanalonLista, 0)
	c.cab.Move(fyne.NewPos(CanalonLista, arriba))
	c.cab.Resize(fyne.NewSize(ancho, ui.AltoCabeceraDrive))
	abajo, debajo := t.Height, PieLista
	if c.compacta {
		debajo = c.pieCompacta()
	}
	if c.pie != nil {
		// El pie va a TODO el ancho (no lleva `mx-3`) y es hermano del listado, así
		// que el `pb-6` de este queda por encima de él.
		abajo -= AltoPieCargando
		c.pie.Move(fyne.NewPos(0, abajo))
		c.pie.Resize(fyne.NewSize(t.Width, AltoPieCargando))
	}
	c.filas.Move(fyne.NewPos(CanalonLista, arriba+ui.AltoCabeceraDrive))
	c.filas.Resize(fyne.NewSize(ancho, max(abajo-arriba-ui.AltoCabeceraDrive-debajo, 0)))
}

// pieCompacta es lo que queda debajo de la lista de la franja compacta: el `pb-6` o, en
// el teléfono, la reserva del «Subir» flotante, que ya lleva su margen. La reserva no
// pasa de lo que deja la lista en el desplazamiento: abajo del todo, sus dos filas se
// ven enteras aunque en una ventana muy baja el botón llegue a la segunda.
func (c *columnaLista) pieCompacta() float32 {
	if c.reserva == 0 {
		return PieLista
	}
	return max(PieLista, min(c.reserva, c.altoVista-c.altoFilas))
}

// viewportListaCarpetas conserva una tarjeta entera cuando el 45 % del cuerpo no
// alcanza para su icono, nombre y subtítulo. En ese caso hay un solo desplazamiento
// alrededor de las carpetas naturales y de dos filas virtualizadas: ningún Scroll
// de la franja se queda el gesto vertical antes de poder llegar a los archivos.
type viewportListaCarpetas struct {
	col             *columnaLista
	contenido       *fyne.Container
	scroll          *container.Scroll
	normal, natural fyne.CanvasObject
	// reserva pone la del «Subir» flotante donde toca según la franja sea compacta o
	// no (Explorador.ajustarReserva); repetirla sin cambios no hace nada.
	reserva func(compacta bool)
}

// franjaCompacta dice si en un cuerpo de ese alto el 45 % no llega a una tarjeta entera.
func franjaCompacta(alto float32) bool {
	return TopeFranja*alto < RellenoFranja+AltoTeselaFranja+RellenoFranjaAbajo
}

func (*viewportListaCarpetas) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, ui.AltoCabeceraDrive)
}

func (v *viewportListaCarpetas) preparar(t fyne.Size) {
	v.col.ancho, v.col.altoVista = t.Width, t.Height
	v.col.compacta = franjaCompacta(t.Height)
	if v.reserva != nil && t.Height > 0 {
		v.reserva(v.col.compacta)
	}
	v.col.franja = v.normal
	if v.col.compacta {
		v.col.franja = v.natural
	}
	v.contenido.Objects[3] = v.col.franja
}

func (v *viewportListaCarpetas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	v.preparar(t)
	v.contenido.Resize(fyne.NewSize(t.Width, max(t.Height, v.col.MinSize(nil).Height)))
	v.scroll.Move(fyne.NewPos(0, 0))
	v.scroll.Resize(t)
	v.scroll.Refresh()
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
	cortes := CortesRejillaDrive
	if ui.EsTactil() {
		// En el teléfono, dos columnas no dejan sitio para el icono y los dos
		// blancos de selección y menú de 44 px.
		cortes[0] = 1
	}
	rej := ui.RejillaTarjetas(e.Ventana, HuecoFranja, cortes)
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

// franjaEnFilas es el alto de la franja con `tope` recortado a filas ENTERAS de teselas:
// al 45 % a secas la última fila visible quedaba partida bajo la cabecera «NOMBRE» (en el
// teléfono, en Marco, media tarjeta con el nombre tapado). Las demás filas se alcanzan
// desplazando la franja. Si no cabe ni una, el tope tal cual: en el táctil esa franja ya
// va entera dentro del desplazamiento del cuerpo (`viewportListaCarpetas`).
func franjaEnFilas(tope float32) float32 {
	filas := int((tope - RellenoFranja - RellenoFranjaAbajo + HuecoFranja) / (AltoTeselaFranja + HuecoFranja))
	if filas < 1 {
		return tope
	}
	return RellenoFranja + float32(filas)*AltoTeselaFranja + float32(filas-1)*HuecoFranja + RellenoFranjaAbajo
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
// es lo que evita el salto de layout al llegar los datos: la cabecera de columnas y las
// filas comunes de carga (`ui.FilasEsqueleto`, el `SkeletonRows` de la web) con la tesela
// de 36 y el tamaño y la fecha, que se esconden por corte de ventana como las de verdad.
//
// Solo se porta el de la LISTA. En rejilla la web pinta diez tarjetas y aquí se pintan
// las mismas nueve filas: es la misma promesa —«esto está llegando»— y la única
// consecuencia es un salto de layout al cambiar a tarjetas mientras carga, que dura lo
// que tarde la respuesta. ponytail: se cierra copiando la geometría de la tarjeta en
// cuanto haya una medida de la tarjeta de carga.
func (e *Explorador) esqueletoCuerpo(v tema.Variante) fyne.CanvasObject {
	haySM := e.Ventana == 0 || e.Ventana >= ui.CorteSM
	hayMD := e.Ventana == 0 || e.Ventana >= ui.CorteMD
	filas := ui.NuevasFilasEsqueleto(FilasEsqueleto, ui.DelanteroTesela, 1)
	filas.Margen = CanalonLista
	filas.Rotulo = e.t("files.loading")
	filas.Meta = []ui.MetaEsqueleto{{Ancho: 44, Oculta: !haySM}, {Ancho: 76, Oculta: !hayMD}}
	// La cabecera (el `header` de la web): tres rayas de `0.6rem` donde van los tres
	// rótulos de columna, con `py-2.5` y su `border-b`, la del tamaño a `ml-8` de la de
	// la fecha. Va DENTRO de las filas: un solo latido y un solo «Cargando».
	raya := float32(0.6 * tema.Rem)
	filas.ConCabecera(ui.CabeceraEsqueleto{
		Alto:      altoCabeceraEsqueleto,
		Filete:    true,
		Rotulo:    ui.MetaEsqueleto{Ancho: 56, Alto: raya},
		Meta:      []ui.MetaEsqueleto{{Ancho: 48, Alto: raya, Oculta: !haySM}, {Ancho: 72, Alto: raya, Oculta: !hayMD}},
		HuecoMeta: 2 * tema.Espacio(4),
	})
	return container.New(esqueletoLista{}, canvas.NewRectangle(tema.Color(ui.FondoHoja(), v)), filas)
}

// esqueletoLista pone la hoja debajo y las filas, con su cabecera, encima; no pide
// nada (la vista ya mide lo suyo) y las filas se alargan si la hoja es más alta.
type esqueletoLista struct{}

var _ fyne.Layout = esqueletoLista{}

func (esqueletoLista) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

// altoCabeceraEsqueleto es `py-2.5` y su `border-b`: 2×10 + 9,6 + 1.
var altoCabeceraEsqueleto = 2*(2.5*0.25*tema.Rem) + 0.6*tema.Rem + tema.Borde

func (esqueletoLista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(t.Width, max(t.Height, o.MinSize().Height)))
	}
}

// ---- La rejilla ----

// rejilla es la vista de tarjetas: `grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4
// xl:grid-cols-5` dentro de un `px-6 py-4`.
//
// Es VIRTUAL, como el `GridWrap` de Fyne: solo existen las tarjetas de las filas que se
// ven y una por encima y por debajo (`rejillaVirtual`), y al desplazar se hacen las que
// entran. Con 2.000 elementos se rehacían 2.000 tarjetas en cada repintado.
func (e *Explorador) rejilla(v tema.Variante) fyne.CanvasObject {
	e.tarjetas = e.tarjetas[:0]
	rej := &rejillaVirtual{
		rejillaDrive: rejillaDrive{rej: ui.RejillaTarjetas(e.Ventana, HuecoRejilla, CortesRejillaDrive), ancho: &e.anchoRejilla},
		e:            e, elementos: e.Elementos, hechas: map[int]tarjetaPintada{}, desde: -1,
	}
	rej.c = container.New(rej)
	var interior fyne.CanvasObject = rej.c
	if e.HayMas {
		// La rejilla no pagina sola: `flex justify-center pt-5` con «Cargar más».
		mas := ui.NuevoBoton(e.t("files.loadMore"), e.AlCargarMas).Con(ui.Secundario, ui.TallaSM)
		mas.Cargando = e.CargandoMas
		interior = container.New(&conPie{separa: SeparaCargarMas}, interior, mas)
	}
	// El desplazable es el MISMO de un pintado a otro: con uno nuevo, cada repintado
	// devolvía la rejilla arriba.
	interior = ui.ConReservaFlotante(interior) // el hueco del «Subir» flotante
	if e.desplazable == nil {
		e.desplazable = container.NewVScroll(interior)
	} else {
		e.desplazable.Content = interior
	}
	if e.arriba {
		e.desplazable.ScrollToTop()
		e.arriba = false
	}
	e.rejillaViva = rej
	e.desplazable.OnScrolled = func(fyne.Position) {
		if e.rejillaViva == rej {
			rej.alDesplazar()
		}
	}
	rej.ponerVentana()
	e.desplazable.Refresh()
	// ponytail: en la rejilla el marcado se ve pero no se desplaza hasta él (la rejilla
	// no sabe en qué fila cae cada tesela hasta medirla); se consume para que no salte
	// al volver a la lista.
	e.AVista = ""
	fondo := canvas.NewRectangle(tema.Color(ui.FondoHoja(), v))
	refresco := e.Refresco
	refresco.Arriba = func() bool { return e.desplazable.Offset.Y <= 0.5 }
	cuerpo := ui.RefrescarAlTirar(e.desplazable, &refresco)
	return container.New(&marcoRejilla{hijo: cuerpo, alColocar: rej.alDesplazar}, fondo, cuerpo)
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

// rejillaVirtual es la cuadrícula de `rejilla` con solo las tarjetas que se ven: las
// filas que caen en el desplazable y FilasDeMas por encima y por debajo, para que las
// flechas y el tabulador tengan adónde ir. Todas miden lo mismo (la medida de la
// tarjeta al ancho de columna), así que la cuenta de qué fila cae dónde es exacta, y
// el mínimo es el de la cuadrícula entera.
type rejillaVirtual struct {
	rejillaDrive
	e         *Explorador
	elementos []Elemento
	c         *fyne.Container
	// hechas son las tarjetas montadas, por índice; [desde, hasta) es lo que se ve.
	hechas       map[int]tarjetaPintada
	desde, hasta int
}

// FilasDeMas son las filas que se montan fuera de la vista por cada lado.
const FilasDeMas = 1

func (r *rejillaVirtual) MinSize([]fyne.CanvasObject) fyne.Size {
	n := len(r.elementos)
	ancho := *r.ancho
	if ancho <= 0 || n == 0 {
		// Antes del primer Layout, el de la rejilla con todas: las tarjetas miden
		// todas lo mismo, así que vale con una repetida.
		if n == 0 || len(r.c.Objects) == 0 {
			return r.rej.MinSize(nil)
		}
		todas := make([]fyne.CanvasObject, n)
		for i := range todas {
			todas[i] = r.c.Objects[0]
		}
		return r.rej.MinSize(todas)
	}
	cols := r.rej.Columnas(ancho)
	filas := (n + cols - 1) / cols
	alto := r.altoTarjeta(r.rej.AnchoColumna(ancho))
	return fyne.NewSize(ancho, float32(filas)*alto+float32(filas-1)*r.rej.Hueco)
}

func (r *rejillaVirtual) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	*r.ancho = t.Width
	r.ponerVentana()
	r.colocar()
}

// colocar pone cada tarjeta montada en su celda.
func (r *rejillaVirtual) colocar() {
	ancho := *r.ancho
	if ancho <= 0 {
		return
	}
	cols := r.rej.Columnas(ancho)
	col := r.rej.AnchoColumna(ancho)
	alto := r.altoTarjeta(col)
	for i := r.desde; i < r.hasta; i++ {
		o := r.hechas[i].objeto
		o.Move(fyne.NewPos(r.rej.X(ancho, i%cols), float32(i/cols)*(alto+r.rej.Hueco)))
		o.Resize(fyne.NewSize(col, alto))
	}
}

// ventana es [desde, hasta): las tarjetas de las filas que se ven, y FilasDeMas.
func (r *rejillaVirtual) ventana() (int, int) {
	n := len(r.elementos)
	ancho := *r.ancho
	s := r.e.desplazable
	if n == 0 {
		return 0, 0
	}
	cols, paso := 1, float32(0)
	if ancho > 0 {
		cols = r.rej.Columnas(ancho)
		paso = r.altoTarjeta(r.rej.AnchoColumna(ancho)) + r.rej.Hueco
	}
	arriba, visible := float32(0), float32(0)
	if s != nil {
		arriba, visible = s.Offset.Y, s.Size().Height
	}
	if visible <= 0 {
		visible = ventanaDe(r.c).Height
	}
	if paso <= 0 {
		// Sin medir todavía: lo que quepa en la ventana con la tarjeta de reserva.
		paso = MedidaTarjetaDrive(0).Height + r.rej.Hueco
	}
	primera := max(int(arriba/paso)-FilasDeMas, 0)
	ultima := int((arriba+visible)/paso) + 1 + FilasDeMas
	return min(primera*cols, n), min(ultima*cols, n)
}

// ponerVentana monta las tarjetas que entran en la vista y suelta las que salen.
func (r *rejillaVirtual) ponerVentana() bool {
	desde, hasta := r.ventana()
	if desde == r.desde && hasta == r.hasta {
		return false
	}
	r.desde, r.hasta = desde, hasta
	tonos := ui.TonosTarjeta()
	for i := range r.hechas {
		if i < desde || i >= hasta {
			delete(r.hechas, i)
		}
	}
	objs := make([]fyne.CanvasObject, 0, hasta-desde)
	pintadas := make([]tarjetaPintada, 0, hasta-desde)
	for i := desde; i < hasta; i++ {
		tp, ok := r.hechas[i]
		if !ok {
			el := r.elementos[i]
			t, o := r.e.nuevaTarjeta(el)
			// Una carpeta es una tarjeta de recurso, con el tono por turno
			// (`CARD_TONES[index % CARD_TONES.length]`): solo pinta en la forma Marco.
			if el.Carpeta {
				t.Tono = tonos[i%len(tonos)]
			}
			tp = tarjetaPintada{el: el, objeto: o, t: t, i: i}
			r.hechas[i] = tp
		}
		objs = append(objs, tp.objeto)
		pintadas = append(pintadas, tp)
	}
	r.c.Objects = objs
	if r.e.rejillaViva == r {
		r.e.tarjetas = pintadas
	}
	return true
}

// alDesplazar monta lo que entra en la vista al desplazar.
func (r *rejillaVirtual) alDesplazar() {
	if r.ponerVentana() {
		r.colocar()
		r.c.Refresh()
	}
}

// irA monta la tarjeta i (desplazando hasta su fila si no se ve) y la devuelve.
func (r *rejillaVirtual) irA(i int) (tarjetaPintada, bool) {
	if i < 0 || i >= len(r.elementos) {
		return tarjetaPintada{}, false
	}
	if tp, ok := r.hechas[i]; ok {
		return tp, true
	}
	if s := r.e.desplazable; s != nil && *r.ancho > 0 {
		cols := r.rej.Columnas(*r.ancho)
		paso := r.altoTarjeta(r.rej.AnchoColumna(*r.ancho)) + r.rej.Hueco
		s.ScrollToOffset(fyne.NewPos(s.Offset.X, float32(i/cols)*paso))
	}
	r.alDesplazar()
	tp, ok := r.hechas[i]
	return tp, ok
}

// vecino es a qué tarjeta lleva una tecla desde la i en la cuadrícula ENTERA: es lo que
// `ui.VecinoEnRejilla` mide sobre las cajas, contado, porque las filas que no se ven no
// tienen tarjeta. Las columnas son iguales y van alineadas, así que la más cercana de
// la fila de debajo es la misma columna, o la última de una fila incompleta.
func (r *rejillaVirtual) vecino(i int, tecla fyne.KeyName) int {
	n := len(r.elementos)
	cols := 1
	if *r.ancho > 0 {
		cols = r.rej.Columnas(*r.ancho)
	}
	if tecla == fyne.KeyDown {
		if (i/cols+1)*cols >= n {
			return -1
		}
		return min(i+cols, n-1)
	}
	if tecla == fyne.KeyUp {
		if i < cols {
			return -1
		}
		return i - cols
	}
	// ← → Inicio y Fin van en orden de lectura: solo cuenta el total.
	return ui.VecinoEnRejilla(make([]ui.CajaRejilla, n), i, tecla)
}

// ventanaDe es el tamaño del lienzo en el que está, o el del escritorio de referencia.
func ventanaDe(o fyne.CanvasObject) fyne.Size {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		return c.Size()
	}
	return fyne.NewSize(1280, 800)
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
	t.RotuloCasilla = e.t("files.select", i18n.Vars{"name": el.Nombre})
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
	t.AlFlecha = func(tecla fyne.KeyName) bool { return e.flechaEnTarjetas(t, tecla) }
	if ConMiniatura(el) {
		if img, ya := e.Miniaturas[el.ID]; ya {
			t.Miniatura = img
		} else if e.AlMiniatura != nil {
			e.AlMiniatura(el)
		}
	}
}

// flechaEnTarjetas mueve el foco desde la tarjeta `t` con las flechas, Inicio y Fin: es
// el `moverFocoEnRejilla` de `FileGrid`. Las tarjetas de este pintado son las de la
// rejilla o las de la franja de carpetas, nunca las dos, y cada una vive en su
// desplazable, que es el que se mueve para que se vea la enfocada.
func (e *Explorador) flechaEnTarjetas(t *TarjetaDrive, tecla fyne.KeyName) bool {
	objs := make([]fyne.CanvasObject, len(e.tarjetas))
	actual := -1
	for i, p := range e.tarjetas {
		objs[i] = p.objeto
		if p.t == t {
			actual = i
		}
	}
	if actual < 0 {
		return false
	}
	s := e.franjaScroll
	if r := e.rejillaViva; e.Rejilla && r != nil {
		// La cuadrícula es virtual: el destino se cuenta sobre todos los elementos y
		// se monta si no se ve.
		destino, ok := r.irA(r.vecino(e.tarjetas[actual].i, tecla))
		if !ok {
			return false
		}
		f, es := destino.objeto.(fyne.Focusable)
		c := fyne.CurrentApp().Driver().CanvasForObject(r.c)
		if !es || c == nil {
			return false
		}
		c.Focus(f)
		ui.MostrarFoco(e.desplazable, c, 0)
		r.alDesplazar() // desplazar por código no avisa a OnScrolled
		return true
	}
	return ui.MoverFocoEnRejilla(objs, actual, tecla, s)
}

// listaFilas es el `widget.List` de la vista lista con el teclado de la web. En la web
// cada fila es un botón y las flechas pasan el foco de una a otra (`moverFocoEnListaVirtual`);
// aquí el foco es de la lista, que ya mueve su resalte con ↑ y ↓ y lo lleva a la vista. Le
// faltaban Inicio y Fin, y que Intro y Espacio ABRAN la fila resaltada, como un botón:
// Espacio la seleccionaba y nadie escucha la selección.
type listaFilas struct {
	widget.List
	alAbrir func(int)
	// reales es el número de filas de verdad, sin la de la reserva del «+» flotante que
	// `ui.ReservarEnLista` le añade a `Length`: Fin y ↓ no van más allá.
	reales    func() int
	resaltada widget.ListItemID
	// sincronizando: OnHighlighted también salta al pasar el ratón por una fila, sin mover
	// el resalte del teclado; solo vale cuando lo provoca la propia lista (teclas y foco).
	// Si no, Intro abría la fila de debajo del puntero y no la resaltada.
	sincronizando bool
}

func nuevaListaFilas(
	alAbrir func(int), largo func() int, crear func() fyne.CanvasObject,
	rellenar func(widget.ListItemID, fyne.CanvasObject),
) *listaFilas {
	l := &listaFilas{alAbrir: alAbrir, reales: largo}
	l.Length, l.CreateItem, l.UpdateItem = largo, crear, rellenar
	l.OnHighlighted = func(i widget.ListItemID) {
		if l.sincronizando {
			l.resaltada = i
		}
	}
	l.ExtendBaseWidget(l)
	return l
}

// abrirFila abre la fila `i` de la lista, si existe.
func (e *Explorador) abrirFila(i int) {
	if l := e.filasDeLista(); i >= 0 && i < len(l) && e.AlAbrir != nil {
		e.AlAbrir(l[i])
	}
}

// TypedKey: Intro abre, Inicio y Fin saltan a los extremos y ↑ ↓ son de la lista.
func (l *listaFilas) TypedKey(ev *fyne.KeyEvent) {
	l.sincronizando = true
	defer func() { l.sincronizando = false }()
	switch ev.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		l.alAbrir(l.resaltada)
	case fyne.KeySpace:
		// Abre TypedRune. La lista la SELECCIONARÍA, y una fila seleccionada se queda
		// pintada con el color de selección sin que nadie la haya marcado.
	case fyne.KeyDown:
		// La fila de la reserva del «+» flotante no es de nadie: ↓ se para en la última
		// de verdad, como Fin.
		if l.resaltada >= l.reales()-1 {
			return
		}
		l.List.TypedKey(ev)
	case fyne.KeyHome, fyne.KeyEnd:
		if d := ui.VecinoEnLista(l.resaltada, l.reales(), ev.Name); d >= 0 {
			antes := l.resaltada
			l.Highlight(d)
			l.RefreshItem(antes)
			l.RefreshItem(d)
		}
	default:
		l.List.TypedKey(ev)
	}
}

// FocusGained: la lista avisa de su resalte al recibir el foco.
func (l *listaFilas) FocusGained() {
	l.sincronizando = true
	defer func() { l.sincronizando = false }()
	l.List.FocusGained()
}

// TypedRune: Espacio abre, como en un botón.
func (l *listaFilas) TypedRune(r rune) {
	if r == ' ' {
		l.alAbrir(l.resaltada)
		return
	}
	l.List.TypedRune(r)
}

// ConMiniatura es el `thumbable` de la web (lib/miniaturas.ts): el servicio hace
// miniaturas de JPEG, PNG, GIF y WebP (las de WebP llegan como PNG, así que aquí no
// hace falta decodificarlo). Manda el content-type, que es lo que mira el servicio; la
// extensión solo cuenta cuando el fichero no lo trae.
func ConMiniatura(el Elemento) bool {
	if el.Carpeta || el.ID == "" {
		return false
	}
	if tipo, _, _ := strings.Cut(el.Tipo, ";"); strings.TrimSpace(tipo) != "" {
		switch strings.ToLower(strings.TrimSpace(tipo)) {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
			return true
		}
		return false
	}
	switch Extension(el.Nombre) {
	case "jpg", "jpeg", "png", "gif", "webp":
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
type marcoRejilla struct {
	hijo fyne.CanvasObject
	// alColocar avisa de que el desplazable ha cambiado de tamaño: con una ventana más
	// alta, la cuadrícula virtual tiene que montar las filas que ahora se ven.
	alColocar func()
}

var _ fyne.Layout = (*marcoRejilla)(nil)

func (m *marcoRejilla) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (m *marcoRejilla) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	m.hijo.Move(fyne.NewPos(CanalonRejilla, RellenoRejilla))
	m.hijo.Resize(fyne.NewSize(max(t.Width-2*CanalonRejilla, 0), max(t.Height-2*RellenoRejilla, 0)))
	if m.alColocar != nil {
		m.alColocar()
	}
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
	// i es su índice en la cuadrícula (solo en la rejilla, que es virtual).
	i int
}

// arrastrar empieza o sigue un arrastre desde `el` con el puntero en `abs`: si `el` forma
// parte de la selección viaja la selección ENTERA —arrastrar uno de doce marcados y
// mover solo ese es lo contrario de lo que acaba de pedir quien los marcó—, y si no, solo
// él. El destino es la carpeta de debajo, si acepta algo de lo que viaja.
func (e *Explorador) arrastrar(el Elemento, abs fyne.Position) {
	e.seguirScrollArrastre(el, abs)
	e.marcarCarpetaArrastre(el, abs)
}

func (e *Explorador) seguirScrollArrastre(el Elemento, abs fyne.Position) {
	if e.Rejilla && e.desplazable != nil {
		e.autoArrastre.Actualizar(e.desplazable, abs, func(_, dy float32) {
			e.desplazable.ScrollToOffset(fyne.NewPos(0, e.desplazable.Offset.Y+dy))
			if r := e.rejillaViva; r != nil {
				r.alDesplazar() // desplazar por código no avisa a OnScrolled
			}
			e.marcarCarpetaArrastre(el, abs)
		})
	} else if e.filas != nil {
		e.autoArrastre.Actualizar(e.filas, abs, func(_, dy float32) {
			e.filas.ScrollToOffset(e.filas.GetScrollOffset() + dy)
			e.marcarCarpetaArrastre(el, abs)
		})
	}
}

func (e *Explorador) marcarCarpetaArrastre(el Elemento, abs fyne.Position) {
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
	e.autoArrastre.Parar()
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

func (e *Explorador) cancelarArrastre() {
	e.autoArrastre.Parar()
	antes := e.destino
	e.arrastrando, e.destino = nil, ""
	e.marcarDestino(antes)
}

type filaTactilDrive struct {
	widget.BaseWidget
	fila     *ui.FilaDrive
	asa      *ui.AsaArrastre
	el       Elemento
	origen   *Elemento
	revision uint64
}

func (e *Explorador) nuevaFilaTactil() *filaTactilDrive {
	a := &filaTactilDrive{fila: ui.NuevaFilaDrive("", ""), asa: ui.NuevaAsaArrastre(e.t("action.move"))}
	a.asa.Puede = func() bool {
		return !e.SinRed() && !e.Ocupado && e.AlMoverA != nil && a.el.ID != "" &&
			(a.origen == nil || e.Refresco.Revision == nil || e.Refresco.Revision() == a.revision)
	}
	a.asa.AlArrastrar = func(pos fyne.Position) {
		if a.origen == nil {
			origen := a.el // una fila virtual puede reciclarse durante el autoscroll
			a.origen = &origen
			if e.Refresco.Revision != nil {
				a.revision = e.Refresco.Revision()
			}
		}
		e.arrastrar(*a.origen, pos)
	}
	a.asa.AlSoltar = func(fyne.Position) {
		a.origen = nil
		e.soltar()
	}
	a.asa.AlCancelar = func() {
		a.origen = nil
		e.cancelarArrastre()
	}
	a.asa.AlPulsar = func() {
		if e.AlMenu != nil {
			e.AlMenu(a.el, a.asa)
		}
	}
	a.ExtendBaseWidget(a)
	return a
}

// CreateRenderer: la fila se queda todo el ancho menos el del asa, que va a la derecha.
// Con `ui.FilaAuto` cada hijo se quedaba en su mínimo y el nombre no salía en táctil.
func (a *filaTactilDrive) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, nil, a.asa, a.fila))
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
