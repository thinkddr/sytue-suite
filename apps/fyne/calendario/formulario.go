// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El formulario de un evento: EventModal.tsx, en un `Modal size="md"`. Los campos, en su
// orden: título, calendario, «todo el día» en su caja, inicio y fin (en dos columnas de
// `sm` para arriba), zona horaria, repetir y cuándo termina, ubicación, videollamada,
// color, invitados y descripción. Pie: «Eliminar» a la izquierda al editar, y
// «Cancelar» y «Guardar» a la derecha. Un evento de solo lectura se abre en consulta:
// todo inerte y un único «Cerrar».
//

// Edicion es `EditingEvent`: lo que el formulario enseña al abrirse.
type Edicion struct {
	// O es la ocurrencia que se edita; nil al crear o al duplicar.
	O            *Ocurrencia
	CalendarioID string
	Titulo       string
	// Inicio y Fin son lo que dicen los campos, que se escriben y se leen en la zona de
	// visualización: su hora de pared en ella es la de PARED del evento en `Zona`, que es
	// en la que la lee EntradaDe (EdicionDe la pasa de una zona a otra).
	Inicio, Fin time.Time
	TodoElDia   bool
	// Zona es el TZID del evento; vacío, la de visualización.
	Zona                                    string
	Lugar, Descripcion, Color, Videollamada string
	Regla                                   *Recurrencia
	// ReglaCruda es la RRULE que el formulario no sabe editar (BYMONTH, 2MO…), tal
	// cual llegó: «Repetir» dice «Personalizada» y al guardar viaja intacta. Vacía si
	// no la hay o si el usuario eligió otra repetición. Excluye a Regla.
	ReglaCruda  string
	Invitados   []string
	SoloLectura bool
	// UID es el del evento que se edita, o el que se generó en esta sesión del
	// formulario para ligar la videollamada al evento que se va a guardar.
	UID string
	// Recordatorios son los minutos antes del inicio. nil es «no se sabe»: se están
	// leyendo del servicio o no se pudieron leer, y al guardar no se tocan.
	Recordatorios *[]int
	// Recursos son las salas y los equipos que se reservan al guardar, por su id.
	Recursos []string
	// Libre es «Mostrar como: disponible» (TRANSP:TRANSPARENT): no ocupa la hora en el
	// free/busy ni en «buscar hueco».
	Libre bool
	// desde es el evento del que se leen los recordatorios al duplicar.
	desde string
}

// origen es el id del evento del que salen los recordatorios: el que se edita o el que
// se duplica.
func (e Edicion) origen() string {
	if e.O != nil {
		return e.O.Event.ID
	}
	return e.desde
}

// PaletaCalendario es `CALENDAR_PALETTE`: la paleta de colores de usuario de la suite
// (`tokens.PaletaDeUsuario`), la misma de las etiquetas de Mail y los cuadernos de Notes,
// con el nombre de cada tono del catálogo común. En hex porque el servicio guarda el
// color tal cual (#RRGGBB).
var PaletaCalendario = func() []struct{ Clave, Hex string } {
	out := make([]struct{ Clave, Hex string }, 0, len(tokens.PaletaDeUsuario))
	for _, c := range tokens.PaletaDeUsuario {
		out = append(out, struct{ Clave, Hex string }{c.Clave(), c.Valor})
	}
	return out
}()

// El tono de un calendario nuevo y el de una suscripción: `COLOR_NUEVO` y
// `COLOR_SUSCRIPCION` de CalendarModal.tsx.
var (
	ColorNuevo       = tonoPorID("blue")
	ColorSuscripcion = tonoPorID("plum")
)

func tonoPorID(id string) string {
	c, _ := tokens.TonoDeUsuario(id)
	return c.Valor
}

// Los valores de «Repetir» y de «Termina», en el orden de sus `<option>`. «WEEKDAYS» es
// «cada día laborable», que se guarda como semanal de lunes a viernes.
var (
	frecuencias = []string{"", "DAILY", "WEEKLY", "MONTHLY", "YEARLY", "WEEKDAYS"}
	finesSerie  = []string{"never", "count", "until"}
)

// correoValido es `isEmail`: la validación mínima; la de verdad la hace el servicio.
var correoValido = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// AccionesFormulario son lo que el formulario pide a la app, que es quien habla con el
// servicio. `listo` se llama al terminar, con `ok` si hay que cerrar.
type AccionesFormulario struct {
	Guardar      func(e Edicion, listo func(ok bool))
	Eliminar     func(e Edicion, listo func(ok bool))
	Videollamada func(uid, titulo string, listo func(enlace string, ok bool))
	// Huecos es «buscar hueco»: los tramos libres en [desde, hasta) donde caben
	// `minutos`, en la franja laboral de la zona y, si `laborables`, de lunes a viernes.
	// Nil, el formulario no ofrece buscar.
	Huecos func(desde, hasta time.Time, minutos int, zona string, laborables bool, listo func([]api.Hueco, error))
	// Asistente pide al servicio de IA la propuesta de un evento y devuelve su texto.
	// Nil, el formulario no lo ofrece.
	Asistente func(p api.PeticionIA, listo func(texto string, err error))
	// Aviso pinta un aviso de error de la app («Selecciona un calendario»).
	Aviso func(clave string)
	// Recordatorios lee del servicio los del evento que se edita o se duplica. Nil: no
	// hay de dónde leerlos.
	Recordatorios func(listo func([]int, error))
	// Recursos lista las salas y los recursos con su disponibilidad en [desde, hasta).
	// Nil: el formulario no ofrece reservar.
	Recursos func(desde, hasta time.Time, listo func([]api.Resource, error))
	// Sugerir es el autocompletado del campo de invitados (Pantalla.Invitar). Nil, sin él.
	Sugerir func(q string, listo func([]SugerenciaInvitado))
	// Reunion crea ya la reunión de un calendario (Pantalla.Reunion.Crear) y da su enlace.
	// Nil, sin ella.
	Reunion func(calendario, titulo string, inicio, fin time.Time, listo func(enlace string, err error))
	// Ocupacion es la de los invitados en «buscar un hueco» (Pantalla.Invitar). Nil, solo
	// la propia.
	Ocupacion func(calendario string, correos []string, desde, hasta time.Time, listo func(map[string][]api.Hueco, error))
	// Choques son los eventos ya cargados que se pisan con [inicio, fin), sin contar el
	// de `uid` («Coincide con…»). Nil: no se avisa.
	Choques func(inicio, fin time.Time, uid string) []Solape
}

