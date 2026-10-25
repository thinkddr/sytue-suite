// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// AgendaDias es `AGENDA_DAYS` de dates.ts: la ventana rodante de la vista agenda.
const AgendaDias = 30

// Rango es `rangeFor(mode, cursor)` de CalendarApp.tsx: lo que se le pide al servicio
// para la vista activa, [desde, hasta). Mes y semana van de medianoche a medianoche
// igual que sus rejillas; la agenda son los treinta días desde el del cursor.
func Rango(v Vista, cursor time.Time, loc *time.Location) (desde, hasta time.Time) {
	dia := inicioDelDia(cursor, loc)
	switch v {
	case VistaAgenda:
		return dia, dia.AddDate(0, 0, AgendaDias)
	case VistaSemana:
		lunes := dia.AddDate(0, 0, -((int(dia.Weekday()) + 6) % 7))
		return lunes, lunes.AddDate(0, 0, 7)
	case VistaDia:
		return dia, dia.AddDate(0, 0, 1)
	case VistaMes:
	}
	d := Dias(dia, loc)
	return d[0], d[len(d)-1].AddDate(0, 0, 1)
}

// Desplazar es `navigate(delta)`: un mes, una semana, un día o una ventana de agenda.
// `AddDate` desborda igual que `setMonth` de JS (31 de enero + 1 mes = 3 de marzo),
// así que las dos apps llegan al mismo día.
func Desplazar(v Vista, cursor time.Time, delta int) time.Time {
	switch v {
	case VistaAgenda:
		return cursor.AddDate(0, 0, delta*AgendaDias)
	case VistaSemana:
		return cursor.AddDate(0, 0, 7*delta)
	case VistaDia:
		return cursor.AddDate(0, 0, delta)
	case VistaMes:
	}
	return cursor.AddDate(0, delta, 0)
}

