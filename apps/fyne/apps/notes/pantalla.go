// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	pet "git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Pantalla es la aplicación entera: el armazón con su menú, el panel del árbol de
// notas y la vista de la sección activa. Es lo único de este paquete que habla con
// Sytue Notepads.
//
// El estado de cada vista vive en su propio tipo (`Lateral`, `Barra`, `Lienzo`,
// `ArbolNotas`, `Busqueda`, `Papelera`) y es una función pura de sus campos; aquí solo
// se rellenan esos campos y se vuelve a pintar. Es lo que hace que los guardianes de
// píxel no necesiten ni red ni servicio.
type Pantalla struct {
	// Widget captura la sesión adoptada antes de consultar el árbol completo.
	Widget widgets.Source
	// peticion es la franja de «Pedir acceso de edición» del cuaderno abierto, y
	// cuadernoDelAviso el del aviso de una petición que atiende su dueño (peticiones.go).
	peticion         pet.Estado
	cuadernoDelAviso string
	svc              *api.Notes
	tr               *i18n.Translator
	loc              i18n.Locale
	red              *offline.Monitor
	win              fyne.Window

	lateral  *Lateral
	barra    *Barra
	lienzo   *Lienzo
	arbol    *ArbolNotas
	busqueda *Busqueda
	papelera *Papelera
	fijadas  *Fijadas
	archivo  *Archivo
	avisos   *ui.PilaAvisos

	vista     Vista
	cuadernos []Cuaderno
	activo    Cuaderno
	notas     []Nota
	etiquetas []api.Tag
	// recordatorios son los míos, por nota (recordatorios.go).
	recordatorios map[string]api.NoteReminder
	orden         Orden
	filtro        string
	// filtroEstado es el estado de etiqueta por el que se filtra; vacío, ninguno.
	filtroEstado string
	// bloqueado oculta también los datos legados hasta verificar la clave local.
	bloqueado bool

	// hist es el deshacer y rehacer del cuaderno: crear, editar, mover y eliminar.
	hist Historial
	// creando corta el doble clic en «Nueva nota». Solo se toca en el hilo de la
	// interfaz.
	creando          bool
	enfoque          string
	abriendoDia      bool
	guardandoCaptura bool
	campoCaptura     *ui.Entrada
	tipoCaptura      string
	// capturaEnfocada y lineaCaptura son el `focus-within` de la fila de captura: la
	// línea de debajo pasa a `primary`.
	capturaEnfocada bool
	lineaCaptura    *canvas.Rectangle
	// pistaCaptura es la pista de la sintaxis: solo se ve con el foco en la captura.
	pistaCaptura  fyne.CanvasObject
	añadirCaptura *ui.BotonIcono
	capturaInerte bool
	borradores    map[string]string
	// dia es lo que el diario trae además de sus notas (semana.go).
	dia estadoDiario
	// cola es lo capturado que aún no ha llegado al servicio (cola.go); vaciando corta
	// dos envíos a la vez y vigilando arranca la vigilancia una sola vez.
	cola      []capturaPendiente
	vaciando  atomic.Bool
	vigilando sync.Once
	// Almacen es donde se guardan la cola y los borradores (`sesion.Almacen`). Nil: sin
	// red no se captura, como antes.
	Almacen *offline.Store
	// ajustes es el diálogo de ajustes abierto, para cerrarlo al guardar.
	ajustes *ui.Dialogo

	raiz *fyne.Container
	// desplazador es el del lienzo, el mismo en cada repintado (ver `desplazar`).
	desplazador *container.Scroll
	// animAncla es el desplazamiento suave hasta el título de un enlace, si va a medias.
	animAncla *fyne.Animation
	// imagenes son las de las notas de imagen, por dirección (ver `imagenDe`), y docs
	// los documentos de las notas `doc`, por ID (ver `docDe`).
	imagenes map[string]imagenRemota
	docs     map[string]docRemoto
	// ancla es el título (su slug) al que llevar la nota abierta en cuanto se pinte: el
	// `h=` de un enlace a una nota (`ancla` de NotesApp.tsx).
	ancla string
	// entrante es lo que un enlace entrante pide y todavía no se ha podido hacer
	// (entrante.go).
	entrante *Entrante
	// vinculoPendiente es la nota vinculada que pide un enlace entrante y todavía no se
	// ha creado: está preguntada, y la crea `aceptarVinculo` si se dice que sí.
	vinculoPendiente *api.NoteInput
	// clave es la del cuaderno cifrado abierto, mientras está abierto (ver cifrado.go).
	clave            []byte
	cryptoEpoch      uint64
	migracionCancel  context.CancelFunc
	migrando         bool
	migracionFallida bool
	GeneracionSesion func() string
	// edicion es la nota abierta en el editor, si la hay.
	edicion *sesionEdicion
	// notasDeOtros son las notas de otros cuadernos que ha pedido la ficha de un enlace
	// (previa.go), por cuaderno; nil es «pedidas y en camino».
	notasDeOtros map[string]*[]Nota
	// portal es la capa sin layout de los flotantes: el menú de «/», la burbuja y la
	// ficha de un enlace (flotante.go).
	portal *portal
	// cajon es el menú abierto por encima en el móvil. Activar algo del menú lo cierra:
	// el cajón tapa lo que se acaba de elegir.
	cajon bool
	// Plazo es cuánto se espera a una petición. Se deja abierto para el guardián.
	Plazo time.Duration
	// AppURL compone una dirección de la WEB de Notepads (`/p/<token>` del enlace
	// público). La pone quien sabe el dominio de la suite; sin ella no se enseña.
	AppURL func(ruta string) string
	// AbrirCalendar abre el día del cuaderno en la app nativa o en su web.
	AbrirCalendar func(ruta string)
	// Docs es el servicio de Sytue Docs, con la misma sesión: de él sale la vista previa
	// de una nota `doc`. nil: la caja se queda vacía.
	Docs *api.Docs
	// Suite son los clientes de las otras apps con la sesión de la persona: «Compartir
	// en Rooms» y los ficheros de Drive de la herramienta de enlaces (suite.go). Nil, no
	// se ofrecen.
	Suite *api.Elegir
	// Recordar es lo que hace falta para abrir un cuaderno cifrado con Touch ID, Face ID
	// o Windows Hello (recordar.go). Nil: no se ofrece, y todo queda como en la web.
	Recordar *Recordador
	// Directorio traduce el correo con que se comparte al `sub` con que se guarda, y de
	// vuelta para decir quién me compartió algo (compartidas.go). Nil: no se ofrece
	// compartir con personas.
	Directorio *api.Directorio
	// Yo es el `sub` de quien usa la app: solo el dueño comparte un cuaderno, y solo quien
	// la escribió, una nota.
	Yo func() string
	// Org es la organización activa: la cola y los borradores se guardan por persona Y
	// organización (claveAlmacen), o lo capturado en una saldría con el token de otra.
	Org func() string
	// comp es «Compartido conmigo» (compartidas.go).
	comp estadoCompartidas
	// tareas es la vista «Tareas» (tareas.go).
	tareas estadoTareas
	// ocultarTerminadas quita del cuaderno las tareas hechas o canceladas.
	ocultarTerminadas bool
	// Asistir es Sytue AI (`api.IA.Asistir`): con él, el editor lleva el asistente al
	// final de la barra (asistente.go). Nil: no sale.
	Asistir func(context.Context, api.PeticionIA) (string, error)
}