// formulario es el estado vivo del modal.
type formulario struct {
	p           *Pantalla
	e           Edicion
	destinos    []api.Calendar
	a           AccionesFormulario
	d           *ui.Dialogo
	cuerpo      *fyne.Container
	ocupado     bool
	nuevo       bool
	errorVideo  bool
	creandoSala bool

	titulo, inicio, fin, lugar, correo, cuenta, hasta *ui.Entrada
	// enlace es el campo libre del enlace de la reunión (Pantalla.PistaEnlace); nil, la
	// sala de Rooms.
	enlace                                       *ui.Entrada
	descripcion                                  *ui.AreaTexto
	calendario, zona, repetir, finSerie, mostrar *ui.Selector
	// aviso es «Coincide con…» del último cálculo; vacío, ningún choque.
	aviso string
	// dias son los días elegidos de una semanal (`repeatDays`); nil, el del inicio.
	dias []string
	// reglaCruda es la RRULE personalizada con la que se abrió el formulario, para
	// devolverla si se vuelve a elegir «Personalizada» tras probar otra opción.
	reglaCruda string
	todoElDia  *ui.Conmutador
	// Los errores que van junto a su campo; vacíos, sin error.
	errTitulo, errFin, errCorreo string
	guardar, eliminar, cancelar  *ui.Boton
	zonas                        []string
	// bh es «buscar hueco»; nil si la app no sabe buscar.
	bh *buscaHueco
	// El asistente: su campo, «Rellenar» y si la última propuesta no se entendió.
	ia      *ui.Entrada
	iaBoton *ui.Boton
	iaError bool
	// Los recordatorios: los controles de cada fila y «Añadir recordatorio».
	filasRec  []*controlesRecordatorio
	anadirRec *ui.Boton
	// rec es «Salas y recursos»; nil si la App no sabe listarlos.
	rec *selectorRecursos
	// inicioCaja y finCaja son los campos con el icono que despliega el selector de
	// fecha; iconosFecha, esos iconos (se apagan en consulta).
	inicioCaja, finCaja fyne.CanvasObject
	iconosFecha         []*ui.BotonIcono
	// Sin personas con Pantalla.Invitar (invitarlocal.go): las direcciones de la libreta
	// bajo el campo, la señalada y el turno que descarta las respuestas viejas; y los
	// invitados con que se abrió, que vuelven si se guarda donde no se puede invitar.
	sugerencias         []SugerenciaInvitado
	sugActiva, turnoSug int
	invitadosIniciales  []string
	// Con Pantalla.Reunion (reunionlocal.go): la reunión pedida al proveedor del
	// calendario para cuando se guarde, la que se está creando y si falló.
	reunionPedida, creandoReunion, errorReunion bool
}

// AbrirFormulario abre el modal sobre el lienzo. `destinos` son los calendarios donde se
// puede escribir: los propios y los compartidos con permiso de edición.
func (p *Pantalla) AbrirFormulario(c fyne.Canvas, e Edicion, destinos []api.Calendar, a AccionesFormulario) *ui.Dialogo {
	f := p.nuevoFormulario(e, destinos, a)
	f.d.Mostrar(c)
	// Armado sin lienzo, el formulario se reparte como en el escritorio; ya puesto, se
	// rehace con el ancho de verdad (la hoja del teléfono: los recordatorios en dos
	// líneas, el panel de huecos a 342).
	f.rehacer()
	return f.d
}

// nuevoFormulario arma el formulario sin mostrarlo.
func (p *Pantalla) nuevoFormulario(e Edicion, destinos []api.Calendar, a AccionesFormulario) *formulario {
	f := &formulario{p: p, e: e, destinos: destinos, a: a, nuevo: e.O == nil, invitadosIniciales: slices.Clone(e.Invitados)}
	if f.e.CalendarioID == "" {
		f.e.CalendarioID = calendarioPorDefecto(destinos)
	}
	if f.e.Zona == "" {
		f.e.Zona = p.nombreZona()
	}
	f.construir()
	return f
}

// calendarioPorDefecto es `defaultUrl`: el marcado por defecto o, si no, el primero.
func calendarioPorDefecto(cals []api.Calendar) string {
	for _, c := range cals {
		if c.IsDefault {
			return c.ID
		}
	}
	if len(cals) > 0 {
		return cals[0].ID
	}
	return ""
}

//nolint:gocyclo // El formulario del evento se monta de una vez, campo a campo, con lo que ofrece cada calendario.
func (f *formulario) construir() {
	p, e := f.p, f.e
	f.titulo = ui.NuevaEntrada(p.t("event.titlePlaceholder", nil))
	f.titulo.PonerTexto(e.Titulo)
	f.titulo.AlCambiar(func(string) {
		f.limpiar(&f.errTitulo, f.titulo)
	})

	nombres := make([]string, len(f.destinos))
	elegido := 0
	for i, c := range f.destinos {
		nombres[i] = c.Title()
		if c.ID == e.CalendarioID {
			elegido = i
		}
	}
	f.calendario = ui.NuevoSelector(nombres, func(i int) {
		f.e.CalendarioID = f.destinos[i].ID
		if p.reunionLocal() != nil {
			// El botón de la reunión es el del proveedor del calendario elegido.
			f.alCambiarCalendarioReunion()
			if p.invitarLocal() == nil {
				f.rehacer()
			}
		}
		if p.invitarLocal() != nil {
			// Los invitados salen o no según el calendario, y su ocupación es la de su cuenta.
			f.sugerencias = nil
			f.rehacer()
			f.pedirHuecos()
		}
	})
	f.calendario.Elegida = elegido

	f.todoElDia = ui.NuevoConmutador(p.t("event.allDay", nil), func(si bool) {
		f.leerFechas()
		f.e.TodoElDia = si
		f.escribirFechas()
		f.aviso = f.textoChoques()
		f.rehacer()
	})
	f.todoElDia.Encendido = e.TodoElDia

	f.inicio, f.fin = campoFechaHora(), campoFechaHora()
	f.inicio.AlCambiar(func(s string) { f.cambiarInicio(s) })
	f.fin.AlCambiar(f.cambiarFin)
	f.escribirFechas()
	f.inicioCaja = f.conSelectorFecha(f.inicio, p.t("event.start", nil), func() time.Time { return f.e.Inicio })
	f.finCaja = f.conSelectorFecha(f.fin, p.t("event.end", nil), func() time.Time { return f.e.Fin })

	f.construirZona()
	f.construirMostrar()
	// En consulta no hay nada que rellenar: el asistente apagado arriba de una ficha
	// que no se puede editar solo estorba (como «buscar un hueco»).
	if f.a.Asistente != nil && !e.SoloLectura {
		f.construirAsistente()
	}
	if a := f.a; a.Huecos != nil && !e.SoloLectura {
		f.construirHuecos()
	}

	f.construirRepeticion()

	f.lugar = ui.NuevaEntrada(p.t("event.locationPlaceholder", nil))
	f.lugar.PonerTexto(e.Lugar)
	f.lugar.AlCambiar(func(s string) { f.e.Lugar = s })

	f.construirInvitados()
	if p.PistaEnlace != "" {
		f.enlace = ui.NuevaEntrada("https://")
		f.enlace.PonerTexto(e.Videollamada)
		f.enlace.AlCambiar(f.alCambiarEnlace)
	}

	f.descripcion = ui.NuevaAreaTexto(p.t("event.descriptionPlaceholder", nil))
	f.descripcion.Filas = 4
	f.descripcion.PonerTexto(e.Descripcion)
	f.descripcion.AlCambiar(func(s string) { f.e.Descripcion = s })

	f.anadirRec = ui.NuevoBoton(p.t("event.reminderAdd", nil), f.anadirRecordatorio).
		Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Mas, iconos.TallaSM)
	if f.e.Recordatorios == nil && f.nuevo && f.a.Recordatorios == nil {
		// Un evento nuevo empieza con el recordatorio de los ajustes, o sin ninguno.
		l := []int{}
		if r := p.ajustes().Recordatorio; r != nil {
			l = append(l, *r)
		}
		f.e.Recordatorios = &l
	}
	f.construirRecordatorios()
	f.construirRecursos()

	f.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(4)})
	f.rehacerFilas()

	titulo := "event.editTitle"
	switch {
	case e.SoloLectura:
		titulo = "event.viewTitle"
	case f.nuevo:
		titulo = "event.newTitle"
	}
	f.d = ui.NuevoDialogo(p.t(titulo, nil), f.cuerpo).Con(ui.DialogoMD, f.pie())
	f.d.RotuloCerrar = p.t("common.close", nil)
	if e.SoloLectura {
		// El título apagado no se enfoca (en la web, el `fieldset disabled`): el foco va
		// a lo primero que se puede usar, el aspa. Se quedaba con el anillo puesto.
		f.inertes()
	} else {
		f.d.Foco = f.titulo
	}
	if f.e.Recordatorios == nil && f.a.Recordatorios != nil {
		f.a.Recordatorios(func(l []int, err error) {
			if err != nil || f.e.Recordatorios != nil {
				return
			}
			f.e.Recordatorios = &l
			f.construirRecordatorios()
			f.rehacer()
		})
	}
	f.pedirRecursos()
}

