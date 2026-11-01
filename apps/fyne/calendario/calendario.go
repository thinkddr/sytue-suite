// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package calendario es la interfaz de Sytue Calendar en Fyne.
//
// paridad-web: apps/frontend/apps/calendar/src/pages/index.astro
// paridad-web: apps/frontend/apps/calendar/src/pages/events/new.astro
//
// Las dos rutas van juntas a propósito y no es un atajo: están MEDIDAS y devuelven la
// misma pantalla. El volcado de producción da 285 elementos y los mismos 19 estilos de
// texto en `/calendar` y en `/calendar/events/new`, píxel por píxel. `events/new` es un
// enlace profundo que abre el modal de creación sobre el mes, no una pantalla aparte, y
// aquí es lo mismo: `Pantalla.AbrirNuevoEvento`.
//
// Es UN producto, así que las dos entradas pueden vivir en el mismo fichero nativo: lo
// que `docs/CONVENTIONS.md` §13.7 prohíbe es que un fichero porte pantallas de DOS
// productos, y Sytue Calendar es uno.
//
// # De dónde salen los números
//
// De `test/e2e/medidas-web.mjs calendar` contra producción (corporate.sytue.eu,
// 1280×800, rem 14), y de las clases de `apps/frontend/apps/calendar/src/**`. Cada
// medida lleva «medido» u «hoja» en el comentario donde se declara.
//
//	cabecera de vista   46,5 (`max(min-h-12, 28 + 2×py-2.5 + borde)`)   medido
//	menú lateral        260, con 231 útiles                             medido
//	contenido           1020 de ancho                                   medido
//	fila de días        28 + filete                                     medido
//	semana              111,41 · seis                                   medido
//	celda               145,703 / 145,719                               medido
package calendario