func inicioDelDia(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Fundir es `shownEvents` de la web: las ocurrencias de los calendarios VISIBLES, cada
// una con el color del evento si lo trae y si no el de su calendario, y de solo lectura
// cuando su calendario lo es o no hay red.
func Fundir(cals []api.Calendar, visibles map[string]bool, porCal map[string][]api.Instance, sinConexion bool) []Ocurrencia {
	var out []Ocurrencia
	for _, c := range cals {
		if visibles != nil && !visibles[c.ID] {
			continue
		}
		for _, inst := range porCal[c.ID] {
			tono := inst.Event.Color
			if tono == "" {
				tono = c.Color
			}
			out = append(out, Ocurrencia{
				Instance: inst, Color: colorDe(tono),
				CalendarioID: c.ID, SoloLectura: c.ReadOnly || sinConexion,
			})
		}
	}
	return out
}

// App es Sytue Calendar CONECTADO: la Pantalla, que no sabe de red, más quien le trae los
// datos del servicio y atiende sus avisos. Es `CalendarApp` de la web sin el pintado.
//
// Todo el estado se toca en el hilo de la interfaz: las cargas corren aparte y vuelven
// por `enUI`. Cada carga lleva su número de secuencia y una respuesta que llega después
// de otra más nueva se tira, que es el `calLoadSeq` de la web: sin él, un arranque en
// frío dejaba el nombre viejo de un calendario recién renombrado.
type App struct {
	// P es la pantalla. La App la reconstruye entera en cada cambio: 42 celdas y un
	// menú son baratos de rehacer, y así no hay dos verdades sobre qué se enseña.
	P *Pantalla
	// Cal es el cliente del servicio. nil: sin servicio (la pantalla se queda en carga).
	Cal *api.Calendars
	// Widget captura la sesión adoptada antes de cargar los calendarios completos.
	Widget       widgets.Source
	importarVida context.Context

	// Correo es el de quien ha entrado: decide si un evento es una invitación que
	// contestar (`meHanInvitado`). Vacío, nunca lo es.
	Correo string
	// Salas crea la videollamada de Sytue Rooms de un evento y devuelve su enlace. nil:
	// sin Rooms, el botón queda inerte.
	Salas func(ctx context.Context, uid, titulo string) (string, error)
	// Asistente es Sytue AI (`api.IA.Asistir`): el que propone un evento a partir de una
	// frase. nil: el formulario no lo ofrece.
	Asistente func(ctx context.Context, p api.PeticionIA) (string, error)
	// AbrirEnlace abre una URL en el sistema («Unirse» a la videollamada).
	AbrirEnlace func(url string)
	// Apex es el dominio de la suite, para los enlaces a su sitio público (la política
	// de privacidad). Vacío: el de producción.
	Apex string
	// Notas son las notas de Sytue Notepads que cuelgan de un evento, por su UID; y
	// AbrirNotas abre una ruta de Notepads (su app o, si no está, su web). nil: sin
	// Notepads, la vista rápida no enseña la sección.
	Notas      func(ctx context.Context, uid string) ([]api.Note, error)
	AbrirNotas func(ruta string)

	// lanzar y enUI son `go f()` y `fyne.Do(f)`. Las pruebas los cambian por llamadas
	// directas para no depender de gorutinas.
	lanzar func(func())
	enUI   func(func())

	raiz   *fyne.Container
	avisos *ui.PilaAvisos
	vista  *ui.Emergente // la vista rápida abierta, si la hay

	// El centro de recordatorios: su panel abierto, lo de dentro, y el estado del
	// sondeo. Los descartados se quitan YA, sin esperar a la ronda siguiente.
	recPanel       *ui.Emergente
	recContenido   *fyne.Container
	recCargando    bool
	recError       bool
	recDescartados map[string]bool

	conocidos     map[string]bool           // `knownUrls`: los calendarios ya vistos
	porCal        map[string][]api.Instance // la caché de eventos, válida para `clave`
	clave         string                    // el rango de la caché
	secCal, secEv int                       // secuencias de carga
	cargandoCal   bool                      // `loadingCals`
	cargandoEv    bool                      // `loadingEvents`
	fallo         string                    // clave del error, o ""
	prohibido     bool                      // el error fue un 403
}

// NuevaApp conecta la pantalla con el servicio. Arranca en carga: hasta que no
// contesta el servicio no hay calendarios que enseñar.
func NuevaApp(p *Pantalla, cal *api.Calendars) *App {
	a := &App{
		P: p, Cal: cal,
		lanzar:         func(f func()) { go f() },
		enUI:           fyne.Do,
		conocidos:      map[string]bool{},
		porCal:         map[string][]api.Instance{},
		cargandoCal:    true,
		avisos:         ui.NuevaPilaAvisos(),
		recCargando:    true,
		recDescartados: map[string]bool{},
	}
	p.AlIrHoy = func() { a.cambiar(func() { p.Cursor = time.Now() }) }
	p.AlNavegar = func(d int) { a.cambiar(func() { p.Cursor = Desplazar(p.Vista, p.Cursor, d) }) }
	p.AlDesplazarMes = func(d int) { a.cambiar(func() { p.Cursor = p.Cursor.AddDate(0, d, 0) }) }
	p.AlElegirDia = func(d time.Time) { a.cambiar(func() { p.Cursor = d }) }
	p.AlCambiarVista = func(v Vista) { a.cambiar(func() { p.Vista = v }) }
	p.AlConmutarCalendario = a.conmutar
	p.AlReintentar = a.reintentar
	p.Refresco.Actualizar = a.Recargar
	p.Refresco.Puede = func() bool {
		return a.Cal != nil && !p.SinConexion && !a.cargandoCal && !a.cargandoEv && p.Vista == VistaAgenda
	}
	p.Refresco.Revision = func() uint64 { return ui.RevisionLista(a.secEv) }
	p.AlAbrirEvento = a.abrirVistaRapida
	p.AlCrearEvento = a.crearEn
	p.AlCrearRango = a.crearRango
	p.AlMoverEvento = a.mover
	p.AlMenuCalendario = a.menuCalendario
	p.AlCrearCalendario = func() { a.formularioCalendario(nil) }
	p.AlSuscribirse = a.formularioSuscripcion
	p.AlGestionarRecursos = a.gestionarRecursos
	p.AlVerInvitaciones = a.abrirInvitaciones
	p.AlBuscar = a.abrirBusqueda
	p.AlVerRecordatorios = a.abrirRecordatorios
	p.AlConmutarCajon = func() { p.cajonAbierto = !p.cajonAbierto; a.pintar() }
	// En el teléfono, lo que se activa en el menú lo CIERRA (el `onClick` del `<aside>`
	// de AppShell): elegir un día, cambiar de mes, crear… Conmutar un calendario no,
	// que es una casilla y no un botón, y el kebab tampoco (`data-mantiene-nav`).
	cierra := func(f func()) func() {
		return func() { p.cajonAbierto = false; f() }
	}
	p.AlElegirDia = func(d time.Time) { p.cajonAbierto = false; a.cambiar(func() { p.Cursor = d }) }
	p.AlDesplazarMes = func(d int) { p.cajonAbierto = false; a.cambiar(func() { p.Cursor = p.Cursor.AddDate(0, d, 0) }) }
	p.AlCrearCalendario = cierra(p.AlCrearCalendario)
	p.AlSuscribirse = cierra(p.AlSuscribirse)
	a.raiz = container.NewStack()
	a.pintar()
	return a
}

// PonerGestorRecursos dice si quien ha entrado es `admin` u `owner`: solo entonces sale
// el botón de salas y recursos del menú.
func (a *App) PonerGestorRecursos(si bool) {
	if a.P.PuedeGestionarRecursos != si {
		a.P.PuedeGestionarRecursos = si
		a.pintar()
	}
}

// Objeto es lo que la ventana enseña.
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Arrancar pide los calendarios y el número de invitaciones pendientes.
func (a *App) Arrancar() {
	if d := a.P.Importar; d != nil && a.importarVida != nil && a.importarVida.Err() == nil {
		d.Cargar()
	}
	a.cargarCalendarios()
	a.contarInvitaciones()
}

// IniciarImportador liga sólo el importador a la identidad que adoptó el binario.
// Las operaciones de eventos y los formularios conservan su ciclo actual.
func (a *App) IniciarImportador(ctx context.Context) {
	a.importarVida = ctx
	if d := a.P.Importar; d != nil {
		d.CargarCon(ctx)
	}
}

// PararImportador retira su contenido y evita que Arrancar reactive una vida cerrada.
func (a *App) PararImportador() {
	a.importarVida = nil
	if d := a.P.Importar; d != nil {
		d.Parar()
	}
}

// Atajo son las teclas de CalendarApp.tsx, las mismas que Apple Calendar y Google
// Calendar. La ventana solo las entrega cuando nada tiene el foco —un campo o un diálogo
// se quedan su tecla—, que es lo que hace `esAtajoDeApp` en la web.
func (a *App) Atajo(r rune) bool {
	p := a.P
	switch r {
	case 't', 'T':
		p.AlIrHoy()
	case 'm', 'M':
		p.AlCambiarVista(VistaMes)
	case 'w', 'W':
		p.AlCambiarVista(VistaSemana)
	case 'd', 'D':
		p.AlCambiarVista(VistaDia)
	case 'a', 'A':
		p.AlCambiarVista(VistaAgenda)
	case 'n', 'N', 'j', 'J':
		p.AlNavegar(1)
	case 'p', 'P', 'k', 'K':
		p.AlNavegar(-1)
	case 'c', 'C':
		p.AbrirNuevoEvento()
	case '/':
		if p.AlBuscar == nil {
			return false
		}
		p.AlBuscar()
	default:
		return false
	}
	return true
}

// cambiar aplica un cambio de cursor o de vista y pide lo que falte del rango nuevo.
func (a *App) cambiar(f func()) {
	f()
	a.cargarEventos()
}

// conmutar es `toggleVisible`: un calendario ya cargado se enciende sin ir a la red.
func (a *App) conmutar(id string) {
	p := a.P
	if p.Visibles == nil {
		p.Visibles = map[string]bool{}
	}
	p.Visibles[id] = !p.Visibles[id]
	a.cargarEventos()
}

// reintentar es el botón del estado de error: sin calendarios, los calendarios; con
// ellos, los eventos.
func (a *App) reintentar() {
	if len(a.P.Calendarios) == 0 {
		a.cargarCalendarios()
		return
	}
	a.Recargar()
}

// Recargar invalida la caché y vuelve a pedir los eventos: es `reload()` de la web,
// lo que se hace tras guardar o borrar.
func (a *App) Recargar() {
	a.porCal, a.clave = map[string][]api.Instance{}, ""
	a.cargarEventos()
}

// RecargarCalendarios vuelve a pedir la lista, tras crear, renombrar o borrar uno.
func (a *App) RecargarCalendarios() { a.cargarCalendarios() }

func (a *App) todos() []api.Calendar {
	return append(append([]api.Calendar(nil), a.P.Calendarios...), a.P.Compartidos...)
}

// cargarCalendarios es `loadCalendars`: propios y compartidos, y que fallen los
// compartidos no tumba la vista (se quedan en lista vacía).
//
// Las dos lecturas independientes arrancan juntas, como en la web. Cada una conserva
// su error; no se comparte un primer fallo que cancele el resultado de la otra.
func (a *App) cargarCalendarios() {
	if a.Cal == nil {
		return
	}
	a.secCal++
	sec := a.secCal
	a.cargandoCal = true
	a.pintar()
	widget := a.Widget.View()
	sinConexion := a.P.SinConexion
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), 20*time.Second)
		defer parar()
		var compartidos []api.Calendar
		var errC error
		var lecturas sync.WaitGroup
		lecturas.Go(func() { compartidos, errC = a.Cal.SharedWithMe(ctx) })
		propios, err := a.Cal.List(ctx)
		lecturas.Wait()
		if err == nil && errC == nil && !sinConexion {
			// Las ocurrencias sólo cubren la ventana visible; estas dos listas sí
			// contienen todos los calendarios accesibles, incluidos los compartidos.
			ids := make(map[string]struct{}, len(propios)+len(compartidos))
			for _, lista := range [][]api.Calendar{propios, compartidos} {
				for _, c := range lista {
					ids[c.ID] = struct{}{}
				}
			}
			_ = widget.Publicar(ctx, widgets.Counter{Kind: "calendars", Value: int64(len(ids))})
		}
		if errC != nil {
			compartidos = nil
		}
		a.enUI(func() {
			if sec != a.secCal {
				return // llegó otra carga después: esta es rancia
			}
			a.cargandoCal = false
			if err != nil {
				a.fallar("cal.error.loadCalendars", err)
				return
			}
			a.P.Calendarios, a.P.Compartidos = propios, compartidos
			a.encender()
			a.engancharPeticiones()
			a.fallo = ""
			a.cargarEventos()
		})
	})
}