// construirZona es el selector de «Zona horaria», con el desfase de cada zona ahora.
func (f *formulario) construirZona() {
	e := f.e
	f.zonas = i18n.ZonasHorarias()
	etiquetas := make([]string, len(f.zonas))
	ahora, iz := time.Now(), 0
	for i, z := range f.zonas {
		etiquetas[i] = i18n.EtiquetaZona(z, ahora)
		if z == e.Zona {
			iz = i
		}
	}
	f.zona = ui.NuevoSelector(etiquetas, func(i int) { f.e.Zona = f.zonas[i] })
	f.zona.Elegida = iz
}

// construirRepeticion son «Repetir», «Termina» y el número de veces o la fecha.
func (f *formulario) construirRepeticion() {
	p, e := f.p, f.e
	opciones := []string{
		p.t("event.repeatNever", nil), p.t("event.repeatDaily", nil),
		p.t("event.repeatWeekly", nil), p.t("event.repeatMonthly", nil),
		p.t("event.repeatYearly", nil), p.t("event.repeatWeekdays", nil),
	}
	// Una regla que no se sabe editar sale como una opción más, «Personalizada», detrás
	// de las de siempre: elegirla la conserva y cualquier otra la sustituye, como en
	// EventModal (`event.repeatCustom`).
	f.reglaCruda = e.ReglaCruda
	if f.reglaCruda != "" {
		opciones = append(opciones, p.t("event.repeatCustom", nil))
	}
	f.repetir = ui.NuevoSelector(opciones, func(i int) {
		f.e.ReglaCruda = ""
		switch {
		case i >= len(frecuencias):
			f.e.Regla, f.e.ReglaCruda = nil, f.reglaCruda
		case frecuencias[i] == "":
			f.e.Regla = nil
		default:
			if f.e.Regla == nil {
				f.e.Regla = &Recurrencia{}
			}
			f.e.Regla.Freq, f.e.Regla.Dias = frecuencias[i], nil
			switch frecuencias[i] {
			case "WEEKDAYS":
				// Siempre cada semana: «cada 2 semanas, de lunes a viernes» no es esto.
				f.e.Regla.Freq, f.e.Regla.Dias, f.e.Regla.Intervalo = "WEEKLY", slices.Clone(Laborables), 0
			case "WEEKLY":
				f.e.Regla.Dias = f.diasRegla()
			}
		}
		f.rehacer()
	})
	f.finSerie = ui.NuevoSelector([]string{
		p.t("event.repeatEndNever", nil), p.t("event.repeatEndCount", nil), p.t("event.repeatEndUntil", nil),
	}, f.cambiarFinSerie)
	if r := e.Regla; r != nil {
		f.elegirRegla(*r)
	} else if f.reglaCruda != "" {
		f.repetir.Elegida = len(frecuencias)
	}
	f.cuenta = ui.NuevaEntrada("")
	f.cuenta.AlCambiar(func(s string) {
		// Solo con «Termina: tras N veces». Rellenar el campo al abrir (las 4 de
		// partida) también avisa, y sin esto una serie sin fin se guardaba con COUNT=4.
		if f.finSerie.Elegida != 1 {
			return
		}
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && f.e.Regla != nil {
			f.e.Regla.Cuenta = max(1, n)
		}
	})
	f.hasta = ui.CampoFecha(ui.CampoMD)
	f.hasta.AlCambiar(func(s string) {
		if t, vacia, ok := ui.LeerFecha(s, time.UTC); ok && !vacia && f.e.Regla != nil {
			f.e.Regla.Hasta = t.Format(time.DateOnly)
		}
	})
	if r := e.Regla; r != nil {
		// Las veces de la serie, o 4 de partida si no tiene (`repeatCount`). Era
		// max(Cuenta, 4) y una serie de 2 se reabría, y se guardaba, con 4.
		cuenta := r.Cuenta
		if cuenta <= 0 {
			cuenta = 4
		}
		f.cuenta.PonerTexto(strconv.Itoa(cuenta))
		if t, err := time.Parse(time.DateOnly, r.Hasta); err == nil {
			f.hasta.PonerTexto(ui.EscribirFecha(t))
		}
	}
}

// elegirRegla deja «Repetir», los días y «Termina» como los dice la regla con la que se
// abre el formulario: lunes a viernes vuelve como «cada día laborable».
func (f *formulario) elegirRegla(r Recurrencia) {
	f.repetir.Elegida = max(0, slices.Index(frecuencias, r.Freq))
	if r.EsLaborable() {
		f.repetir.Elegida = slices.Index(frecuencias, "WEEKDAYS")
	} else if r.Freq == "WEEKLY" && len(r.Dias) > 0 {
		f.dias = slices.Clone(r.Dias)
	}
	switch {
	case r.Cuenta > 0:
		f.finSerie.Elegida = 1
	case r.Hasta != "":
		f.finSerie.Elegida = 2
	}
}

// diasRegla es el BYDAY de la semanal con los días elegidos: nil si no se tocaron o si
// solo está el del inicio, que es la semanal de siempre y sigue al evento si cambia
// de día (`buildRecurrence`).
func (f *formulario) diasRegla() []string {
	dias := normalizarDias(f.dias)
	if len(dias) == 0 || (len(dias) == 1 && dias[0] == diaDe(f.e.Inicio)) {
		return nil
	}
	return dias
}

// diasElegidos son los días encendidos en el selector: los elegidos o el del inicio.
func (f *formulario) diasElegidos() []string {
	if len(f.dias) == 0 {
		return []string{diaDe(f.e.Inicio)}
	}
	return normalizarDias(f.dias)
}

// semanal dice si «Repetir» está en «Semanalmente» (no en «cada día laborable»).
func (f *formulario) semanal() bool {
	i := f.repetir.Elegida
	return f.e.Regla != nil && i >= 0 && i < len(frecuencias) && frecuencias[i] == "WEEKLY"
}

// bloqueDias es `WeekdayPicker`: «Se repite el» y las siete iniciales como botones que
// se encienden (primario) y se apagan (fantasma). El lector de pantalla oye el nombre
// entero; el último día encendido no se apaga.
func (f *formulario) bloqueDias() fyne.CanvasObject {
	l := f.p.locale()
	iniciales, nombres := i18n.WeekdaysNarrow(l), i18n.WeekdaysLong(l)
	elegidos := f.diasElegidos()
	botones := make([]fyne.CanvasObject, 0, len(DiasSemana))
	for i, d := range DiasSemana {
		encendido := slices.Contains(elegidos, d)
		b := ui.NuevoBoton(iniciales[i], func() {
			switch {
			case encendido && len(elegidos) == 1:
				return
			case encendido:
				f.dias = slices.DeleteFunc(slices.Clone(elegidos), func(x string) bool { return x == d })
			default:
				f.dias = normalizarDias(append(slices.Clone(elegidos), d))
			}
			if f.e.Regla != nil {
				f.e.Regla.Dias = f.diasRegla()
			}
			f.rehacer()
		})
		v := ui.Fantasma
		if encendido {
			v = ui.Primario
		}
		b.Con(v, ui.TallaSM)
		b.Ancho, b.Rotulo = tema.Espacio(8), nombres[i]
		botones = append(botones, b)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		alto(rotuloFuerte(f.p.t("event.repeatOn", nil)), tema.TextoSM.Linea),
		container.New(ui.Flujo{Hueco: tema.Espacio(1)}, botones...))
}