import (
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Estado es el estado de carga de la pantalla, con los seis del encargo.
type Estado = agenda.Estado

// Pantalla es Sytue Calendar entero: menú, cabecera de vista y la vista activa.
//
// No habla con la red: recibe los datos ya cargados y avisa por callbacks. Así la
// pantalla se puede pintar en un test sin servidor y sin CGO, que es lo que hace el
// guardián.
type Pantalla struct {
	// T traduce con el catálogo de esta app.
	T *i18n.Translator
	// Cursor es el día que manda la vista.
	Cursor time.Time
	// Zona es la de visualización; nil, la del sistema.
	Zona *time.Location
	// Vista es cuál de las cuatro se enseña.
	Vista Vista
	// Estado son los seis del encargo. `agenda.Exito` pinta los datos.
	Estado Estado
	// Fallo es la clave del mensaje de error («no se pudieron cargar los calendarios» o
	// «los eventos»), el `error` de la web. Vacía: el de los eventos.
	Fallo string
	// CargandoCalendarios pone el esqueleto en «Mis calendarios». Va aparte de Estado
	// porque en la web el menú solo mira `loadingCals`: mientras llegan los EVENTOS de
	// otro mes, la lista de calendarios sigue en su sitio.
	CargandoCalendarios bool
	// Recargando es el refresco con datos previos: la rejilla se queda, atenuada al
	// 60 % y sin atender al puntero, en vez de volver al esqueleto en cada navegación.
	Recargando bool
	// Calendarios son los propios; Compartidos, los de «Compartidos conmigo».
	Calendarios []api.Calendar
	Compartidos []api.Calendar
	// Visibles son los IDs cuyos eventos se pintan. nil: todos.
	Visibles map[string]bool
	// Eventos son las ocurrencias del rango visible, ya expandidas.
	Eventos []Ocurrencia
	// Invitaciones es el contador de la cabecera. 0: sin insignia.
	Invitaciones int
	// Recordatorios son los vencidos sin descartar: el contador de la campana.
	Recordatorios []api.Reminder
	// SinConexion apaga las escrituras, como hace `useConnectivity` en la web.
	SinConexion bool
	// PuedeGestionarRecursos es `user.roles` con admin u owner: solo entonces sale el
	// botón de salas y recursos.
	PuedeGestionarRecursos bool
	// Importar es «Trae tus calendarios» (`ui.ImportarDatos`), en el pie del menú; nil,
	// el pie se queda vacío.
	Importar *ui.ImportarDatos
	// AlServiciosVinculados abre las cuentas y calendarios vivos de otros proveedores.
	AlServiciosVinculados func()

	// peticiones son las franjas de «Pedir acceso de edición» de los compartidos de
	// lector, por calendario; delAviso, el calendario propio del aviso de una petición,
	// y peticionesDueno, su franja (peticiones.go).
	peticiones      map[string]*peticion.Estado
	delAviso        string
	peticionesDueno peticion.Estado

	// consultaAgenda es lo escrito en el buscador de la agenda. Vive aquí para que
	// sobreviva a que la App reconstruya la pantalla.
	consultaAgenda string
	// pasoIman es el imantado del arrastre (15 o 30; 0 es 15).
	pasoIman int
	// scrollHorario es el desplazamiento de la rejilla horaria: se fija al MONTAR la
	// vista y se conserva al navegar de semana, como en la web.
	scrollHorario float32
	scrollPuesto  bool
	// pendiente es el bloque soltado que se está guardando; lo quita la App.
	pendiente *Fantasma
	// cuerpoHorario es el de la vista en pantalla, para mover la línea de la hora.
	cuerpoHorario *cuerpoHorario
	// cajonAbierto es el menú sacado como cajón en ventana estrecha.
	cajonAbierto bool

	// Los avisos hacia fuera. Todos pueden ser nil.
	AlIrHoy   func()
	AlNavegar func(delta int)
	// AlDesplazarMes son las flechas del mini-mes, que cambian de MES sea cual sea la
	// vista: `onShift` de la web es `addMonths`, no `navigate`.
	AlDesplazarMes    func(delta int)
	AlCambiarVista    func(Vista)
	AlElegirDia       func(time.Time)
	AlBuscar          func()
	AlVerInvitaciones func()
	// AlVerRecordatorios despliega el panel de la campana, que es lo que recibe.
	AlVerRecordatorios func(campana fyne.CanvasObject)
	AlCrearEvento      func(dia time.Time)
	// AlCrearRango crea con el rango pintado en la rejilla horaria.
	AlCrearRango func(inicio, fin time.Time)
	// AlMoverEvento guarda lo movido o estirado en la rejilla horaria.
	AlMoverEvento func(o Ocurrencia, inicio, fin time.Time)
	// AlAbrirEvento abre la vista rápida junto a `ancla`, el objeto pulsado.
	AlAbrirEvento        func(o Ocurrencia, ancla fyne.CanvasObject)
	AlConmutarCalendario func(id string)
	AlCrearCalendario    func()
	// AlMenuCalendario despliega el menú «⋮» de un calendario propio.
	AlMenuCalendario    func(c api.Calendar, disparador fyne.CanvasObject)
	AlSuscribirse       func()
	AlGestionarRecursos func()
	AlReintentar        func()
	Refresco            ui.RefrescoLista
	// AlConmutarCajon abre o cierra el menú en ventana estrecha.
	AlConmutarCajon func()
}

// t es azúcar para no repetir el nil-check del traductor en cada rótulo.
func (p *Pantalla) t(clave string, vars i18n.Vars) string {
	if p.T == nil {
		return clave
	}
	return p.T.T(clave, vars)
}

func (p *Pantalla) locale() i18n.Locale {
	if p.T == nil {
		return i18n.Default
	}
	return p.T.Locale()
}

// nombreZona es el nombre IANA de la zona de visualización (`browserZone()`).
func (p *Pantalla) nombreZona() string { return sistema.ZonaIANA(p.zona()) }

func (p *Pantalla) zona() *time.Location {
	if p.Zona == nil {
		return time.Local
	}
	return p.Zona
}

// visible dice si los eventos de ese calendario se pintan. Sin mapa, todos.
func (p *Pantalla) visible(id string) bool {
	if p.Visibles == nil {
		return true
	}
	return p.Visibles[id]
}

// Construir arma el árbol de la pantalla entera, listo para `SetContent`.
func (p *Pantalla) Construir() fyne.CanvasObject {
	a := ui.NuevoArmazon(p.contenido())
	a.Menu = p.menu()
	a.BarraMovil = p.nuevaBarraMovil()
	a.CajonAbierto = p.cajonAbierto
	a.AlCajon = func(abierto bool) {
		if p.cajonAbierto != abierto && p.AlConmutarCajon != nil {
			p.AlConmutarCajon()
		} else {
			p.cajonAbierto = abierto
		}
	}
	return a
}

// Titulo es el `titleFor(mode, cursor, locale)` de la web: el mes con su año, la
// semana como RANGO (`formatRange`, que une los extremos a la manera de cada lengua),
// el día entero con su año y, en la agenda, la fecha sin más: ahí la web no capitaliza.
func (p *Pantalla) Titulo() string {
	l := p.locale()
	switch p.Vista {
	case VistaDia:
		return capitalizar(i18n.FormatFullDate(l, p.Cursor))
	case VistaAgenda:
		return i18n.FormatDayMonthLong(l, p.Cursor)
	case VistaSemana:
		desde, _ := Rango(VistaSemana, p.Cursor, p.zona())
		return capitalizar(i18n.FormatDateRange(l, desde, desde.AddDate(0, 0, 6)))
	case VistaMes:
	}
	return capitalizar(i18n.FormatMonthYear(l, p.Cursor))
}

// capitalizar es el `capitalize()` de dates.ts: `Intl` devuelve «septiembre de 2026»
// en minúscula —en español el mes no es nombre propio— y la web lo pone en mayúscula
// para el título de la vista. Se vio en la captura: el título salía en minúscula donde
// producción pone «Septiembre de 2026».
//
// Va sobre RUNAS y no sobre bytes: en griego «Σεπτεμβρίου» y en checo «září» la primera
// letra no cabe en un byte, y `s[0]` la partiría por la mitad.
func capitalizar(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
	}
	return s
}

