// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que la persona HACE con un evento: abrir su vista rápida, crearlo, editarlo,
// duplicarlo, moverlo, borrarlo y contestar a la invitación. Es la mitad de
// CalendarApp.tsx que no pinta: cada acción habla con el servicio, avisa con un aviso
// del catálogo y recarga.

// plazo es lo que se espera al servicio en una escritura.
const plazo = 20 * time.Second

// lienzo es el de la ventana donde está la app, que es donde van diálogos y paneles.
func (a *App) lienzo() fyne.Canvas {
	if a.raiz == nil {
		return nil
	}
	return fyne.CurrentApp().Driver().CanvasForObject(a.raiz)
}

// avisar saca un aviso con una clave del catálogo.
func (a *App) avisar(tono ui.TonoAviso, clave string) {
	a.avisos.Notificar(ui.NuevoAviso(tono, a.P.t(clave, nil), ""))
}

// escribir lanza una escritura contra el servicio y vuelve al hilo de la interfaz con
// su resultado. Sin cliente (una prueba sin servicio), falla.
func (a *App) escribir(f func(ctx context.Context) error, listo func(err error)) {
	if a.Cal == nil {
		listo(errSinServicio)
		return
	}
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), plazo)
		defer parar()
		err := f(ctx)
		a.enUI(func() { listo(err) })
	})
}

type errorSinServicio struct{}

func (errorSinServicio) Error() string { return "calendario: sin servicio" }

var errSinServicio error = errorSinServicio{}

// destinos es `eventTargets`: donde se puede crear o mover un evento, los propios y los
// compartidos para editar.
func (a *App) destinos() []api.Calendar {
	out := append([]api.Calendar(nil), a.P.Calendarios...)
	for _, c := range a.P.Compartidos {
		if !c.ReadOnly {
			out = append(out, c)
		}
	}
	return out
}

// ---- La vista rápida ----

// abrirVistaRapida es `openQuickView`: el panel junto al evento pulsado.
func (a *App) abrirVistaRapida(o Ocurrencia, ancla fyne.CanvasObject) {
	c := a.lienzo()
	if c == nil || ancla == nil {
		return
	}
	a.cerrarVistaRapida()
	if w := c.Size().Width; w > 0 && w < ui.Estrecho {
		a.abrirVistaRapidaHoja(c, o)
		return
	}
	var e *ui.Emergente
	acciones := AccionesVistaRapida{
		Cerrar:   func() { e.Cerrar() },
		Editar:   func() { e.Cerrar(); a.editar(o) },
		Duplicar: func() { e.Cerrar(); a.duplicar(o) },
		Eliminar: func() { e.Cerrar(); a.eliminar(o) },
		Responder: func(estado string) {
			e.Cerrar()
			a.responder(o, estado)
		},
	}
	if a.AbrirEnlace != nil {
		acciones.Unirse = func() { a.AbrirEnlace(o.Event.ConferenceURL) }
	}
	if a.Notas != nil && o.Event.UID != "" {
		uid := o.Event.UID
		acciones.Notas = func(listo func([]api.Note, error)) {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				l, err := a.Notas(ctx, uid)
				a.enUI(func() { listo(l, err) })
			})
		}
		acciones.AbrirNotas = func(ruta string) {
			e.Cerrar()
			if a.AbrirNotas != nil {
				a.AbrirNotas(ruta)
			}
		}
		acciones.Crecer = func() {
			if e != nil {
				e.Recolocar()
			}
		}
	}
	contenido := a.P.nuevaVistaRapida(o, a.meHanInvitado(o), acciones)
	e = ui.MostrarEmergenteEn(c, contenido, rellenoRapida,
		func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
			tam.Width = min(tam.Width, ventana.Width-2*MargenVistaRapida) // `max-w-[calc(100vw-16px)]`
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(ancla)
			return ColocarVistaRapida(pos, ancla.Size(), tam, ventana), tam
		})
	e.AlRecogerse = func() {
		if a.vista == e {
			a.vista = nil
		}
	}
	a.vista = e
}