func (f *formulario) cambiarFinSerie(i int) {
	switch finesSerie[i] {
	case "count":
		f.e.Regla.Hasta = ""
		if f.e.Regla.Cuenta == 0 {
			f.e.Regla.Cuenta = 4
		}
	case "until":
		f.e.Regla.Cuenta = 0
		if f.e.Regla.Hasta == "" {
			f.e.Regla.Hasta = f.e.Inicio.Format(time.DateOnly)
		}
	default:
		f.e.Regla.Cuenta, f.e.Regla.Hasta = 0, ""
	}
	f.rehacer()
}

// construirInvitados es el campo del correo: Intro o salir del campo lo añaden.
func (f *formulario) construirInvitados() {
	p := f.p
	f.correo = ui.NuevaEntrada(p.t("event.guestPlaceholder", nil))
	f.correo.AlCambiar(func(s string) {
		f.limpiar(&f.errCorreo, f.correo)
		f.sugerir(s)
	})
	f.correo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if f.teclaSugerencias(k) {
			return true
		}
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			f.anadirInvitado(f.correo.Texto())
			return true
		}
		return false
	}
	f.correo.AlEnfocar = func(dentro bool) {
		if dentro {
			return
		}
		// Con la lista de la libreta abierta, salir del campo puede ser pulsar una de
		// sus direcciones: lo escrito a medias no es un error todavía.
		if len(f.sugerencias) > 0 && !correoValido.MatchString(strings.TrimSpace(f.correo.Texto())) {
			return
		}
		f.anadirInvitado(f.correo.Texto()) // `onBlur`
	}
}

// campoFechaHora es el `<Input type="datetime-local">`: la misma caja de 31,5. La fecha
// y la hora se teclean («21/09/2026, 09:00», que leen LeerFecha y LeerHora) o se eligen
// en el desplegable del icono de la derecha (selectorfecha.go).
func campoFechaHora() *ui.Entrada { return ui.NuevaEntrada("31/12/2026, 09:30") }

// escribirFechas vuelca inicio y fin en los campos: fecha y hora, o solo la fecha en un
// evento de todo el día (`type="date"`).
func (f *formulario) escribirFechas() {
	f.inicio.PonerTexto(f.textoFecha(f.e.Inicio))
	f.fin.PonerTexto(f.textoFecha(f.e.Fin))
}

// leerFechaHora lee «21/09/2026, 09:00» (o solo la fecha) como hora de pared en la
// zona de visualización. ok=false si no se entiende.
func (f *formulario) leerFechaHora(s string, previa time.Time) (time.Time, bool) {
	loc := f.p.zona()
	fecha, hora, _ := strings.Cut(s, ",")
	d, vacia, ok := ui.LeerFecha(fecha, loc)
	if !ok || vacia {
		return time.Time{}, false
	}
	h, m := previa.In(loc).Hour(), previa.In(loc).Minute()
	if !f.e.TodoElDia {
		hh, mm, vaciaH, okH := ui.LeerHora(hora)
		if !okH || vaciaH {
			return time.Time{}, false
		}
		h, m = hh, mm
	}
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc), true
}

// leerFechas recoge lo tecleado en inicio y fin, sin tocar lo que no se entienda.
func (f *formulario) leerFechas() {
	if t, ok := f.leerFechaHora(f.inicio.Texto(), f.e.Inicio); ok {
		f.e.Inicio = t
	}
	if t, ok := f.leerFechaHora(f.fin.Texto(), f.e.Fin); ok {
		f.e.Fin = t
	}
}

// cambiarInicio es `changeStart`: mover el inicio ARRASTRA el fin conservando la
// duración, como Apple Calendar y Google Calendar.
func (f *formulario) cambiarInicio(s string) {
	nuevo, ok := f.leerFechaHora(s, f.e.Inicio)
	if !ok || nuevo.Equal(f.e.Inicio) {
		return
	}
	if fin, okFin := f.leerFechaHora(f.fin.Texto(), f.e.Fin); okFin {
		f.e.Fin = fin
	}
	f.e.Fin = f.e.Fin.Add(nuevo.Sub(f.e.Inicio))
	f.e.Inicio = nuevo
	loc := f.p.zona()
	fin := ui.EscribirFecha(f.e.Fin.In(loc))
	if !f.e.TodoElDia {
		fin += ", " + ui.EscribirHora(f.e.Fin.In(loc))
	}
	f.fin.PonerTexto(fin)
	if f.errFin != "" {
		f.errFin, f.fin.Invalida = "", false
		f.rehacer()
	}
	f.pedirRecursos()
	f.revisarChoques()
}

// cambiarFin es el campo «Fin» al teclear: quita su error, pide las salas libres y
// revisa los choques con la hora nueva.
func (f *formulario) cambiarFin(s string) {
	f.limpiar(&f.errFin, f.fin)
	t, ok := f.leerFechaHora(s, f.e.Fin)
	if !ok {
		return
	}
	f.e.Fin = t
	if f.rec != nil {
		f.pedirRecursos()
	}
	f.revisarChoques()
}

// construirMostrar es «Mostrar como»: ocupado (lo de siempre) o disponible, y el primer
// cálculo de «Coincide con…».
func (f *formulario) construirMostrar() {
	p := f.p
	f.mostrar = ui.NuevoSelector([]string{p.t("event.showAsBusy", nil), p.t("event.showAsFree", nil)}, func(i int) {
		f.e.Libre = i == 1
		f.revisarChoques()
	})
	if f.e.Libre {
		f.mostrar.Elegida = 1
	}
	f.mostrar.Rotulo = p.t("event.showAs", nil)
	f.aviso = f.textoChoques()
}

// textoChoques es «Coincide con…» para las horas puestas: vacío en un día completo, en
// un evento «disponible», en consulta o sin nada que se pise (`choques` en EventModal).
func (f *formulario) textoChoques() string {
	e := f.e
	if f.a.Choques == nil || e.TodoElDia || e.Libre || e.SoloLectura {
		return ""
	}
	l := f.a.Choques(e.Inicio, e.Fin, e.UID)
	if len(l) == 0 {
		return ""
	}
	loc := f.p.zona()
	vars := i18n.Vars{
		"title": resumenDe(l[0].O, f.p.t("event.untitled", nil)),
		"time": i18n.FormatTimeDigits(f.p.locale(), l[0].Inicio.In(loc)) + "–" +
			i18n.FormatTimeDigits(f.p.locale(), l[0].Fin.In(loc)),
	}
	if len(l) == 1 {
		return f.p.t("event.overlaps", vars)
	}
	return f.p.t("event.overlapsMore", vars)
}

// revisarChoques recalcula el aviso y solo rehace el formulario si ha cambiado: se llama
// con cada hora tecleada.
func (f *formulario) revisarChoques() {
	nuevo := f.textoChoques()
	if nuevo == f.aviso {
		return
	}
	f.aviso = nuevo
	// Mientras se arma (escribir las fechas dispara los campos) aún no hay filas que
	// rehacer: las primeras ya salen con el aviso.
	if f.cuerpo != nil {
		f.rehacer()
	}
}

// ancho es el de la ventana, que es lo que miran los cortes `sm:` de la hoja.
func (f *formulario) ventana() float32 {
	if c := fyne.CurrentApp().Driver().CanvasForObject(f.cuerpo); c != nil {
		return c.Size().Width
	}
	return 1280
}

// limpiar quita el error de un campo en cuanto se escribe en él.
func (f *formulario) limpiar(err *string, e *ui.Entrada) {
	if *err == "" {
		return
	}
	*err, e.Invalida = "", false
	f.rehacer()
}

// rehacer vuelve a montar las filas (cambió «todo el día» o la repetición) y recoloca
// el diálogo, que se ajusta a su contenido.
func (f *formulario) rehacer() {
	f.rehacerFilas()
	f.recolocar()
}