// ConServiciosVinculados añade el acceso a las páginas del proveedor sin copiarlas
// ni convertirlas a notas nativas.
func (p *Pantalla) ConServiciosVinculados(abrir func()) {
	p.lateral.AlServiciosVinculados = nil
	if abrir != nil {
		p.lateral.AlServiciosVinculados = func() { p.cajon = false; p.Repintar(); abrir() }
	}
	p.Repintar()
}

// clavePrefOrden es donde se recuerda el orden de lectura, el `sytue.notes.orden` que la
// web guarda en `localStorage`. Va con guiones y no con puntos: es una preferencia, no
// una clave del catálogo, y con forma de clave el guardián de `catalogos` la buscaría
// en los mensajes.
const clavePrefOrden = "sytue-notes-orden"

// clavePrefOcultarTerminadas recuerda «Ocultar completadas» (el `localStorage` de la web).
const clavePrefOcultarTerminadas = "sytue-notes-ocultar-terminadas"

// NuevaPantalla monta la aplicación. `svc` puede ser nil: la pantalla se pinta igual y
// se queda en la fase que se le ponga, que es lo que hace el guardián.
func NuevaPantalla(tr *i18n.Translator, l i18n.Locale, svc *api.Notes, red *offline.Monitor, win fyne.Window) *Pantalla {
	p := &Pantalla{
		svc: svc, tr: tr, loc: l, red: red, win: win,
		Plazo: 30 * time.Second, avisos: ui.NuevaPilaAvisos(), orden: OrdenManual,
	}
	if o := fyne.CurrentApp().Preferences().String(clavePrefOrden); EsOrden(o) {
		p.orden = Orden(o)
	}
	p.ocultarTerminadas = fyne.CurrentApp().Preferences().Bool(clavePrefOcultarTerminadas)
	p.lateral = NuevoLateral(tr)
	p.barra = NuevaBarra(tr)
	p.lienzo = NuevoLienzo(tr)
	p.portal = nuevoPortal()
	p.lienzo.portal = p.portal
	p.arbol = NuevoArbolNotas(tr, l)
	p.busqueda = NuevaBusqueda(tr)
	p.papelera = NuevaPapelera(tr)
	p.fijadas, p.archivo = &Fijadas{tr: tr}, &Archivo{tr: tr}
	p.enganchar()
	p.engancharCompartidas()
	p.atajos()
	return p
}

// Objeto arma el armazón. La TOPBAR no está: existe solo en la web y en nativo la
// sustituye el cromo de la ventana (CONVENTIONS.md §13.6, excepción 1).
func (p *Pantalla) Objeto() fyne.CanvasObject {
	p.raiz = container.New(&raiz{p: p, corte: corteDe(p.anchoVentana())}, p.armazon(p.variante()), p.portal.c, p.avisos)
	return p.raiz
}

// raiz apila el armazón y los avisos, y vigila los CORTES de la ventana: la barra y el
// árbol se arman distinto por debajo de `sm` y de `md`, y redimensionar la ventana no
// repinta nada por sí solo. Al cruzar un corte, se repinta.
type raiz struct {
	p     *Pantalla
	corte int
	ancho float32
}

func (r *raiz) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return layout.NewStackLayout().MinSize(objs)
}

func (r *raiz) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	layout.NewStackLayout().Layout(objs, t)
	if c := corteDe(t.Width); c != r.corte || t.Width != r.ancho {
		r.corte, r.ancho = c, t.Width
		// `fyne.Do` desde la rutina de la interfaz ENCOLA: el repintado va después de
		// este Layout y no cambia los hijos que se están colocando ahora mismo. (El
		// driver de pruebas lo ejecuta en el acto, en la misma rutina.)
		fyne.Do(r.p.Repintar)
	}
}

// corteDe es en qué tramo de la hoja está un ancho de ventana: 0 por debajo de `sm`, 1
// hasta `md`, 2 desde `md`. Son los dos cortes que cambian cómo se ARMA la pantalla; los
// demás los resuelven los propios layouts al colocar.
func corteDe(ancho float32) int {
	switch {
	case ancho < ui.CorteSM:
		return 0
	case ancho < ui.Estrecho:
		return 1
	}
	return 2
}

func (p *Pantalla) armazon(v tema.Variante) *ui.Armazon {
	a := ui.NuevoArmazon(p.contenido(v))
	a.Menu = p.lateral.Objeto(v)
	// El panel secundario SOLO existe en la vista de árbol, que es cuando hay una lista
	// de notas a la izquierda y una nota a la derecha. En las otras tres disposiciones
	// el lienzo ocupa el panel entero.
	conPanel := p.vista == VistaCuaderno && p.activo.Vista == VistaArbol
	if conPanel {
		// `border-border bg-surface border-r`: el panel es papel blanco con su filete.
		a.Panel = container.New(&conFondo{}, canvas.NewRectangle(tema.Color("surface", v)),
			canvas.NewRectangle(tema.Color("border", v)), p.arbol.Objeto(v))
		a.ClavePreferenciaPanel = "sytue.notes.panel-width"
		a.DetalleAbierto = p.lienzo.Seleccionada != ""
	}
	a.BarraMovil = p.barraMovil(conPanel, a.DetalleAbierto)
	a.CajonAbierto = p.cajon
	a.AlCajon = func(abierto bool) { p.cajon = abierto } // el velo lo cierra solo
	return a
}

// conFondo pone un fondo entero, un filete de un píxel a la derecha y el contenido.
type conFondo struct{}