// cabecera arma la barra de control con sus dos grupos.
func (p *Pantalla) cabecera() *CabeceraVista {
	hoy := nuevoBotonInvitaciones(p.t("header.today", nil), 0, "", p.AlIrHoy)
	hoy.sinIcono = true
	izquierda := []fyne.CanvasObject{
		hoy,
		BotonIconoCabecera(iconos.ChevronIzquierda, p.t("nav.prev", nil), func() { p.navegar(-1) }),
		BotonIconoCabecera(iconos.ChevronDerecha, p.t("nav.next", nil), func() { p.navegar(1) }),
	}

	buscar := BotonIconoCabecera(iconos.Buscar, p.t("search.events", nil), p.AlBuscar)
	buscar.TamIcono = 18 // `<Icon name="search" size={18} />`

	// «Invitaciones» lleva `users` y NO `bell`: la campana ya es el centro de
	// recordatorios, que va justo a su izquierda, y dos campanas seguidas con
	// significados distintos no distinguen nada. Es el comentario de la web.
	invitaciones := nuevoBotonInvitaciones(p.t("header.invitations", nil), p.Invitaciones,
		p.t("header.pending", i18n.Vars{"count": p.Invitaciones}), p.AlVerInvitaciones)

	derecha := []fyne.CanvasObject{buscar, p.nuevaCampana(), invitaciones, NuevoGrupoVista([4]string{
		p.t("view.month", nil), p.t("view.week", nil),
		p.t("view.day", nil), p.t("view.agenda", nil),
	}, p.Vista, p.cambiarVista)}
	return NuevaCabeceraVista(p.Titulo(), izquierda, derecha)
}

func (p *Pantalla) navegar(d int) {
	if p.AlNavegar != nil {
		p.AlNavegar(d)
	}
}