// recolocar pide a la capa del diálogo que vuelva a medirlo.
func (f *formulario) recolocar() {
	f.cuerpo.Refresh()
	if c := fyne.CurrentApp().Driver().CanvasForObject(f.cuerpo); c != nil {
		if arriba := c.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
}

func (f *formulario) rehacerFilas() {
	p, e := f.p, f.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var filas []fyne.CanvasObject
	if e.SoloLectura {
		filas = append(filas, ui.NuevoParrafo(p.t("event.readOnlyNotice", nil), tema.TextoSM, ui.Normal, "text-muted", v))
	}
	if f.ia != nil {
		filas = append(filas, f.bloqueAsistente())
	}
	filas = append(filas, ui.GrupoCampo(p.t("event.title", nil), true, f.titulo, "", f.errTitulo),
		ui.GrupoCampo(p.t("event.calendar", nil), false, f.calendario, "", ""))

	// `border-border bg-bg-subtle rounded-md border px-3 py-2`.
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	filas = append(filas, container.New(&conCaja{caja: caja, X: espacio3, Y: espacio2}, append(caja.Objetos(), f.todoElDia)...))

	gInicio := ui.GrupoCampo(p.t("event.start", nil), false, f.inicioCaja, "", "")
	gFin := ui.GrupoCampo(p.t("event.end", nil), false, f.finCaja, "", f.errFin)
	filas = append(filas, container.New(&dosColumnas{Hueco: tema.Espacio(4), Corte: CorteSM, ventana: f.ventana}, gInicio, gFin))
	if f.aviso != "" {
		// «Coincide con…» bajo las horas, que es lo que se toca para arreglarlo: un aviso
		// en `warning-text`, no un error (no bloquea Guardar).
		filas = append(filas, ui.NuevoParrafo(f.aviso, tema.TextoSM, ui.Normal, "warning-text", v).A(f.util()))
	}
	filas = append(filas, ui.GrupoCampo(p.t("event.showAs", nil), false, f.mostrar, p.t("event.showAsHint", nil), ""))
	if !e.TodoElDia {
		filas = append(filas, ui.GrupoCampo(p.t("event.timeZone", nil), false, f.zona, "", ""))
		if f.bh != nil {
			filas = append(filas, f.bloqueHuecos())
		}
	}
	gRepetir := ui.GrupoCampo(p.t("event.repeat", nil), false, f.repetir, "", "")
	fila := []fyne.CanvasObject{gRepetir}
	if e.Regla != nil {
		fila = append(fila, ui.GrupoCampo(p.t("event.repeatEnds", nil), false, f.finSerie, "", ""))
	}
	filas = append(filas, container.New(&dosColumnas{Hueco: tema.Espacio(4), Corte: CorteSM, ventana: f.ventana}, fila...))
	if f.semanal() {
		filas = append(filas, f.bloqueDias())
	}
	if e.Regla != nil && f.finSerie.Elegida == 1 {
		filas = append(filas, ui.GrupoCampo(p.t("event.repeatCount", nil), false, f.cuenta, "", ""))
	}
	if e.Regla != nil && f.finSerie.Elegida == 2 {
		filas = append(filas, ui.GrupoCampo(p.t("event.repeatUntil", nil), false, f.hasta, "", ""))
	}
	filas = append(filas, ui.GrupoCampo(p.t("event.location", nil), false, f.lugar, "", ""),
		f.bloqueVideollamada())
	if b := f.bloqueRecursos(); b != nil {
		filas = append(filas, b)
	}
	filas = append(filas, f.bloqueColor())
	if f.invitaAqui() {
		filas = append(filas, f.bloqueInvitados())
	} else if pista := f.pistaSinInvitar(); pista != "" {
		// Sin personas, en un calendario de este equipo: dónde se puede invitar.
		filas = append(filas, ui.GrupoCampo(p.t("event.guests", nil), false,
			ui.NuevoParrafo(pista, tema.TextoSM, ui.Normal, "text-muted", v).A(f.util()), "", ""))
	}
	filas = append(filas, ui.GrupoCampo(p.t("event.description", nil), false, f.descripcion, "", ""))
	if b := f.bloqueRecordatorios(); b != nil {
		filas = append(filas, b)
	}
	f.cuerpo.Objects = filas
}

// rotuloFuerte es el `<span class="text-text text-sm font-semibold">` de los bloques
// que no son un FieldGroup (videollamada y color).
func rotuloFuerte(s string) *canvas.Text {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return ui.Texto(s, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
}

// bloqueVideollamada es la sección de la sala de Sytue Rooms: el enlace con «Quitar», o
// el botón que la crea con su pista y su error.
func (f *formulario) bloqueVideollamada() fyne.CanvasObject {
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := []fyne.CanvasObject{alto(rotuloFuerte(p.t("event.conference", nil)), tema.TextoSM.Linea)}
	if f.enlace != nil {
		// A-06: cualquier enlace de reunión, que «Unirse» abre en el sistema.
		sub := []fyne.CanvasObject{f.enlace, ui.NuevoParrafo(p.PistaEnlace, tema.TextoXS, ui.Normal, "text-subtle", v)}
		if b := f.bloqueReunion(); b != nil {
			// Sytue Office, A-13: crear la reunión con el proveedor del calendario.
			sub = append(sub, b)
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(1)}, sub...))
		return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, hijos...)
	}
	if url := f.e.Videollamada; url != "" {
		// `border-border-subtle flex items-center justify-between gap-2 rounded-md
		// border px-3 py-2`: el enlace subrayado y el botón fantasma.
		quitar := ui.NuevoBoton(p.t("common.remove", nil), func() { f.e.Videollamada = ""; f.rehacer() }).Con(ui.Fantasma, ui.TallaSM)
		quitar.Rotulo = p.t("event.conferenceRemoveAria", nil)
		enlace := ui.NuevoEnlace(url, tema.TextoSM, ui.Normal, "primary-text", nil)
		caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border-subtle", v), tema.RadioMD)
		hijos = append(hijos, container.New(&filaEnlace{caja: caja, enlace: enlace, boton: quitar},
			append(caja.Objetos(), enlace, quitar)...))
	} else {
		rotulo := p.t("event.conferenceAdd", nil)
		if f.creandoSala {
			rotulo = p.t("event.conferenceCreating", nil)
		}
		crear := ui.NuevoBoton(rotulo, f.crearSala).Con(ui.Secundario, ui.TallaSM)
		crear.Inerte = f.creandoSala || f.a.Videollamada == nil
		sub := []fyne.CanvasObject{encajar(crear), ui.NuevoParrafo(p.t("event.conferenceHint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)}
		if f.errorVideo {
			sub = append(sub, ui.NuevoParrafo(p.t("event.conferenceError", nil), tema.TextoXS, ui.Normal, "danger-text", v))
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(1)}, sub...))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, hijos...)
}

// crearSala es `handleAddConference`: el UID tiene que ser el que se guarde después.
func (f *formulario) crearSala() {
	if f.creandoSala || f.a.Videollamada == nil {
		return
	}
	if f.e.UID == "" {
		f.e.UID = nuevoUID()
	}
	f.creandoSala, f.errorVideo = true, false
	f.rehacer()
	titulo := strings.TrimSpace(f.titulo.Texto())
	if titulo == "" {
		titulo = f.p.t("event.conferenceDefaultSummary", nil)
	}
	f.a.Videollamada(f.e.UID, titulo, func(enlace string, ok bool) {
		f.creandoSala = false
		if ok {
			f.e.Videollamada = enlace
		} else {
			f.errorVideo = true
		}
		f.rehacer()
	})
}