func (conFondo) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[2].MinSize() }

func (conFondo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	objs[1].Move(fyne.NewPos(t.Width-tema.Borde, 0))
	objs[1].Resize(fyne.NewSize(tema.Borde, t.Height))
	objs[2].Move(fyne.NewPos(0, 0))
	objs[2].Resize(fyne.NewSize(t.Width-tema.Borde, t.Height))
}

// AltoBarraMovil es el `h-11` de la barra del AppShell en el móvil.
var AltoBarraMovil = ui.AltoBarraMovil

// barraMovil configura la navegación compartida: conserva el estado de Notepads y
// deja al armazón pintar el marco continuo, también al cambiar la apariencia.
func (p *Pantalla) barraMovil(conPanel, detalle bool) *ui.BarraMovil {
	t := p.tr.T
	b := ui.NuevaBarraMovil()
	b.ConLista, b.EnDetalle, b.CajonAbierto = conPanel, conPanel && detalle, p.cajon
	b.RotuloAbrir, b.RotuloCerrar, b.RotuloVolver = t("ui.openNav", nil), t("ui.closeNav", nil), t("ui.backToList", nil)
	b.AlMenu = func() {
		p.cajon = !p.cajon
		p.Repintar()
	}
	b.AlVolver = func() {
		p.lienzo.Seleccionada = ""
		p.Repintar()
	}
	b.Titulo = p.activo.Titulo
	// En un día, el título ya lo escribe la página: aquí, el de la app.
	if b.Titulo == "" || p.activo.Crudo.DailyDate != "" {
		b.Titulo = t("app.title", nil)
	}
	return b
}

// contenido es el panel principal de la vista activa, con su barra si la lleva.
func (p *Pantalla) contenido(v tema.Variante) fyne.CanvasObject {
	switch p.vista {
	case VistaBusqueda:
		return p.busqueda.Objeto(v)
	case VistaPapelera:
		return p.papelera.Objeto(v)
	case VistaCompartidas:
		return p.vistaCompartidas(v)
	case VistaTareas:
		return p.vistaTareas(v)
	case VistaFijadas:
		return p.fijadas.Objeto(v)
	case VistaArchivo:
		p.archivo.Cuadernos = raicesArchivadas(p.cuadernos)
		return p.archivo.Objeto(v)
	case VistaCuaderno:
	}
	if p.activo.ID == "" {
		// Sin ningún cuaderno: el estado vacío que invita a crear el primero, que es
		// lo que hace la web con `empty.noNotebooks`.
		x := ui.NuevoVacio(p.tr.T("empty.noNotebooks.title", nil),
			p.tr.T("empty.noNotebooks.body", nil))
		x.Ilustracion = iconos.Pintar(iconos.VacioNotas, v).A(LadoIlustracion).Objeto() // `Illustration name="emptyNotes"`
		x.Accion = ui.NuevoBoton(p.tr.T("sidebar.newNotebook", nil), p.lateral.AlNuevoCuaderno)
		return centrar(x, v)
	}
	// Un día y un cuaderno en lista son páginas: su título y sus acciones los escribe el
	// panel (dashboard), sin barra encima.
	var franjas []fyne.CanvasObject
	if !enPagina(p.activo) {
		franjas = append(franjas, p.barra.Objeto(v))
	}
	// El aviso de «sin conexión» va SOBRE el contenido y debajo de la barra, que es
	// donde lo pone la web (`role="status"`).
	if aviso := p.avisoMigracion(); aviso != nil {
		franjas = append(franjas, aviso)
	}
	if !p.hayRed() {
		franjas = append(franjas, ui.NuevoAviso(ui.AvisoAdvertencia,
			p.tr.T("common.offline", nil), ""))
	}
	if f := p.franjaPeticion(); f != nil {
		franjas = append(franjas, f)
	}
	// Y el cartel de SOLO LECTURA de la nota abierta, que es la regla que manda: si el
	// documento no vuelve entero, se dice antes de que nadie escriba nada.
	if n, ok := p.notaAbierta(); ok {
		if c := Cartel(p.tr, n, v); c != nil {
			franjas = append(franjas, c)
		}
	}
	p.lienzo.AnchoVentana = p.anchoVentana()
	// En árbol el panel es UNA nota: ni captura ni filtros encima de ella (las notas
	// nuevas nacen en la lista de la izquierda).
	var bloques []ui.Bloque
	if p.activo.Vista != VistaArbol {
		bloques = append(bloques, ui.Bloque{Obj: p.dashboard(v)})
	}
	if d, err := time.ParseInLocation(time.DateOnly, p.activo.Crudo.DailyDate, time.Local); err == nil && p.vistaDiario() != VistaDia {
		bloques = append(bloques, ui.Bloque{Obj: p.vistaDias(d, v)})
	} else {
		bloques = append(bloques, ui.Bloque{Obj: p.lienzo.Objeto(v)})
		if e := p.esteDia(v); err == nil && e != nil {
			bloques = append(bloques, ui.Bloque{Obj: e})
		}
	}
	contenido := ui.PilaBloques(bloques...)
	franjas = append(franjas, p.desplazar(ui.AlAnchoFluido(p.anchoContenido(), contenido), p.lienzo.Cuaderno.Vista == Libre))
	return container.New(pilaElastica{elastico: len(franjas) - 1}, franjas...)
}

// desplazar mete el lienzo en su desplazador (`min-h-0 flex-1 overflow-auto`), que es
// SIEMPRE el mismo: con uno nuevo en cada repintado, marcar una tarea del fondo de la
// lista devolvía el cuaderno arriba del todo. El libre se desplaza en los dos ejes.
func (p *Pantalla) desplazar(o fyne.CanvasObject, libre bool) fyne.CanvasObject {
	if p.desplazador == nil {
		p.desplazador = container.NewScroll(o)
	}
	p.desplazador.Content = o
	p.desplazador.Direction = container.ScrollVerticalOnly
	if libre {
		p.desplazador.Direction = container.ScrollBoth
	}
	p.desplazador.Refresh()
	return p.desplazador
}

// notaAbierta es la seleccionada, si la hay.
func (p *Pantalla) notaAbierta() (Nota, bool) {
	for _, n := range p.notas {
		if n.ID == p.lienzo.Seleccionada {
			return n, true
		}
	}
	return Nota{}, false
}

// anchoVentana es el de la ventana, o el de escritorio si no hay ninguna (guardianes).
func (p *Pantalla) anchoVentana() float32 {
	if p.win == nil || p.win.Canvas().Size().Width == 0 {
		return 1280
	}
	return p.win.Canvas().Size().Width
}