// cerrarVistaRapida la quita si está puesta.
func (a *App) cerrarVistaRapida() {
	if v := a.vista; v != nil {
		a.vista = nil
		v.Cerrar()
	}
}

// meHanInvitado es la regla de la web: SOLO si mi dirección está entre los asistentes.
// Al organizador no se le pregunta «¿vas a ir?» a su propia reunión.
func (a *App) meHanInvitado(o Ocurrencia) bool {
	if a.Correo == "" {
		return false
	}
	for _, at := range o.Event.Attendees {
		if strings.EqualFold(strings.TrimPrefix(strings.ToLower(at.Email), "mailto:"), a.Correo) {
			return true
		}
	}
	return false
}

// responder es `responderInvitacion`: el PARTSTAT desde el propio evento.
func (a *App) responder(o Ocurrencia, estado string) {
	if a.P.SinConexion {
		return
	}
	partstat := strings.ToUpper(estado)
	a.escribir(func(ctx context.Context) error {
		return a.Cal.SetPartStat(ctx, o.Event.ID, partstat)
	}, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "popover.respondFailed")
			return
		}
		a.avisar(ui.AvisoExito, "popover.responded")
		a.Recargar()
	})
}

// ---- Crear y editar ----

// crearEn es `openCreate(day)`: en mes y agenda el día no dice hora y el evento nace a
// las 9:00; en semana y día, a la hora del cursor. Una hora.
func (a *App) crearEn(dia time.Time) {
	p := a.P
	if p.SinConexion {
		return
	}
	inicio := dia.In(p.zona())
	if p.Vista == VistaMes || p.Vista == VistaAgenda {
		inicio = time.Date(inicio.Year(), inicio.Month(), inicio.Day(), 9, 0, 0, 0, p.zona())
	}
	a.abrirFormulario(Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour)})
}

// crearRango es `openCreateRange`: con el rango pintado en la rejilla horaria.
func (a *App) crearRango(inicio, fin time.Time) {
	if a.P.SinConexion {
		return
	}
	a.abrirFormulario(Edicion{Inicio: inicio, Fin: fin})
}

// editar es `openEdit`.
func (a *App) editar(o Ocurrencia) {
	a.abrirFormulario(a.P.EdicionDe(o))
}

// duplicar es `duplicateFromQuickView`: el mismo evento SIN identidad en el formulario
// de crear. De una suscripción de solo lectura, la copia nace en tu calendario por
// defecto, que esa sí es tuya.
func (a *App) duplicar(o Ocurrencia) {
	if a.P.SinConexion {
		return
	}
	e := a.P.EdicionDe(o)
	e.O, e.UID, e.SoloLectura, e.desde = nil, "", false, o.Event.ID
	if o.SoloLectura {
		e.CalendarioID = ""
	}
	a.abrirFormulario(e)
}

func (a *App) abrirFormulario(e Edicion) {
	if c := a.lienzo(); c != nil {
		a.P.AbrirFormulario(c, e, a.destinos(), a.accionesFormulario(e))
	}
}