// bloqueColor es «Color» con sus muestras, la primera «color del calendario».
func (f *formulario) bloqueColor() fyne.CanvasObject {
	m := nuevasMuestras(f.p, f.e.Color, true, func(hex string) { f.e.Color = hex })
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, alto(rotuloFuerte(f.p.t("common.colour", nil)), tema.TextoSM.Linea), m)
}

// bloqueInvitados son las píldoras de los añadidos y el campo del correo; sin personas
// con Pantalla.Invitar, también las direcciones de la libreta y la pista de por dónde
// saldrá la invitación.
func (f *formulario) bloqueInvitados() fyne.CanvasObject {
	rotulo := f.p.t("event.guests", nil)
	campo := fyne.CanvasObject(f.correo)
	if extra := f.bajoElCampo(); len(extra) > 0 {
		campo = container.New(ui.Columna{Hueco: espacio1p5}, append([]fyne.CanvasObject{f.correo}, extra...)...)
	}
	if len(f.e.Invitados) == 0 {
		return ui.GrupoCampo(rotulo, false, campo, "", f.errCorreo)
	}
	// Al editar, la respuesta que ya dio cada uno y quién organiza (GuestList.tsx).
	respuestas := map[string]string{}
	organizador := ""
	if f.e.O != nil {
		for _, a := range f.e.O.Event.Attendees {
			// Un ATTENDEE sin PARTSTAT (llega así por CalDAV de otros clientes) ya
			// estaba invitado: la web lo enseña «Sin responder», no sin insignia.
			respuestas[strings.ToLower(a.Email)] = cmp.Or(a.PartStat, "NEEDS-ACTION")
		}
		organizador = nombreOrganizador(f.e.O.Event)
	}
	var pildoras []fyne.CanvasObject
	for _, correo := range f.e.Invitados {
		pildoras = append(pildoras, nuevaPildoraInvitado(f.p, correo, respuestas[strings.ToLower(correo)], func() {
			f.e.Invitados = quitar(f.e.Invitados, correo)
			f.rehacer()
			f.invitadosCambiaron()
		}))
	}
	lista := container.New(ui.FilaAuto{Hueco: espacio1p5, SaltoFila: espacio1p5, Derecha: len(pildoras), Partir: true}, pildoras...)
	filas := []fyne.CanvasObject{lista, campo}
	if organizador != "" {
		filas = append([]fyne.CanvasObject{f.p.filaOrganizador(organizador, f.util(), tema.TextoSM)}, filas...)
	}
	return ui.GrupoCampo(rotulo, false, container.New(ui.Columna{Hueco: espacio2}, filas...), "", f.errCorreo)
}

// bajoElCampo son la lista de la libreta y la pista (sin personas con Invitar).
func (f *formulario) bajoElCampo() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if l := f.listaSugerencias(); l != nil {
		out = append(out, l)
	}
	if pista := f.pistaInvitar(); pista != "" {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		out = append(out, ui.NuevoParrafo(pista, tema.TextoXS, ui.Normal, "text-subtle", v).A(f.util()))
	}
	return out
}

func quitar(l []string, s string) []string {
	out := make([]string, 0, len(l))
	for _, x := range l {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// anadirInvitado es `addInvitee`: al pulsar Intro o al salir del campo.
func (f *formulario) anadirInvitado(bruto string) {
	correo := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(bruto), ",;"))
	if correo == "" {
		return
	}
	if !correoValido.MatchString(correo) {
		f.errCorreo, f.correo.Invalida = f.p.t("event.guestInvalid", nil), true
		f.rehacer()
		return
	}
	for _, x := range f.e.Invitados {
		if strings.EqualFold(x, correo) {
			f.correo.PonerTexto("")
			return
		}
	}
	f.e.Invitados = append(f.e.Invitados, correo)
	f.correo.PonerTexto("")
	f.errCorreo, f.correo.Invalida = "", false
	f.rehacer()
	f.invitadosCambiaron()
}