// encender es la regla de visibilidad de la web: en la primera carga, todos; después se
// respeta lo que la persona apagó y se encienden solo los recién llegados.
func (a *App) encender() {
	p := a.P
	primera := len(a.conocidos) == 0
	antes := p.Visibles
	ahora := map[string]bool{}
	conocidos := map[string]bool{}
	for _, c := range a.todos() {
		if primera || antes[c.ID] || !a.conocidos[c.ID] {
			ahora[c.ID] = true
		}
		conocidos[c.ID] = true
	}
	p.Visibles, a.conocidos = ahora, conocidos
}

// cargarEventos es el efecto de eventos de la web: un `Instances` por calendario
// visible que no esté ya en la caché del rango, todos en paralelo.
func (a *App) cargarEventos() {
	p := a.P
	desde, hasta := Rango(p.Vista, p.Cursor, p.zona())
	clave := desde.Format(time.RFC3339) + ".." + hasta.Format(time.RFC3339)
	if clave != a.clave {
		a.porCal, a.clave = map[string][]api.Instance{}, clave
	}
	var faltan []string
	for _, c := range a.todos() {
		if p.visible(c.ID) {
			if _, ok := a.porCal[c.ID]; !ok {
				faltan = append(faltan, c.ID)
			}
		}
	}
	a.secEv++
	sec := a.secEv
	if len(faltan) == 0 || a.Cal == nil {
		a.cargandoEv = false
		a.pintar()
		return
	}
	a.cargandoEv = true
	a.pintar()
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), 20*time.Second)
		defer parar()
		type par struct {
			id   string
			inst []api.Instance
			err  error
		}
		pares := make(chan par, len(faltan))
		for _, id := range faltan {
			go func() {
				inst, err := a.Cal.Instances(ctx, id, desde, hasta)
				pares <- par{id, inst, err}
			}()
		}
		llegados := make([]par, 0, len(faltan))
		for range faltan {
			llegados = append(llegados, <-pares)
		}
		a.enUI(func() {
			if sec != a.secEv {
				return
			}
			a.cargandoEv = false
			// Un 403/404 de UN calendario —le han retirado la compartición, o lo
			// borraron en otro sitio— no es un fallo de la pantalla: ese se quita y se
			// avisa. Tumbarla entera por él la dejaba en «sin permiso» con todos los
			// demás calendarios a la vista del servicio.
			// Lo que sí llegó se guarda aunque otro falle: es válido, y «Reintentar»
			// pide entonces solo lo que falta.
			fuera := map[string]bool{}
			for _, r := range llegados {
				switch {
				case sinAcceso(r.err):
					fuera[r.id] = true
				case r.err != nil:
					a.fallar("cal.error.loadEvents", r.err)
					return
				default:
					a.porCal[r.id] = r.inst
				}
			}
			a.quitar(fuera)
			// Llegó lo guardado: el fantasma del bloque soltado ya no hace falta.
			a.P.pendiente = nil
			a.fallo = ""
			a.pintar()
		})
	})
}