func (p *Pantalla) variante() tema.Variante {
	return tema.De(fyne.CurrentApp().Settings().ThemeVariant())
}

// Repintar rehace el árbol con el estado de ahora. Se llama SIEMPRE desde la rutina
// principal: quien venga de una goroutine pasa por `enHilo`.
func (p *Pantalla) Repintar() {
	if p.raiz == nil {
		return
	}
	p.sincronizar()
	// Repintar rehace las tarjetas: la ficha de un enlace era de una prosa que ya no está.
	p.portal.poner(clavePrevia)
	p.raiz.Objects[0] = p.armazon(p.variante())
	p.raiz.Refresh()
	// En el pintado en que la nota ya está —la de otro cuaderno llega después—, al título.
	if a := p.ancla; a != "" {
		if _, ok := p.nota(p.lienzo.Seleccionada); ok {
			p.ancla = ""
			fyne.Do(func() { p.irAlAncla(a) })
		}
	}
}

// irAlAncla desplaza el lienzo hasta el título con ese slug de la nota abierta: es el
// `scrollIntoView({behavior: 'smooth', block: 'start'})` de la web.
func (p *Pantalla) irAlAncla(slug string) {
	s := p.desplazador
	n, ok := p.nota(p.lienzo.Seleccionada)
	if s == nil || s.Content == nil || !ok {
		return
	}
	i, ok := bloqueDeAncla(n.Doc, slug)
	if !ok {
		return
	}
	t := p.lienzo.vivas[n.ID]
	if t == nil || t.prosa == nil || t.prosa.maq == nil || i >= len(t.prosa.maq.Bloques) {
		return
	}
	prosa := t.prosa
	d := fyne.CurrentApp().Driver()
	y := d.AbsolutePositionForObject(prosa).Y - d.AbsolutePositionForObject(s.Content).Y + prosa.maq.Bloques[i].Y
	tope := max(s.Content.MinSize().Height-s.Size().Height, 0)
	p.animAncla = ui.DesplazarSuave(s, min(max(y, 0), tope), p.animAncla)
}

// bloqueDeAncla es el bloque del título con ese slug, contados como `titulosDe`: en
// orden, y el segundo título igual con «-2».
func bloqueDeAncla(d *Documento, slug string) (int, bool) {
	if d == nil {
		return 0, false
	}
	vistos := map[string]int{}
	for i, b := range d.Bloques {
		texto := strings.TrimSpace(b.Texto())
		if b.Tipo != Titulo || texto == "" {
			continue
		}
		base := cmp.Or(slugDe(texto), "titulo")
		n := vistos[base]
		vistos[base] = n + 1
		if s := base; (n == 0 && s == slug) || (n > 0 && s+"-"+strconv.Itoa(n+1) == slug) {
			return i, true
		}
	}
	return 0, false
}

// sincronizar vuelca el estado de la pantalla en cada vista antes de pintarla. Está
// junto y no repartido para que no haya un campo que solo se actualice en un camino.
func (p *Pantalla) sincronizar() {
	visibles := Ordenar(Filtrar(enfocarNotas(p.notas, p.enfoque, p.ocultarTerminadas), p.filtro, p.filtroEstado), p.orden, p.loc)

	p.lateral.Cuadernos, p.lateral.Activo, p.lateral.EsMio = enArbol(p.cuadernos), p.activo.ID, p.esMio
	// Las del cuaderno abierto cuelgan de su fila TAL COMO LLEGARON: la web le pasa
	// `notes` y no las visibles, así que ni el filtro ni el orden las tocan ahí.
	p.lateral.Notas, p.lateral.NotaActiva = p.notas, p.lienzo.Seleccionada
	p.lateral.Vista, p.lateral.Bloqueado = p.vista, p.bloqueado

	p.barra.Cuaderno, p.barra.Filtro, p.barra.Orden = p.activo, p.filtro, p.orden
	p.barra.Etiquetas, p.barra.FiltroEstado = p.etiquetas, p.filtroEstado
	p.barra.Bloqueado, p.barra.SinRed = p.bloqueado, !p.hayRed()
	p.barra.Deshacer, p.barra.Rehacer = p.hist.SiguienteDeshacer(), p.hist.SiguienteRehacer()
	p.barra.Estrecha = p.anchoVentana() < ui.CorteSM
	// «Compartir con personas» del cuaderno: solo su dueño, con red y con directorio.
	p.barra.AlCompartir = nil
	if p.Directorio != nil && p.svc != nil && p.hayRed() && p.esMio(p.activo.Crudo.OwnerSub) {
		p.barra.AlCompartir = p.compartirCuaderno
	}

	p.lienzo.Cuaderno, p.lienzo.Bloqueado = p.activo, p.bloqueado
	// Un cuaderno público de otra persona se lee y no se escribe: su franja ofrece pedir
	// acceso de edición (peticiones.go).
	p.lienzo.SoloLectura = !p.hayRed() || p.activo.Crudo.ReadOnly || p.migrando
	p.arbol.Notas, p.arbol.Bloqueado = visibles, p.bloqueado
	p.arbol.SoloLectura, p.arbol.Seleccionada = p.lienzo.SoloLectura, p.lienzo.Seleccionada
	p.arbol.Fase = p.lienzo.Fase

	// En árbol el panel principal es UNA nota a todo el ancho. Se reutiliza la
	// disposición de LISTA con una sola nota: es exactamente eso, y así no hay una
	// segunda tarjeta que mantener. Es lo que hace la web.
	if p.activo.Vista == VistaArbol {
		p.lienzo.Cuaderno.Vista, p.lienzo.Llenar = Lista, true
		p.lienzo.Notas = nil
		if n, ok := p.notaAbierta(); ok {
			p.lienzo.Notas = []Nota{n}
		}
		return
	}
	p.lienzo.Llenar, p.lienzo.Notas = false, visibles
}

// enHilo ejecuta algo en la rutina de la interfaz. `fyne.Do` es obligatorio desde Fyne
// 2.6 para tocar la interfaz desde otra rutina, y aquí hace falta de verdad: la carga
// corre en su propia rutina para no congelar la ventana.
func enHilo(f func()) { fyne.Do(f) }

func (p *Pantalla) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

// hayRed dice si el monitor de conectividad ve red. Sin monitor se asume que sí: no
// tenerlo no es estar desconectado.
func (p *Pantalla) hayRed() bool {
	return p.red == nil || p.red.State() != offline.Offline
}