// pie es el de EventModal: «Eliminar» a la izquierda al editar, y «Cancelar» y
// «Guardar» a la derecha; en consulta, un «Cerrar».
func (f *formulario) pie() fyne.CanvasObject {
	p := f.p
	if f.e.SoloLectura {
		f.cancelar = ui.NuevoBoton(p.t("common.close", nil), func() { f.d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
		return container.New(ui.PieAcciones{}, f.cancelar)
	}
	f.cancelar = ui.NuevoBoton(p.t("common.cancel", nil), func() { f.d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	f.guardar = ui.NuevoBoton(p.t("common.save", nil), f.enviar).Con(ui.Primario, ui.TallaMD)
	if f.nuevo {
		return container.New(ui.PieAcciones{}, f.cancelar, f.guardar)
	}
	f.eliminar = ui.NuevoBoton(p.t("common.delete", nil), f.borrar).Con(ui.Peligro, ui.TallaMD)
	// «Eliminar» a la izquierda en escritorio; en el teléfono, sola arriba y Cancelar |
	// Guardar debajo (`ui.PieAcciones`, el pie de Modal.tsx).
	return container.New(ui.PieAcciones{Izquierda: 1}, f.eliminar, f.cancelar, f.guardar)
}

// inertes apaga todos los controles: el `<fieldset disabled>` del modo consulta.
func (f *formulario) inertes() {
	for _, e := range []*ui.Entrada{f.titulo, f.inicio, f.fin, f.lugar, f.correo, f.cuenta, f.hasta} {
		e.Inerte(true)
	}
	if f.enlace != nil {
		f.enlace.Inerte(true)
	}
	for _, s := range []*ui.Selector{f.calendario, f.zona, f.repetir, f.finSerie} {
		s.Inerte(true)
	}
	f.todoElDia.Disable()
	f.descripcion.Inerte(true)
	for _, b := range f.iconosFecha {
		b.Inerte = true
	}
	if f.ia != nil {
		f.ia.Inerte(true)
		f.iaBoton.Inerte = true
	}
	f.recordatoriosInertes()
}

// ocupar es el `busy`: solo el botón pulsado lleva la rueda y el resto se apaga.
func (f *formulario) ocupar(b *ui.Boton, si bool) {
	f.ocupado = si
	for _, x := range []*ui.Boton{f.guardar, f.eliminar, f.cancelar} {
		if x == nil {
			continue
		}
		x.Inerte = si
		x.Cargando = si && x == b
		x.Refresh()
	}
}

// enviar es `handleSave`: valida junto al campo y, si todo vale, pide a la app que
// guarde.
func (f *formulario) enviar() {
	if f.ocupado {
		return
	}
	p := f.p
	if f.e.CalendarioID == "" {
		if f.a.Aviso != nil {
			f.a.Aviso("toast.selectCalendar")
		}
		return
	}
	f.e.Titulo = strings.TrimSpace(f.titulo.Texto())
	if f.e.Titulo == "" {
		f.errTitulo, f.titulo.Invalida = p.t("event.titleRequired", nil), true
		f.rehacer()
		fyne.CurrentApp().Driver().CanvasForObject(f.titulo).Focus(f.titulo)
		return
	}
	f.leerFechas()
	if f.e.FinAntesDelInicio(p.zona()) {
		f.errFin, f.fin.Invalida = p.t("event.endBeforeStart", nil), true
		f.rehacer()
		return
	}
	if !f.invitaAqui() {
		// Sin personas, en un calendario donde no se puede invitar: los invitados son los
		// que traía el evento (los que se añadieran en otro calendario no se guardan).
		f.e.Invitados = slices.Clone(f.invitadosIniciales)
	} else if borrador := strings.TrimSpace(f.correo.Texto()); borrador != "" {
		// Un correo tecleado y sin confirmar: se añade si vale y, si no, se para aquí.
		if !correoValido.MatchString(borrador) {
			f.errCorreo, f.correo.Invalida = p.t("event.guestInvalid", nil), true
			f.rehacer()
			return
		}
		f.anadirInvitado(borrador)
	}
	if f.a.Guardar == nil {
		return
	}
	// Una reserva se liga al evento por su UID: al crear, hace falta ya (`ensureUid`).
	if len(f.e.Recursos) > 0 && f.e.UID == "" {
		f.e.UID = nuevoUID()
	}
	// La reunión pedida al proveedor del calendario va con el UID del evento.
	f.pedirReunion()
	f.ocupar(f.guardar, true)
	f.a.Guardar(f.e, func(ok bool) {
		f.ocupar(f.guardar, false)
		if ok {
			f.d.Cerrar()
		}
	})
}

// borrar es `handleDelete`.
func (f *formulario) borrar() {
	if f.ocupado || f.a.Eliminar == nil {
		return
	}
	f.ocupar(f.eliminar, true)
	f.a.Eliminar(f.e, func(ok bool) {
		f.ocupar(f.eliminar, false)
		if ok {
			f.d.Cerrar()
		}
	})
}

// EntradaDe es lo que se manda al servicio: el evento ENTERO, porque el PUT sustituye.
// Parte del evento guardado cuando se edita, para no perder lo que el formulario no
// enseña (el organizador, el estado, la transparencia, los PARTSTAT de los invitados).
//
//nolint:gocyclo // Copia campo a campo el evento entero para el PUT, que sustituye: partirla es repartir la misma copia.
func (e Edicion) EntradaDe(p *Pantalla) api.EventInput {
	in := api.EventInput{}
	if e.O != nil {
		in = e.O.Event.Input()
	}
	in.Summary, in.Description, in.Location = e.Titulo, e.Descripcion, e.Lugar
	in.Color, in.ConferenceURL = e.Color, e.Videollamada
	in.AllDay = e.TodoElDia
	in.Transparency = "OPAQUE"
	if e.Libre {
		in.Transparency = "TRANSPARENT"
	}
	if e.TodoElDia {
		// Día completo: FLOTANTE, a las 00:00 UTC de su fecha y sin zona. El «Fin» del
		// formulario es el ÚLTIMO día y el DTEND es exclusivo (RFC 5545 §3.6.1): «8 → 10
		// nov» guarda el 11, y un día suelto —o un fin que no pasa del inicio— el día
		// siguiente, nunca DTEND = DTSTART.
		d := func(t time.Time) time.Time {
			t = t.In(p.zona())
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}
		ini, ultimo := d(e.Inicio), d(e.Fin)
		if e.Fin.IsZero() || ultimo.Before(ini) {
			ultimo = ini
		}
		in.Start, in.End, in.TZID = ini, ultimo.AddDate(0, 0, 1), ""
	} else {
		// La hora del formulario es la de PARED en la zona elegida.
		loc := e.zonaDe(p)
		pared := func(t time.Time) time.Time {
			t = t.In(p.zona())
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		}
		in.Start, in.End, in.TZID = pared(e.Inicio), pared(e.Fin), e.Zona
		if o := e.O; o != nil && !o.Event.AllDay && (e.Zona == o.Event.TZID || o.Event.TZID == "" && e.Zona == p.nombreZona()) {
			// Con la zona de siempre, lo que no se ha tocado se queda en su instante tal
			// cual: una hora de pared que no existe en la zona de visualización (la que
			// se salta al pasar al horario de verano) o que existe dos veces (la que se
			// repite al volver) no se puede llevar en los campos sin moverse una hora.
			ini, fin := p.camposDe(*o)
			if mismaPared(e.Inicio, ini, p.zona()) {
				in.Start = o.Start
			}
			if mismaPared(e.Fin, fin, p.zona()) {
				in.End = o.End
			}
			if o.Event.TZID == "" {
				// Un evento sin zona (las horas en UTC de un .ics importado o de otro
				// cliente) se queda sin ella si no se elige otra: ponerle la de
				// visualización, que es la que enseña el formulario, movería las
				// repeticiones de una serie que caen al otro lado de un cambio de hora.
				in.TZID = ""
			}
		}
	}
	in.RRule = ""
	switch {
	case e.Regla != nil && e.Regla.Freq != "":
		in.RRule = e.Regla.RRule()
	case e.ReglaCruda != "":
		in.RRule = e.ReglaCruda
	}
	// Los invitados que ya estaban conservan su respuesta; los nuevos entran a la espera.
	previos := map[string]api.Attendee{}
	for _, a := range in.Attendees {
		previos[strings.ToLower(a.Email)] = a
	}
	in.Attendees = nil
	for _, correo := range e.Invitados {
		a, ok := previos[strings.ToLower(correo)]
		if !ok {
			a = api.Attendee{Email: correo, Role: "REQ-PARTICIPANT", PartStat: "NEEDS-ACTION", RSVP: true}
		}
		in.Attendees = append(in.Attendees, a)
	}
	// Sin personas (Sytue Office) no se manda nada a nadie: los invitados que trajera el
	// evento se conservan, pero guardarlo no es invitarlos.
	in.Send = len(in.Attendees) > 0 && !p.SinPersonas
	// Sin saber los recordatorios, no viajan y el servicio conserva los que hubiera; si
	// se saben —aunque sean cero—, sustituyen a los que la web deja editar.
	in.Reminders = nil
	if e.Recordatorios != nil {
		l := append([]int{}, *e.Recordatorios...)
		in.Reminders = &l
	}
	return in
}

// FinAntesDelInicio es el aviso «el fin es anterior al inicio» de `handleSave`. En un
// día completo cuentan solo las fechas: los campos conservan una hora que no se ve (la
// de antes de encender «Todo el día»), y un 8 22:00 → 8 01:00 no es un fin anterior.
func (e Edicion) FinAntesDelInicio(loc *time.Location) bool {
	if e.TodoElDia {
		return inicioDelDia(e.Fin, loc).Before(inicioDelDia(e.Inicio, loc))
	}
	return e.Fin.Before(e.Inicio)
}

// finInclusivo es el de dates.ts: el último día de un día completo a partir de su
// DTEND exclusivo, que es lo que enseña el campo «Fin» (un «8–10» de Apple se abría con
// fin el 11). Sin fin, o con uno que no pasa del inicio —de otros clientes, o de los
// guardados antes con el fin inclusivo—, el último día es el del inicio.
func finInclusivo(inicio, fin time.Time) time.Time {
	if fin.IsZero() {
		return inicio
	}
	if ultimo := fin.AddDate(0, 0, -1); !ultimo.Before(inicio) {
		return ultimo
	}
	return inicio
}

// zonaDe es la zona en la que se lee la hora de pared de los campos: la de `Zona` o, si
// está vacía o no se conoce, la de visualización.
func (e Edicion) zonaDe(p *Pantalla) *time.Location {
	if e.Zona != "" {
		if loc, err := time.LoadLocation(e.Zona); err == nil {
			return loc
		}
	}
	return p.zona()
}

// enLaPared es `t` con la hora de pared que tiene en la zona `de`, puesta en la zona `a`.
func enLaPared(t time.Time, de, a *time.Location) time.Time {
	if t.IsZero() {
		return t
	}
	t = t.In(de)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), a)
}

// mismaPared dice si `a` y `b` dicen el mismo día y la misma hora y minuto en `loc`, que
// es lo que dicen los campos.
func mismaPared(a, b time.Time, loc *time.Location) bool {
	a, b = a.In(loc), b.In(loc)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay() && a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

// camposDe son el inicio y el fin de la ocurrencia como los enseñan los campos (Edicion).
// Un evento con hora, con la hora de pared de SU zona —la que dice «Zona horaria», y en la
// que EntradaDe la vuelve a leer—, se pinte en la zona que se pinte: uno de las 10:00 de
// Madrid abierto con la pantalla en UTC dice 10:00, y guardarlo sin tocar nada lo deja
// donde estaba. Antes decía 09:00 y se guardaba a las 09:00 de Madrid.
func (p *Pantalla) camposDe(o Ocurrencia) (ini, fin time.Time) {
	ini, fin = ocurrenciaInstantes(o, p.zona())
	switch {
	case o.Event.AllDay:
		// Flotante: el mismo día en cualquier zona (ocurrenciaInstantes).
		fin = finInclusivo(ini, fin)
	case o.Event.TZID != "":
		loc := Edicion{Zona: o.Event.TZID}.zonaDe(p)
		ini, fin = enLaPared(ini, loc, p.zona()), enLaPared(fin, loc, p.zona())
	}
	return ini, fin
}

// EdicionDe es `editingDe`: la ocurrencia tal y como la entiende el formulario.
func (p *Pantalla) EdicionDe(o Ocurrencia) Edicion {
	ini, fin := p.camposDe(o)
	e := Edicion{
		O: &o, CalendarioID: o.CalendarioID, Titulo: o.Event.Summary,
		Inicio: ini, Fin: fin, TodoElDia: o.Event.AllDay, Zona: o.Event.TZID,
		Lugar: o.Event.Location, Descripcion: o.Event.Description, Color: o.Event.Color,
		Videollamada: o.Event.ConferenceURL, SoloLectura: o.SoloLectura, UID: o.Event.UID,
		Libre: Disponible(o),
	}
	if r, ok := LeerRRule(o.Event.RRule); ok {
		e.Regla = &r
	} else if strings.TrimSpace(o.Event.RRule) != "" {
		e.ReglaCruda = o.Event.RRule
	}
	for _, a := range o.Event.Attendees {
		e.Invitados = append(e.Invitados, a.Email)
	}
	return e
}

// nuevoUID es el `crypto.randomUUID()@calendar.sytue.eu` de la web.
func nuevoUID() string { return uuidV4() + "@calendar.sytue.eu" }

// ---- Piezas de maquetación del formulario ----

// conCaja es una caja con relleno alrededor de UN control: el primer o los dos primeros
// objetos son la caja (según lleve borde) y el último, el control.
type conCaja struct {
	caja *ui.Caja
	X, Y float32
}

func (c *conCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[len(objs)-1].MinSize().AddWidthHeight(2*(c.X+tema.Borde), 2*(c.Y+tema.Borde))
}

func (c *conCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	o := objs[len(objs)-1]
	m := o.MinSize()
	o.Move(fyne.NewPos(tema.Borde+c.X, (t.Height-m.Height)/2))
	o.Resize(fyne.NewSize(max(t.Width-2*(c.X+tema.Borde), 0), m.Height))
}

// dosColumnas es `grid grid-cols-1 gap-4 sm:grid-cols-2`: dos columnas con la ventana
// de `sm` para arriba, una debajo de otra por debajo.
type dosColumnas struct {
	Hueco, Corte float32
	ventana      func() float32
}

func (d *dosColumnas) anchas() bool { return d.ventana == nil || d.ventana() >= d.Corte }

func (d *dosColumnas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if !d.anchas() {
		return ui.Columna{Hueco: d.Hueco}.MinSize(objs)
	}
	var ancho, alto float32
	for _, o := range objs {
		m := o.MinSize()
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho+d.Hueco, alto)
}

func (d *dosColumnas) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	if !d.anchas() {
		return ui.AltoA(container.New(ui.Columna{Hueco: d.Hueco}, objs...), ancho)
	}
	col := max((ancho-d.Hueco)/2, 0)
	alto := float32(0)
	for _, o := range objs {
		alto = max(alto, ui.AltoA(o, col))
	}
	return alto
}

func (d *dosColumnas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if !d.anchas() {
		ui.Columna{Hueco: d.Hueco}.Layout(objs, t)
		return
	}
	col := (t.Width - d.Hueco) / 2
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(col+d.Hueco), 0))
		o.Resize(fyne.NewSize(col, ui.AltoA(o, col)))
	}
}