// accionesFormulario es lo que el formulario de `e` pide al servicio.
func (a *App) accionesFormulario(e Edicion) AccionesFormulario {
	acc := AccionesFormulario{
		Guardar:  a.guardar,
		Eliminar: func(e Edicion, listo func(bool)) { a.borrarDesdeFormulario(e, listo) },
		Aviso:    func(clave string) { a.avisar(ui.AvisoPeligro, clave) },
	}
	if a.Salas != nil {
		acc.Videollamada = func(uid, titulo string, listo func(string, bool)) {
			var enlace string
			a.escribir(func(ctx context.Context) error {
				var err error
				enlace, err = a.Salas(ctx, uid, titulo)
				return err
			}, func(err error) { listo(enlace, err == nil) })
		}
	}
	if a.Cal != nil && !a.P.SinConexion {
		acc.Huecos = func(desde, hasta time.Time, minutos int, zona string, laborables bool, listo func([]api.Hueco, error)) {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				l, err := a.Cal.SuggestSlots(ctx, desde, hasta, minutos, zona, FranjaDesde, FranjaHasta, laborables)
				a.enUI(func() { listo(l, err) })
			})
		}
		acc.Recursos = func(desde, hasta time.Time, listo func([]api.Resource, error)) {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				l, err := a.Cal.Resources(ctx, desde, hasta)
				a.enUI(func() { listo(l, err) })
			})
		}
	}
	if a.Asistente != nil && !a.P.SinConexion {
		acc.Asistente = func(pet api.PeticionIA, listo func(string, error)) {
			a.lanzar(func() {
				// Los 130 s del modelo de calidad en el proxy de la web, y un margen.
				ctx, parar := context.WithTimeout(context.Background(), 135*time.Second)
				defer parar()
				texto, err := a.Asistente(ctx, pet)
				a.enUI(func() { listo(texto, err) })
			})
		}
	}
	if id := e.origen(); id != "" && a.Cal != nil && !a.P.SinConexion {
		acc.Recordatorios = func(listo func([]int, error)) {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				ev, err := a.Cal.GetEvent(ctx, id)
				if err == nil && ev.Reminders == nil {
					// Un servicio que no los manda no dice «ninguno»: guardar [] los borraría.
					err = errSinServicio
				}
				var l []int
				if err == nil {
					l = append([]int{}, *ev.Reminders...)
				}
				a.enUI(func() { listo(l, err) })
			})
		}
	}
	return acc
}

// guardar es `handleSave`: una serie pregunta el alcance; lo demás crea, sustituye o
// —si cambió de calendario— crea en el destino y borra el original (primero crear: si
// el borrado falla queda un duplicado, nunca se pierde).
func (a *App) guardar(e Edicion, listo func(ok bool)) {
	in := e.EntradaDe(a.P)
	in.UID = e.UID // solo lo lee el servicio al crear
	nuevo := e.O == nil
	if !nuevo && esSerie(*e.O) {
		a.preguntarAlcance(false, func(alcance string) {
			a.escribir(func(ctx context.Context) error {
				return EditarSerie(ctx, a.Cal, *e.O, alcance, in, e.Regla)
			}, func(err error) { a.trasGuardar(err, false, "toast.eventSaveFailed", listo) })
		}, func() { listo(false) })
		return
	}
	var reservas []string
	a.escribir(func(ctx context.Context) error {
		err := a.sustituir(ctx, e, in)
		if err == nil && len(e.Recursos) > 0 {
			reservas = a.reservar(ctx, e.UID, in, e.Recursos)
		}
		return err
	}, func(err error) {
		if err != nil || len(reservas) == 0 {
			a.trasGuardar(err, nuevo, "cal.error.saveEvent", listo)
			return
		}
		// Con reservas, el aviso es su resumen: `bookSelectedResources`.
		tono, texto := avisoReservas(a.P, reservas)
		a.avisos.Notificar(ui.NuevoAviso(tono, texto, ""))
		listo(true)
		a.Recargar()
	})
}

// sustituir crea, sustituye o —si cambió de calendario— crea en el destino y borra el
// original.
func (a *App) sustituir(ctx context.Context, e Edicion, in api.EventInput) error {
	switch {
	case e.O == nil:
		_, err := a.Cal.CreateEvent(ctx, e.CalendarioID, in)
		return err
	case e.CalendarioID != e.O.CalendarioID:
		if _, err := a.Cal.CreateEvent(ctx, e.CalendarioID, in); err != nil {
			return err
		}
		return a.Cal.DeleteEvent(ctx, e.O.Event.ID)
	}
	_, err := a.Cal.UpdateEvent(ctx, e.O.Event.ID, in)
	return err
}

// reservar reserva cada recurso para el evento recién guardado y devuelve el estado de
// cada uno. Una reserva que falla cuenta como rechazada: sin confirmación, no se da
// por hecha (lo mismo que la web).
//
// ponytail: una detrás de otra, donde la web las lanza a la vez; son unas pocas salas.
func (a *App) reservar(ctx context.Context, uid string, in api.EventInput, ids []string) []string {
	estados := make([]string, len(ids))
	for i, id := range ids {
		estado, err := a.Cal.BookResource(ctx, id, reservaDe(uid, in))
		if err != nil {
			estado = api.ReservaRechazada
		}
		estados[i] = estado
	}
	return estados
}