func (p *Pantalla) cambiarVista(v Vista) {
	if p.AlCambiarVista != nil {
		p.AlCambiarVista(v)
	}
}

// contar escribe un entero pequeño sin traer `strconv` a la cabecera.
func contar(n int) string {
	if n <= 0 {
		return ""
	}
	d := make([]byte, 0, 4)
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// contenido es la cabecera y, debajo, la vista o el estado que toque.
func (p *Pantalla) contenido() fyne.CanvasObject {
	cuerpo := p.conFranjas(p.cuerpo())
	cab := p.cabecera()
	return container.New(&bajoCabecera{c: cab}, cab, cuerpo)
}

// cuerpo es la vista activa, o el panel del estado cuando no hay datos que enseñar.
func (p *Pantalla) cuerpo() fyne.CanvasObject {
	if panel := agenda.Panel(p.Estado, p.textosDeEstado(), p.esqueleto()); panel != nil {
		return panel
	}
	vista := p.vista()
	if p.Recargando {
		return container.NewStack(vista, nuevaCortina())
	}
	return vista
}

// vista es la vista activa con sus datos.
func (p *Pantalla) vista() fyne.CanvasObject {
	if p.Vista != VistaSemana && p.Vista != VistaDia {
		// Volver a semana o día vuelve a encuadrar la jornada, como un montaje nuevo.
		p.scrollPuesto, p.cuerpoHorario = false, nil
	}
	switch p.Vista {
	case VistaAgenda:
		return p.vistaAgenda()
	case VistaSemana, VistaDia:
		return p.vistaHoraria()
	case VistaMes:
	}
	m := NuevoMes(p.locale(), p.Cursor)
	m.Zona = p.zona()
	m.Eventos = p.Eventos
	m.SinTitulo = p.t("event.untitled", nil)
	m.MasRotulo = func(n int) string { return p.t("grid.more", i18n.Vars{"count": n}) }
	m.RotuloCrear = func(d time.Time) string {
		return p.t("nav.createEventOn", i18n.Vars{"date": i18n.FormatDayMonthLong(p.locale(), d)})
	}
	m.DesdeAntes = p.t("cal.fromEarlier", nil)
	m.AlCrear = p.AlCrearEvento
	m.AlAbrir = p.AlAbrirEvento
	// El mes conserva seis semanas legibles en horizontal: si no cabe su mínimo,
	// se recorre entero en vez de apretar los números y eventos unos sobre otros.
	_, vista := desplazable(m)
	return vista
}

// textosDeEstado es la copia de cada uno de los seis, con las claves del catálogo de
// la app. No se inventa ninguna: son las que la web ya tiene.
func (p *Pantalla) textosDeEstado() agenda.Textos {
	switch p.Estado {
	case agenda.Error:
		// `illustration={<Illustration name="errorRetry" />}`.
		return agenda.Textos{
			Titulo:       p.t("load.error", nil),
			Descripcion:  p.t(p.claveFallo(), nil),
			RotuloAccion: p.t("common.retry", nil),
			Accion:       p.AlReintentar,
			Ilustracion:  iconos.Pintar(iconos.ErrorReintentar, tema.De(fyne.CurrentApp().Settings().ThemeVariant())).Objeto(),
		}
	case agenda.SinConexion:
		// Sin red Calendar pasa a consulta: el reintento existe pero no se puede
		// pulsar, que es lo que hace `disabled={offline}` en la web.
		return agenda.Textos{
			Titulo:       p.t("load.error", nil),
			Descripcion:  p.t("cal.error.loadEvents", nil),
			RotuloAccion: p.t("common.retry", nil),
		}
	case agenda.SinPermiso:
		// Un 403 no se arregla reintentando, así que no lleva botón.
		return agenda.Textos{
			Titulo:      p.t("load.error", nil),
			Descripcion: p.t(p.claveFallo(), nil),
		}
	case agenda.Vacia:
		return agenda.Textos{
			Titulo:       p.t("cal.noUpcoming", nil),
			Descripcion:  p.t("agenda.empty", nil),
			RotuloAccion: p.t("nav.createEvent", nil),
			Accion:       p.crearHoy,
		}
	case agenda.Exito, agenda.Cargando:
	}
	return agenda.Textos{}
}

// claveFallo es la clave del mensaje de error: la que dejó la carga que falló o, sin
// ella, la de los eventos.
func (p *Pantalla) claveFallo() string {
	if p.Fallo != "" {
		return p.Fallo
	}
	return "cal.error.loadEvents"
}

func (p *Pantalla) crearHoy() {
	if p.AlCrearEvento != nil {
		p.AlCrearEvento(p.Cursor)
	}
}

// esqueleto es el `GridSkeleton` de la web: la GEOMETRÍA de la rejilla sin datos, para
// que cambiar de mes no dé un salto de layout.
func (p *Pantalla) esqueleto() fyne.CanvasObject {
	// La fila de nombres: siete bloques de `2rem × 0.75rem` con `px-2.5 py-2.5`.
	cabecera := make([]fyne.CanvasObject, 0, 7)
	for range 7 {
		cabecera = append(cabecera, conMargenSimple(espacio2p5,
			agenda.Bloque(2*tema.Rem, 0.75*tema.Rem, false)))
	}
	// Y 42 celdas con su círculo de `1.5rem` y su `p-1.5`, que es lo que hace que el
	// esqueleto tenga la GEOMETRÍA de la rejilla y no haya salto de layout al cargar.
	celdas := make([]fyne.CanvasObject, 0, 42)
	for range 42 {
		celdas = append(celdas, conMargenSimple(espacio1p5,
			agenda.Bloque(1.5*tema.Rem, 1.5*tema.Rem, false)))
	}
	return container.New(&arriba{Alto: AltoDiasSemana + tema.Borde},
		container.New(Cuadricula, cabecera...),
		container.New(ui.Rejilla{Fijas: 7}, celdas...))
}

// conMargenSimple envuelve un objeto con el mismo relleno por los cuatro lados, que es
// lo único que el esqueleto necesita: `ui.Margen`.
func conMargenSimple(m float32, hijo fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, hijo)
}