// pieRepartido es la fila de acciones de un formulario DENTRO de un cuerpo (compartir,
// recursos): `flex justify-end gap-2`. Si no caben, la fila compartida los reparte en
// varias líneas: ningún botón recibe una X negativa ni se sale del diálogo en un teléfono.
// El pie de un diálogo es `ui.PieAcciones`.
type pieRepartido struct{}

func (p *pieRepartido) fila() ui.FilaAuto {
	return ui.FilaAuto{Hueco: espacio2, Partir: true, SaltoFila: espacio2}
}

func (p *pieRepartido) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return p.fila().MinSize(objs)
}

func (p *pieRepartido) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return p.fila().AltoCon(objs, ancho)
}

func (p *pieRepartido) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	p.fila().Layout(objs, t)
}

// filaEnlace es la caja de la videollamada puesta: el enlace se recorta y el botón va
// a su medida a la derecha.
type filaEnlace struct {
	caja   *ui.Caja
	enlace *ui.Enlace
	boton  *ui.Boton
}

func (f *filaEnlace) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(f.boton.MinSize().Height, tema.TextoSM.Linea)+2*(espacio2+tema.Borde))
}

func (f *filaEnlace) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.caja.Redimensionar(t)
	b := f.boton.MinSize()
	f.boton.Move(fyne.NewPos(t.Width-tema.Borde-espacio3-b.Width, (t.Height-b.Height)/2))
	f.boton.Resize(b)
	e := f.enlace.MinSize()
	w := max(t.Width-2*(tema.Borde+espacio3)-b.Width-espacio2, 0)
	f.enlace.Move(fyne.NewPos(tema.Borde+espacio3, (t.Height-e.Height)/2))
	f.enlace.Resize(fyne.NewSize(min(e.Width, w), e.Height))
}

// alto fija el alto de un texto suelto a su caja de línea de CSS.
func alto(t *canvas.Text, linea float32) fyne.CanvasObject {
	return container.New(&cajaLinea{Linea: linea}, t)
}

type cajaLinea struct{ Linea float32 }

func (c *cajaLinea) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(objs[0].MinSize().Width, c.Linea)
}

func (c *cajaLinea) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	if t, ok := objs[0].(*canvas.Text); ok {
		ui.ColocarTexto(t, tema.Escalon{Tam: t.TextSize, Linea: c.Linea}, 0, 0)
	}
}

// encajar deja un objeto a su medida y a la izquierda: el `<div>` que envuelve el botón
// de «Añadir videollamada» para que no se estire.
func encajar(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&aSuMedida{}, o)
}

type aSuMedida struct{}

func (aSuMedida) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }
func (aSuMedida) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(objs[0].MinSize())
}

// anchoUtilFormulario es lo que queda dentro del diálogo `md` en un lienzo de `tam`: 560
// menos sus bordes y el `px-6` de cada lado; en la hoja del teléfono, la pantalla menos el
// `px-6`. Era un 510 fijo, y en 390 las tarjetas de «Compartir» y las horas de «Buscar un
// hueco» se medían para 510 y se pintaban en 342: el texto se salía de su caja.
func anchoUtilFormulario(tam fyne.Size) float32 { return ui.AnchoCuerpoTalla(ui.DialogoMD, tam) }

// util es anchoUtilFormulario donde está el formulario; sin pintar todavía, el del
// escritorio (AbrirFormulario lo rehace ya puesto).
func (f *formulario) util() float32 {
	var tam fyne.Size
	if f.cuerpo != nil {
		if c := fyne.CurrentApp().Driver().CanvasForObject(f.cuerpo); c != nil {
			tam = c.Size()
		}
	}
	return anchoUtilFormulario(tam)
}