// Arrancar hace la primera carga.
func (p *Pantalla) Arrancar() {
	go p.cargarCuadernos()
	go p.cargarEtiquetas()
	go p.cargarRecordatorios()
	if p.Almacen != nil {
		p.cola = pendientesDe(p.Almacen, p.claveAlmacen())
	}
	p.vigilarCola()
}

func (p *Pantalla) cargarCuadernos() {
	widget := p.Widget.View()
	conRed := p.hayRed()
	var generacion string
	fyne.DoAndWait(func() { generacion = p.generacionCrypto() })
	enHilo(func() { p.lateral.Fase = Cargando; p.Repintar() })
	// Con almacén se pide igual sin red: el transporte sirve la última copia.
	if p.svc == nil || !p.hayRed() && p.Almacen == nil {
		enHilo(func() { p.lateral.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	lista, err := p.svc.ListNotebooks(ctx)
	if err == nil && conRed && p.hayRed() {
		// NoteCount ya aplica los permisos del lector. El árbol no se pagina y
		// cada nota pertenece a un cuaderno; no se cuentan las páginas de notas.
		if total, valido := notasParaWidget(lista); valido {
			_ = widget.Publicar(ctx, widgets.Counter{Kind: "notes", Value: total})
		}
	}
	cuadernos := DeCuadernos(lista)
	enHilo(func() {
		if p.generacionCrypto() != generacion {
			return
		}
		p.cuadernos = cuadernos
		p.lateral.Fase = FaseDe(err, p.hayRed(), len(cuadernos))
		// El abierto se queda, con sus datos frescos: es el `lista.find(n => n.id ===
		// prev.id) ?? prev` de la web, que es lo que hace que renombrarlo se vea.
		for _, c := range cuadernos {
			if c.ID == p.activo.ID {
				changed := c.Cifrado != p.activo.Cifrado || !bytes.Equal(c.Crudo.Crypto, p.activo.Crudo.Crypto)
				p.activo = c
				if changed {
					p.cancelarMigracion()
					p.creando = false
					p.olvidarClaveActiva()
					p.bloqueado = c.Cifrado
					p.edicion = nil
					p.lienzo.Editor = nil
					raw := make([]api.Note, len(p.notas))
					for i, n := range p.notas {
						raw[i] = n.Crudo
					}
					p.notas = p.deNotas(raw)
				}
			}
		}
		if p.entrante != nil && p.activo.ID == "" {
			p.aplicarEntrante() // el cuaderno del enlace, y no el primero
		}
		if visibles := enArbol(cuadernos); p.activo.ID == "" && len(visibles) > 0 {
			p.abrirCuaderno(visibles[0])
			return
		}
		p.Repintar()
	})
}

func notasParaWidget(lista []api.Notebook) (int64, bool) {
	ids := make(map[string]struct{}, len(lista))
	var total int64
	for _, c := range lista {
		if c.ArchivedAt != nil {
			continue
		}
		if _, repetido := ids[c.ID]; repetido {
			continue
		}
		if c.NoteCount < 0 || int64(c.NoteCount) > 1_000_000_000_000-total {
			return 0, false
		}
		ids[c.ID] = struct{}{}
		total += int64(c.NoteCount)
	}
	return total, true
}

func (p *Pantalla) cargarEtiquetas() {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	tags, err := p.svc.ListTags(ctx)
	if err != nil {
		return // las etiquetas son un extra: sin ellas la app funciona entera
	}
	enHilo(func() { p.etiquetas = tags; p.Repintar() })
}

// abrirCuaderno cambia de cuaderno y carga sus notas.
//
// AL CAMBIAR DE CUADERNO SE OLVIDA TODO LO DESCIFRADO. Aquí no hay nada descifrado
// todavía, pero la bandera se reinicia igual: dejarla puesta haría que un cuaderno
// cifrado se abriera como si ya lo estuviera.
func (p *Pantalla) abrirCuaderno(c Cuaderno) {
	p.cerrarEditor()
	if p.campoCaptura != nil && p.activo.ID != "" {
		p.guardarBorrador(p.activo.ID, p.campoCaptura.Texto())
	}
	p.campoCaptura = nil
	p.enfoque = "all"
	p.activo, p.vista, p.cajon = c, VistaCuaderno, false
	// AL CAMBIAR DE CUADERNO SE OLVIDA LA CLAVE, y con ella todo lo descifrado.
	p.cancelarMigracion()
	p.creando = false
	p.olvidarClaveActiva()
	p.bloqueado = c.Cifrado
	p.notas, p.lienzo.Seleccionada = nil, ""
	p.engancharPeticion(c)
	if p.desplazador != nil {
		p.desplazador.ScrollToTop() // otro cuaderno empieza por arriba
	}
	p.Repintar()
	go p.cargarNotas(c)
}

func (p *Pantalla) cargarNotas(c Cuaderno) {
	var generacion string
	var epoch uint64
	fyne.DoAndWait(func() { generacion, epoch = p.generacionCrypto(), p.cryptoEpoch })
	if esProvisional(c.ID) {
		// El día provisional no existe todavía: no hay nada que leer.
		enHilo(func() { p.notas, p.lienzo.Fase = nil, Vacia; p.Repintar() })
		return
	}
	enHilo(func() { p.lienzo.Fase = Cargando; p.Repintar() })
	if p.svc == nil || !p.hayRed() && p.Almacen == nil {
		enHilo(func() { p.lienzo.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	lista, err := p.svc.ListNotes(ctx, c.ID)
	enHilo(func() {
		if p.activo.ID != c.ID || p.generacionCrypto() != generacion || p.cryptoEpoch != epoch {
			return // se ha cambiado de cuaderno mientras llegaba: esto ya no vale
		}
		// Se convierten AQUÍ y no antes: si el cuaderno está abierto, con su clave.
		p.notas = p.deNotas(lista)
		if p.clave != nil && !p.migrando {
			p.migrarContenido()
		}
		p.lienzo.Fase = FaseDe(err, p.hayRed(), len(p.notas))
		p.aplicarEntrante()
		p.Repintar()
	})
}

func (p *Pantalla) cargarPapelera() {
	enHilo(func() { p.papelera.Fase = Cargando; p.Repintar() })
	if !p.hayRed() || p.svc == nil {
		enHilo(func() { p.papelera.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	lista, err := p.svc.ListTrash(ctx, limitePapelera)
	notas := DeNotas(lista)
	enHilo(func() {
		p.papelera.Notas = notas
		p.papelera.Fase = FaseDe(err, p.hayRed(), len(notas))
		p.Repintar()
	})
}

func (p *Pantalla) buscar(q string, archivados bool) {
	enHilo(func() { p.busqueda.Fase = Cargando; p.busqueda.Consulta = q; p.Repintar() })
	if !p.hayRed() || p.svc == nil {
		enHilo(func() { p.busqueda.Fase = SinConexion; p.Repintar() })
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	res, err := p.svc.Search(ctx, q, limiteBusqueda, archivados)
	enHilo(func() {
		p.busqueda.Resultados, p.busqueda.Buscada = res, q
		p.busqueda.Fase = FaseDe(err, p.hayRed(), len(res))
		p.Repintar()
	})
}

// cargarHashtags trae los hashtags del panel de búsqueda. Un fallo deja los que hubiera:
// son un atajo, y lo que no se ha podido buscar ya lo dice la lista.
func (p *Pantalla) cargarHashtags() {
	if !p.hayRed() || p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if tags, err := p.svc.Hashtags(ctx); err == nil {
		enHilo(func() { p.busqueda.Hashtags = tags; p.Repintar() })
	}
}

// Los dos topes de una consulta. Son los mismos que pide la web.
const (
	limitePapelera = 100
	limiteBusqueda = 50
)

// enganchar conecta las acciones de las vistas con el servicio.
func (p *Pantalla) enganchar() {
	l := p.lateral
	l.AlIrA = p.irA
	l.AlAbrir = p.abrirCuaderno
	l.AlAbrirNota = p.abrirNota
	l.AlPlegar = func(id string) { l.Cerrados[id] = !l.Cerrados[id]; p.Repintar() }
	l.AlNuevoCuaderno = func() { p.dialogoAjustes(nil, "") }
	l.AlNuevoDentro = func(c Cuaderno) { p.dialogoAjustes(nil, c.ID) }
	l.AlAjustes = func(c Cuaderno) { p.dialogoAjustes(&c, "") }
	l.AlEtiquetas = p.dialogoEtiquetas
	l.AlImportar = p.importar
	l.AlHoy = func() { p.abrirDia(time.Now().Format(time.DateOnly)) }
	l.AlDia = p.abrirDia
	// El candado del árbol abre el cuaderno y pide su contraseña (`onUnlock`).
	l.AlDesbloquear = func(c Cuaderno) {
		if c.ID != p.activo.ID {
			p.abrirCuaderno(c)
		}
		p.desbloquear()
	}

	b := p.barra
	b.AlFiltrar = func(s string) { p.filtro = s; p.Repintar() }
	b.AlOrdenar = func(o Orden) {
		p.orden = o
		// Se recuerda entre sesiones: quien prefiere ver lo último tocado lo prefiere
		// siempre. Es el `localStorage` de la web, con la misma clave.
		fyne.CurrentApp().Preferences().SetString(clavePrefOrden, string(o))
		p.Repintar()
	}
	b.AlFiltroEstado = func(s string) { p.filtroEstado = s; p.Repintar() }
	// La lupa de una página: abre el campo y le da el foco; con el campo vacío, lo cierra.
	b.AlAbrirFiltro = func() {
		b.Filtrando = !b.Filtrando
		p.Repintar()
		if b.Filtrando && p.win != nil {
			p.win.Canvas().Focus(b.campoFiltro())
		}
	}
	b.AlExportar = func(formato string, ver bool) { go p.exportar(p.activo.ID, "", formato, ver) }
	b.AlMenu = p.abrirMenu
	b.AlNuevaNota = p.nuevaNota
	b.AlAjustes = func() { c := p.activo; p.dialogoAjustes(&c, "") }
	b.AlDeshacer = func() { go p.deshacer() }
	b.AlRehacer = func() { go p.rehacer() }
	b.AlDesbloquear = p.desbloquear
	b.AlBloquear = p.bloquear

	p.lienzo.AlSeleccionar = p.abrirNota
	p.lienzo.AlMarcar = func(n Nota, m bool) {
		lanzar(func() error { return p.editar(n.ID, Parche{Marcada: &m}, true) })
	}
	p.lienzo.AlRecordatorio = p.dialogoRecordatorio
	p.lienzo.AlReintentar = func() { go p.cargarNotas(p.activo) }
	p.lienzo.AlCrearPrimera = p.nuevaNota
	p.lienzo.AlTecla = p.tecla
	p.lienzo.AlTeclaEspecial = p.teclaEspecial
	p.lienzo.AlMenuNota = func(n Nota, d fyne.CanvasObject) { p.abrirMenu(d, p.menuNota(n, false)) }

	p.arbol.AlSeleccionar = p.abrirNota
	p.arbol.AlNueva = p.nuevaNota
	p.arbol.AlPlegar = func(n Nota, plegada bool) {
		lanzar(func() error { return p.editar(n.ID, Parche{Plegada: &plegada}, true) })
	}
	p.arbol.AlMenuNota = func(n Nota, d fyne.CanvasObject) { p.abrirMenu(d, p.menuNota(n, true)) }
	p.arbol.AlRenombrar = p.renombrarEnArbol
	p.lienzo.AlRenombrar = p.renombrar
	p.lienzo.AlEnlace = p.seguirEnlace
	p.lienzo.Previa = p.previaDe
	p.lienzo.AlPlegarLista = p.plegarLista
	p.lienzo.AlEditar = p.abrirEditor
	p.lienzo.AlMoverLibre = func(n Nota, en fyne.Position) { lanzar(func() error { return p.moverLibre(n, en) }) }
	p.lienzo.AlSoltarEnDia = p.soltarEnDia
	p.lienzo.AlEnsanchar = func() {
		p.lienzo.Ancha = !p.lienzo.Ancha
		p.Repintar()
		if p.edicion != nil {
			p.enfocar(p.edicion.editor.Zona())
		}
	}
	p.lienzo.AlDescargar = p.descargarAdjunto
	p.lienzo.Imagen, p.lienzo.Documento = p.imagenDe, p.docDe
	p.lienzo.AlQuitarAdjunto = p.quitarAdjunto

	p.busqueda.AlBuscar = func(q string) { go p.buscar(q, p.busqueda.Archivados) }
	p.busqueda.AlAbrir = p.abrirResultado

	p.fijadas.AlAbrir = p.abrirResultado
	p.archivo.AlAbrir, p.archivo.EsMio = p.abrirCuaderno, p.esMio
	p.archivo.AlDesarchivar = func(c Cuaderno) { go p.archivar(c, false) }
	p.papelera.AlRestaurar = func(n Nota) { go p.restaurar(n) }
	p.papelera.AlPurgar = p.purgar
	p.papelera.AlVaciar = p.vaciarPapelera
}

// nuevaNota es «Nueva nota», venga de la barra, del estado vacío, del árbol o de la
// tecla `n`.
func (p *Pantalla) nuevaNota() { lanzar(func() error { return p.crear(api.NoteInput{Kind: "text"}) }) }

// atajos engancha al lienzo de la ventana lo que la web escucha en `document`: Ctrl+Z y
// Ctrl+Mayús+Z (o Ctrl+Y) deshacen y rehacen el CUADERNO, y la `n` crea una nota.
//
// Dentro de un campo no llegan, y es lo que se quiere: el campo que tiene el foco se
// queda el atajo —Fyne se lo da primero al que lo sabe atender— y ahí deshacer es letra a
// letra, como en la web.
func (p *Pantalla) atajos() {
	if p.win == nil {
		return
	}
	c := p.win.Canvas()
	atajo := func(k fyne.KeyName, m fyne.KeyModifier, f func()) {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: k, Modifier: m}, func(fyne.Shortcut) { f() })
	}
	atajo(fyne.KeyZ, fyne.KeyModifierShortcutDefault, func() { go p.deshacer() })
	atajo(fyne.KeyZ, fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift, func() { go p.rehacer() })
	atajo(fyne.KeyY, fyne.KeyModifierShortcutDefault, func() { go p.rehacer() })
	// Los del diario, como la web: ⌘D hoy y ⌘⇧←/→ el día de al lado (⌘K lo cede la
	// barra de la suite: `EnfocarCaptura`).
	atajo(fyne.KeyD, fyne.KeyModifierShortcutDefault, func() { p.atajoDiario("today") })
	atajo(fyne.KeyLeft, fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift, func() { p.atajoDiario("previous") })
	atajo(fyne.KeyRight, fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift, func() { p.atajoDiario("next") })
	// «Ir a» (abrir_rapido.go): Ctrl/⌘+P y Ctrl/⌘+O, como la web.
	atajo(fyne.KeyP, fyne.KeyModifierShortcutDefault, p.abrirIrA)
	atajo(fyne.KeyO, fyne.KeyModifierShortcutDefault, p.abrirIrA)
	c.AddShortcut(&fyne.ShortcutPaste{}, p.pegar)
	// Alt+flechas mueve la nota seleccionada: la alternativa de teclado al arrastre
	// (WCAG 2.1.1), que en la web escucha `document`.
	for _, k := range []fyne.KeyName{fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown} {
		atajo(k, fyne.KeyModifierAlt, func() { p.moverConTeclado(k) })
	}
	// Las teclas sueltas solo llegan aquí cuando NO hay nada enfocado; con una tarjeta
	// enfocada las recibe ella y las pasa a `tecla` y a `teclaEspecial`.
	c.SetOnTypedRune(p.tecla)
	c.SetOnTypedKey(p.teclaEspecial)
}

// teclaEspecial atiende Escape: quita la selección, que es la única forma de hacerlo en
// la web (no hay «clic en el vacío», que sería un manejador sin equivalente de teclado).
func (p *Pantalla) teclaEspecial(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyEscape && p.lienzo.Seleccionada != "" && p.hayRed() {
		p.lienzo.Seleccionada = ""
		p.Repintar()
	}
}

// moverLibre coloca una nota soltada en el lienzo libre: `{x, y, col: 0, position}` de
// `empezarArrastre`, redondeado.
//
// ponytail: la web no manda `group_id` y el servicio entiende que la saca de su grupo;
// aquí se conserva, como en Alt+flechas.
func (p *Pantalla) moverLibre(n Nota, en fyne.Position) error {
	pos := posicionDe(n.Crudo)
	pos.X, pos.Y, pos.Col = int(math.Round(float64(en.X))), int(math.Round(float64(en.Y))), 0
	return p.colocar(n.ID, pos, true)
}

// PasoTeclado es cuánto mueve Alt+flecha una nota en el lienzo libre.
const PasoTeclado = 24

// moverConTeclado es el Alt+flecha de NoteCanvas.tsx: en el libre desplaza la nota 24
// px, y en las demás cambia de columna (izquierda, derecha) o de orden (arriba, abajo).
func (p *Pantalla) moverConTeclado(k fyne.KeyName) {
	n, ok := p.nota(p.lienzo.Seleccionada)
	if !ok || !p.hayRed() || p.vista != VistaCuaderno {
		return
	}
	pos := posicionConTeclado(n, k, p.activo.Vista == Libre)
	lanzar(func() error { return p.colocar(n.ID, pos, true) })
}

// posicionConTeclado es adónde lleva Alt+flecha una nota.
func posicionConTeclado(n Nota, k fyne.KeyName, libre bool) api.NotePosition {
	pos := posicionDe(n.Crudo)
	if libre {
		switch k {
		case fyne.KeyLeft:
			pos.X = max(0, pos.X-PasoTeclado)
		case fyne.KeyRight:
			pos.X += PasoTeclado
		case fyne.KeyUp:
			pos.Y = max(0, pos.Y-PasoTeclado)
		case fyne.KeyDown:
			pos.Y += PasoTeclado
		default:
		}
	} else {
		switch k {
		case fyne.KeyLeft:
			pos.Col = max(0, pos.Col-1)
		case fyne.KeyRight:
			pos.Col = min(NumColumnas-1, pos.Col+1)
		case fyne.KeyUp:
			pos.Position = max(0, pos.Position-1)
		case fyne.KeyDown:
			pos.Position++
		default:
		}
	}
	return pos
}

// tecla atiende una tecla suelta fuera de todo campo: es `esAtajoDeApp` de la web.
//
// Con una nota seleccionada, primero los atajos de estado de etiqueta: un carácter que
// alguien ha asignado a «Alta» es de esa persona antes que de la app.
func (p *Pantalla) tecla(r rune) {
	if tag, estado, ok := p.atajoDeEstado(r); ok {
		nota := p.lienzo.Seleccionada
		go p.marcarEstado(nota, tag, estado)
		return
	}
	if r == 'n' && p.vista == VistaCuaderno && !p.bloqueado && p.hayRed() {
		p.nuevaNota()
	}
}

// atajoDeEstado es la etiqueta y el estado cuyo atajo es esa tecla, si hay una nota
// seleccionada a la que aplicarlo.
func (p *Pantalla) atajoDeEstado(r rune) (tag, estado string, ok bool) {
	if p.lienzo.Seleccionada == "" || p.vista != VistaCuaderno {
		return "", "", false
	}
	for _, t := range p.etiquetas {
		for _, s := range t.States {
			if s.Shortcut != "" && s.Shortcut == string(r) {
				return t.ID, s.ID, true
			}
		}
	}
	return "", "", false
}

func (p *Pantalla) irA(v Vista) {
	if v == VistaTareas {
		p.verTareas(api.TareaAbierta)
		return
	}
	p.cerrarEditor()
	p.vista, p.cajon = v, false
	p.Repintar()
	if v == VistaPapelera {
		go p.cargarPapelera()
	}
	if v == VistaFijadas {
		go p.cargarFijadas()
	}
	if v == VistaCompartidas {
		p.comp.abierta, p.comp.fase = "", Cargando
		p.Repintar()
		go p.cargarCompartidas()
	}
	if v == VistaBusqueda {
		go p.buscar("", p.busqueda.Archivados)
		go p.cargarHashtags()
		// `autofocus` del buscador: se abre para escribir.
		if p.win != nil {
			p.win.Canvas().Focus(p.busqueda.Campo())
		}
	}
}

func (p *Pantalla) abrirNota(id string) {
	p.lienzo.Seleccionada, p.cajon = id, false
	p.Repintar()
}

// abrirResultado salta a la nota de un resultado de búsqueda, cambiando de cuaderno si
// hace falta. Sin recargar nada: es lo que hace `navegarA` en la web.
func (p *Pantalla) abrirResultado(h api.NoteHit) {
	if h.NotebookID == p.activo.ID {
		p.vista = VistaCuaderno
		p.abrirNota(h.ID)
		return
	}
	for _, c := range p.cuadernos {
		if c.ID == h.NotebookID {
			p.abrirCuaderno(c)
			p.lienzo.Seleccionada = h.ID
			p.Repintar()
			return
		}
	}
}

func (p *Pantalla) restaurar(n Nota) {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.RestoreNote(ctx, n.ID); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return
	}
	p.cargarPapelera()
	// Vuelve a su cuaderno: si es el abierto, que se vea ya.
	p.releerNotas(ctx)
}

// purgar borra para siempre, con su confirmación: no hay vuelta atrás.
func (p *Pantalla) purgar(n Nota) {
	t := p.tr.T
	p.confirmar(t("trash.purge", nil), t("trash.purgeConfirm", nil), t("trash.purge", nil),
		func() { go p.purgarYa(n) })
}

func (p *Pantalla) purgarYa(n Nota) {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.PurgeNote(ctx, n.ID); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return
	}
	p.cargarPapelera()
}

// seguirEnlace es el clic en un enlace del cuerpo: a OTRA nota de esta app se navega
// dentro, sin salir (`destinoInterno` de la web); cualquier otro lo abre el sistema, que
// es el `target="_blank"` que el servicio pone a todos.
func (p *Pantalla) seguirEnlace(href string) {
	if cuaderno, nota, ancla, ok := destinoInterno(href, p.AppURL); ok {
		p.ancla = ancla
		p.abrirResultado(api.NoteHit{ID: nota, NotebookID: cuaderno})
		return
	}
	_ = api.AbrirFuera(fyne.CurrentApp(), href)
}

// destinoInterno reconoce un enlace a una nota de Notepads: mismo origen que la app y
// `?notebook=…&note=…`, con el título al que ir en `h=` si lo trae.
func destinoInterno(href string, appURL func(string) string) (cuaderno, nota, ancla string, ok bool) {
	if appURL == nil {
		return "", "", "", false
	}
	u, err1 := url.Parse(href)
	base, err2 := url.Parse(appURL("/"))
	if err1 != nil || err2 != nil || u.Scheme != base.Scheme || u.Host != base.Host {
		return "", "", "", false
	}
	q := u.Query()
	cuaderno, nota = q.Get("notebook"), q.Get("note")
	return cuaderno, nota, q.Get("h"), cuaderno != "" && nota != ""
}

// vaciarPapelera es «Vaciar la papelera», con su confirmación.
func (p *Pantalla) vaciarPapelera() {
	t := p.tr.T
	notas := p.papelera.Notas
	p.confirmar(t("trash.emptyAll", nil), t("trash.emptyAllConfirm", i18n.Vars{"count": len(notas)}),
		t("trash.emptyAll", nil), func() { go p.vaciarYa(notas) })
}

// vaciarYa borra cada nota por su lado y deja en la papelera SOLO las que fallaron: es el
// `Promise.allSettled` de la web. Una que no se pudo borrar sigue ahí para reintentarla,
// y el aviso dice cuántas.
func (p *Pantalla) vaciarYa(notas []Nota) {
	if p.svc == nil {
		return
	}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		fallidas = map[string]bool{}
	)
	for _, n := range notas {
		wg.Go(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			if p.svc.PurgeNote(ctx, n.ID) != nil {
				mu.Lock()
				fallidas[n.ID] = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	enHilo(func() {
		p.papelera.Notas = slices.DeleteFunc(slices.Clone(p.papelera.Notas), func(n Nota) bool { return !fallidas[n.ID] })
		if len(p.papelera.Notas) == 0 {
			p.papelera.Fase = Vacia
		}
		t := p.tr.T
		if len(fallidas) == 0 {
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, t("trash.emptied", i18n.Vars{"count": len(notas)}), ""))
		} else {
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, t("trash.emptyFailed", i18n.Vars{"count": len(fallidas)}), ""))
		}
		p.Repintar()
	})
}

// abrirMenu saca un menú superpuesto colgando de su disparador. La colocación —que no
// se salga de la ventana, que se vuelque arriba si no cabe abajo— la hace `ui.Menu`.
func (p *Pantalla) abrirMenu(disparador fyne.CanvasObject, m *ui.Menu) {
	if p.win == nil || m == nil || disparador == nil {
		return
	}
	m.Mostrar(disparador)
}

// avisar apila un aviso desde cualquier rutina.
func (p *Pantalla) avisar(tono ui.TonoAviso, titulo, descripcion string) {
	enHilo(func() { p.avisos.Notificar(ui.NuevoAviso(tono, titulo, descripcion)) })
}