// sinAcceso: el servicio dice que ese calendario ya no es para esta persona (403) o
// que ya no existe (404).
func sinAcceso(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && (e.Status == 403 || e.Status == 404)
}

// quitar saca de la vista los calendarios a los que ya no hay acceso y lo avisa, uno a
// uno y con su nombre. Si siguieran en la lista, cada cambio de rango los volvería a
// pedir para recibir el mismo 403.
func (a *App) quitar(fuera map[string]bool) {
	if len(fuera) == 0 {
		return
	}
	p := a.P
	quedan := func(cs []api.Calendar) []api.Calendar {
		var r []api.Calendar
		for _, c := range cs {
			if !fuera[c.ID] {
				r = append(r, c)
				continue
			}
			delete(p.Visibles, c.ID)
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, p.t("cal.error.noAccess", i18n.Vars{"name": c.DisplayName}), ""))
		}
		return r
	}
	p.Calendarios, p.Compartidos = quedan(p.Calendarios), quedan(p.Compartidos)
}

// fallar deja la pantalla en su estado de error con el mensaje que toca.
func (a *App) fallar(clave string, err error) {
	var e *api.Error
	a.fallo, a.prohibido = clave, errors.As(err, &e) && e.Status == 403
	a.pintar()
}

// contarInvitaciones pone el número de la insignia de «Invitaciones». Falla en
// silencio, como en la web: sin número la cabecera sigue sirviendo.
func (a *App) contarInvitaciones() {
	if a.Cal == nil {
		return
	}
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), 20*time.Second)
		defer parar()
		inv, err := a.Cal.Invitations(ctx, true)
		if err != nil {
			return
		}
		a.enUI(func() {
			a.P.Invitaciones = len(inv)
			a.pintar()
		})
	})
}