func (a *App) trasGuardar(err error, nuevo bool, claveFallo string, listo func(bool)) {
	if err != nil {
		a.avisar(ui.AvisoPeligro, claveFallo)
		listo(false)
		return
	}
	if nuevo {
		a.avisar(ui.AvisoExito, "toast.eventCreated")
	} else {
		a.avisar(ui.AvisoExito, "toast.eventUpdated")
	}
	listo(true)
	a.Recargar()
}

// preguntarAlcance abre ScopeDialog.
func (a *App) preguntarAlcance(borrar bool, si func(alcance string), no func()) {
	c := a.lienzo()
	if c == nil {
		no()
		return
	}
	a.P.DialogoAlcance(borrar, si, no).Mostrar(c)
}

// ---- Borrar ----

// eliminar es `deleteFromQuickView`: una serie pregunta el alcance.
func (a *App) eliminar(o Ocurrencia) {
	if a.P.SinConexion || o.SoloLectura {
		return
	}
	a.borrar(o, func(bool) {})
}

func (a *App) borrarDesdeFormulario(e Edicion, listo func(bool)) {
	if e.O == nil {
		listo(false)
		return
	}
	a.borrar(*e.O, listo)
}

func (a *App) borrar(o Ocurrencia, listo func(bool)) {
	hecho := func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "cal.error.deleteEvent")
			listo(false)
			return
		}
		a.avisar(ui.AvisoExito, "toast.eventDeleted")
		listo(true)
		a.Recargar()
	}
	if esSerie(o) {
		a.preguntarAlcance(true, func(alcance string) {
			a.escribir(func(ctx context.Context) error { return BorrarSerie(ctx, a.Cal, o, alcance) }, hecho)
		}, func() { listo(false) })
		return
	}
	a.escribir(func(ctx context.Context) error { return a.Cal.DeleteEvent(ctx, o.Event.ID) }, hecho)
}

// ---- Mover en la rejilla horaria ----

// mover es `moveResizeEvent`: el mismo PUT con el inicio y el fin nuevos, conservando
// todo lo demás del evento. Una serie pregunta el alcance; cancelarlo devuelve el bloque
// a su sitio.
func (a *App) mover(o Ocurrencia, inicio, fin time.Time) {
	p := a.P
	if p.SinConexion || o.SoloLectura {
		p.pendiente = nil
		a.pintar()
		return
	}
	in := o.Event.Input()
	in.Start, in.End = inicio, fin
	fallo := func(err error) {
		p.pendiente = nil
		if err != nil {
			a.avisar(ui.AvisoPeligro, "cal.error.updateEvent")
		}
		a.Recargar()
	}
	if esSerie(o) {
		regla, _ := LeerRRule(o.Event.RRule)
		a.preguntarAlcance(false, func(alcance string) {
			a.escribir(func(ctx context.Context) error {
				return EditarSerie(ctx, a.Cal, o, alcance, in, &regla)
			}, fallo)
		}, func() { fallo(nil) })
		return
	}
	a.escribir(func(ctx context.Context) error {
		_, err := a.Cal.UpdateEvent(ctx, o.Event.ID, in)
		return err
	}, fallo)
}

// Reloj pone en marcha lo que corre solo mientras la app está abierta: la línea de la
// hora actual, que se mueve cada minuto como el `setInterval` de TimeGrid, y el sondeo
// de los recordatorios. Lo arranca el binario, no las pruebas: el cuerpo en pantalla se
// busca en la pantalla en cada tic, así que sobrevive a que la App la reconstruya.
func (a *App) Reloj() {
	go a.sondearRecordatorios()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			a.enUI(func() {
				if c := a.P.cuerpoHorario; c != nil {
					c.Ahora()
				}
			})
		}
	}()
}