// AbrirNuevoEvento es la ruta `/calendar/events/new`: el enlace profundo de creación.
// En la web no cambia la pantalla —el volcado da los mismos 285 elementos— sino que
// abre el modal encima, y aquí hace lo mismo.
func (p *Pantalla) AbrirNuevoEvento() {
	if p.SinConexion {
		return
	}
	p.crearHoy()
}

// bajoCabecera es `flex flex-col` con la cabecera arriba —que mide lo que pidan sus
// filas al ancho que tenga— y la vista debajo, con lo que sobre.
type bajoCabecera struct{ c *CabeceraVista }

func (b *bajoCabecera) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return (&arriba{Alto: AltoCabeceraVista()}).MinSize(objs)
}

func (b *bajoCabecera) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	(&arriba{Alto: b.c.AltoPara(t.Width)}).Layout(objs, t)
}

// arriba es el layout de «una franja de alto fijo y el resto para el segundo hijo»:
// `flex flex-col` con un `flex-1 min-h-0` debajo. No usa `ui.Columna` porque allí cada
// hijo se queda su MinSize y aquí el segundo tiene que estirarse.
type arriba struct{ Alto float32 }

var _ fyne.Layout = (*arriba)(nil)

func (a *arriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := fyne.NewSize(0, a.Alto)
	if len(objs) > 1 {
		h := objs[1].MinSize()
		m = fyne.NewSize(h.Width, a.Alto+h.Height)
	}
	return m
}

func (a *arriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, a.Alto))
	if len(objs) > 1 {
		objs[1].Move(fyne.NewPos(0, a.Alto))
		objs[1].Resize(fyne.NewSize(t.Width, max(t.Height-a.Alto, 0)))
	}
}