// pintar deriva el estado de la pantalla de las cargas y la reconstruye. Es la
// condición de la web, literal: esqueleto mientras cargan los calendarios o mientras
// cargan eventos sin ninguno que enseñar; error si lo hay; y si no, la rejilla,
// atenuada mientras llega lo que falta.
func (a *App) pintar() {
	p := a.P
	p.Eventos = Fundir(a.todos(), p.Visibles, a.porCal, p.SinConexion)
	ordenar(p.Eventos)
	p.CargandoCalendarios = a.cargandoCal
	p.Recargando = false
	p.Fallo = ""
	switch {
	case a.cargandoCal || (a.cargandoEv && len(p.Eventos) == 0):
		p.Estado = agenda.Cargando
	case a.fallo != "" && a.prohibido:
		p.Estado, p.Fallo = agenda.SinPermiso, a.fallo
	case a.fallo != "":
		p.Estado, p.Fallo = agenda.Error, a.fallo
	default:
		p.Estado, p.Recargando = agenda.Exito, a.cargandoEv
	}
	if a.raiz == nil {
		return
	}
	a.raiz.Objects = []fyne.CanvasObject{p.Construir(), a.capaFlotante(), a.avisos}
	a.raiz.Refresh()
}

// ordenar deja las ocurrencias por inicio, ESTABLE: a igualdad manda el orden de los
// calendarios y el del servicio, que es lo que hace el `sort` de la web por día.
func ordenar(o []Ocurrencia) {
	sort.SliceStable(o, func(i, j int) bool { return o[i].Start.Before(o[j].Start) })
}

// ConImportar engancha «Trae tus calendarios» (`ImportData domains={['calendar']}`, la
// pieza común `ui.ImportarDatos`) al pie del menú. Al acabar una importación se recargan
// los calendarios, que es el `onFinished={loadCalendars}` de la web.
func (a *App) ConImportar(i *api.Integraciones) {
	d := ui.NuevoImportarDatos(a.P.T, i, "calendar")
	d.AlCambiar = a.pintar
	d.AlTerminar = a.RecargarCalendarios
	d.AlPolitica = a.abrirPolitica
	a.P.Importar = d
}

// ConServiciosVinculados muestra las cuentas del proveedor junto a los calendarios.
func (a *App) ConServiciosVinculados(abrir func()) {
	a.P.AlServiciosVinculados = nil
	if abrir != nil {
		a.P.AlServiciosVinculados = func() { a.P.cajonAbierto = false; a.pintar(); abrir() }
	}
	a.pintar()
}
