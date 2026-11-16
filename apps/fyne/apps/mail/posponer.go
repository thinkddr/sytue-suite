// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Posponer y silenciar una conversación
//
// Es D-MAIL-07 de la web (PosponerMenu.tsx y los manejadores de MailApp): «Posponer»
// saca el hilo de Recibidos hasta una fecha y el servicio lo devuelve solo, sin leer;
// «Silenciar conversación» archiva lo que tiene en Recibidos y manda al Archivo sus
// respuestas nuevas. En «Pospuestos» cada fila dice cuándo VUELVE, con el reloj. Lo que
// vuelve SOLO al vencer sube arriba de Recibidos, como en Gmail: el servicio lo ordena
// por la hora de vuelta (`unsnoozed_at`) y la fila la enseña con el mismo reloj
// (`Mensaje.Vuelto`). Lo que se trae a mano («Volver a Recibidos», «Deshacer») vuelve
// con su fecha de llegada, a su sitio: quien lo trae ya lo está mirando.
//
// Es del servicio: en el modo privado los hilos los guarda cifrados el núcleo y nadie
// los despertaría, así que ahí no se ofrece nada (`MailPospone`).

// Posponedor es lo que el servicio sabe hacer para posponer y silenciar. Va aparte de
// Servicio para que lo pregunte quien lo usa (y un servicio de pruebas sin él no
// tenga que fingirlo).
type Posponedor interface {
	MailPospone() bool
	Snoozes(ctx context.Context) ([]api.MailSnooze, error)
	SnoozeThread(ctx context.Context, hilo string, hasta time.Time) (api.MailSnooze, error)
	UnsnoozeThread(ctx context.Context, hilo string) error
	MuteThread(ctx context.Context, hilo string, silenciar bool) (int, error)
	ThreadState(ctx context.Context, hilo string) (api.MailEstadoHilo, error)
}

var _ Posponedor = (*api.Mail)(nil)

// Las horas de las fechas fijas, las de la web.
const (
	// horaManana es a la que «vuelve» lo pospuesto a un día: la de empezar a trabajar.
	horaManana = 8
	// horaTarde es «Más tarde hoy», al acabar la jornada; pasada ultimaHoraParaHoy ya no
	// se ofrece.
	horaTarde         = 18
	ultimaHoraParaHoy = 17
	// MinimoPosponer y MaximoPosponer son lo que acepta el servicio: entre un minuto
	// y un año. Se dicen junto al campo en vez de esperar al rechazo.
	MinimoPosponer = time.Minute
	MaximoPosponer = 366 * 24 * time.Hour
)

// OpcionPosponer es una fecha fija del menú, con la clave de su rótulo.
type OpcionPosponer struct {
	Clave  string
	Cuando time.Time
}

// aLas es `d` a las `hora`:00 en punto, en su zona.
func aLas(d time.Time, hora int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), hora, 0, 0, 0, d.Location())
}

// OpcionesDePosponer es `opcionesDePosponer` de la web: las fechas fijas que tiene
// sentido ofrecer `ahora`, en su hora LOCAL. «Más tarde hoy» solo antes de las cinco;
// «Este fin de semana» solo de lunes a jueves; «La semana que viene» es el lunes
// siguiente, salvo que mañana ya sea lunes. Los días son de calendario (AddDate): el
// cambio de hora no mueve las ocho.
func OpcionesDePosponer(ahora time.Time) []OpcionPosponer {
	var out []OpcionPosponer
	if ahora.Hour() < ultimaHoraParaHoy {
		out = append(out, OpcionPosponer{"snooze.laterToday", aLas(ahora, horaTarde)})
	}
	manana := aLas(ahora.AddDate(0, 0, 1), horaManana)
	out = append(out, OpcionPosponer{"snooze.tomorrow", manana})
	dia := int(ahora.Weekday()) // 0 domingo … 6 sábado
	if dia >= 1 && dia <= 4 {
		out = append(out, OpcionPosponer{"snooze.weekend", aLas(ahora.AddDate(0, 0, 6-dia), horaManana)})
	}
	saltar := (8 - dia) % 7
	if saltar == 0 {
		saltar = 7
	}
	if lunes := aLas(ahora.AddDate(0, 0, saltar), horaManana); !lunes.Equal(manana) {
		out = append(out, OpcionPosponer{"snooze.nextWeek", lunes})
	}
	return out
}

// FechaDeOpcion es la fecha corta junto a cada opción: «mar 17 nov, 8:00».
func FechaDeOpcion(l i18n.Locale, d time.Time) string {
	return i18n.FormatWeekdayDayMonthShort(l, d) + ", " + i18n.FormatTime(l, d)
}

// posponedor es el del servicio, o nil donde no se pospone (el modo privado).
func (b *Buzon) posponedor() Posponedor {
	p, ok := b.S.(Posponedor)
	if !ok || !p.MailPospone() {
		return nil
	}
	return p
}

// ponerPosponer monta Posponer y Silenciar en las acciones de la vista, como
// `onSnooze` y `onMute` de MailApp: posponer desde Recibidos (o cambiar la fecha desde
// «Pospuestos»), silenciar en cualquier carpeta propia, también buscando.
func (b *Buzon) ponerPosponer(a *Acciones) {
	if b.posponedor() == nil || b.compartido != nil {
		return
	}
	if rol := b.rolVista(); b.consulta == "" && (rol == "inbox" || rol == "snoozed") {
		a.Posponer = b.posponer
	}
	a.QuitarPosposicion = b.quitarPosposicion
	a.Silenciar = b.silenciar
}

// posponer es `handleSnooze`: desde Recibidos el hilo sale de la lista y el aviso
// lleva «Deshacer»; desde «Pospuestos» solo cambia la fecha.
func (b *Buzon) posponer(c Conversacion, hasta time.Time) {
	p := b.posponedor()
	if p == nil {
		return
	}
	cambiaFecha := b.rolVista() == "snoozed"
	vista, hilo := b.vista, c.HiloID
	var antes, antesR []api.MailMessage
	if !cambiaFecha {
		antes, antesR = b.quitarDeLaVista([]Conversacion{c})
		b.pintarLista()
	}
	pedir(b, func(ctx context.Context) (api.MailSnooze, error) {
		return p.SnoozeThread(ctx, hilo, hasta)
	}, func(sn api.MailSnooze, err error) {
		if err != nil {
			if !cambiaFecha && vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			clave := "snooze.failed"
			switch {
			case errors.Is(err, api.ErrNadaQuePosponer):
				clave = "snooze.nothing"
			case errors.Is(err, api.ErrSnoozeInvalido):
				clave = "snooze.invalid"
			}
			b.avisar(ui.AvisoPeligro, clave, nil)
			return
		}
		b.refrescarContadores()
		if cambiaFecha {
			if b.vuelven == nil {
				b.vuelven = map[string]time.Time{}
			}
			b.vuelven[hilo] = sn.Until
			if b.hiloID == hilo {
				b.P.Lector.PospuestaHasta = sn.Until
				b.P.Lector.Refresh()
			}
			b.pintarLista()
		}
		t := b.P.T
		a := ui.NuevoAviso(ui.AvisoExito, t.T("snooze.done", i18n.Vars{"date": FechaLarga(b.P.Locale, sn.Until.In(time.Local))}), "")
		a.RotuloCerrar = t.T("ui.dismissNotification", nil)
		if !cambiaFecha {
			a.RotuloAccion = t.T("compose.undo", nil)
			a.AlPulsarAccion = func() { b.deshacerPosponer(hilo) }
		}
		b.P.Notificar(a)
	})
}

// deshacerPosponer es el «Deshacer» del aviso: el hilo vuelve a Recibidos ya.
func (b *Buzon) deshacerPosponer(hilo string) {
	p := b.posponedor()
	if p == nil {
		return
	}
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, p.UnsnoozeThread(ctx, hilo)
	}, func(_ struct{}, err error) {
		if err != nil {
			b.avisar(ui.AvisoPeligro, "snooze.unsnoozeFailed", nil)
			return
		}
		delete(b.vuelven, hilo)
		b.Refrescar()
		b.refrescarContadores()
		b.avisar(ui.AvisoExito, "triage.undone", nil)
	})
}

// quitarPosposicion es «Volver a Recibidos»: desde «Pospuestos» el hilo sale de la
// lista, y vuelve con su fecha de llegada (ya no está en `vuelven`).
func (b *Buzon) quitarPosposicion(c Conversacion) {
	p := b.posponedor()
	if p == nil {
		return
	}
	enPospuestos := b.rolVista() == "snoozed"
	vista, hilo := b.vista, c.HiloID
	var antes, antesR []api.MailMessage
	if enPospuestos {
		antes, antesR = b.quitarDeLaVista([]Conversacion{c})
		b.pintarLista()
	}
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, p.UnsnoozeThread(ctx, hilo)
	}, func(_ struct{}, err error) {
		if err != nil {
			if enPospuestos && vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			b.avisar(ui.AvisoPeligro, "snooze.unsnoozeFailed", nil)
			return
		}
		delete(b.vuelven, hilo)
		if b.hiloID == hilo {
			b.P.Lector.PospuestaHasta = time.Time{}
			b.P.Lector.Refresh()
		}
		b.refrescarContadores()
		b.avisar(ui.AvisoExito, "snooze.unsnoozed", nil)
	})
}

// silenciar es `handleMute`: silenciar desde Recibidos saca el hilo de la lista (lo
// archiva el servicio), y «Deshacer» lo deja como estaba.
func (b *Buzon) silenciar(c Conversacion, silenciar bool) {
	p := b.posponedor()
	if p == nil {
		return
	}
	enEntrada := b.consulta == "" && b.rolVista() == "inbox"
	vista, hilo := b.vista, c.HiloID
	var antes, antesR []api.MailMessage
	if silenciar && enEntrada {
		antes, antesR = b.quitarDeLaVista([]Conversacion{c})
		b.pintarLista()
	}
	entrada, hayEntrada := b.carpetaDeRol("inbox")
	var ids []string
	if hayEntrada {
		ids = idsEnCarpeta(c, entrada.ID, b.carpetaVista())
	}
	pedir(b, func(ctx context.Context) (int, error) {
		return p.MuteThread(ctx, hilo, silenciar)
	}, func(archivados int, err error) {
		if err != nil {
			if silenciar && enEntrada && vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			b.avisar(ui.AvisoPeligro, "mute.failed", nil)
			return
		}
		b.refrescarContadores()
		if b.hiloID == hilo {
			b.P.Lector.Silenciada = silenciar
			b.P.Lector.Refresh()
		}
		t := b.P.T
		clave := "mute.undone"
		if silenciar {
			clave = "mute.done"
		}
		a := ui.NuevoAviso(ui.AvisoExito, t.T(clave, nil), "")
		a.RotuloCerrar = t.T("ui.dismissNotification", nil)
		if silenciar {
			a.RotuloAccion = t.T("compose.undo", nil)
			a.AlPulsarAccion = func() { b.deshacerSilenciar(hilo, archivados > 0 && hayEntrada, ids, entrada.ID) }
		}
		b.P.Notificar(a)
	})
}

// deshacerSilenciar deja de silenciar y devuelve a Recibidos SOLO lo que estaba en
// Recibidos (`idsDeLaEntrada` de la web): ni las respuestas enviadas ni lo archivado
// antes.
func (b *Buzon) deshacerSilenciar(hilo string, devolver bool, ids []string, entrada string) {
	p := b.posponedor()
	if p == nil {
		return
	}
	pedir(b, func(ctx context.Context) (struct{}, error) {
		if _, err := p.MuteThread(ctx, hilo, false); err != nil {
			return struct{}{}, err
		}
		if devolver && len(ids) > 0 {
			return struct{}{}, b.S.MoveMany(ctx, ids, entrada)
		}
		return struct{}{}, nil
	}, func(_ struct{}, err error) {
		if err != nil {
			b.avisar(ui.AvisoPeligro, "mute.failed", nil)
			return
		}
		if b.hiloID == hilo {
			b.P.Lector.Silenciada = false
		}
		b.Refrescar()
		b.refrescarContadores()
	})
}

// idsEnCarpeta son los mensajes del hilo que están en `carpeta`; los que no dicen la
// suya están en la que se mira (`vista`).
func idsEnCarpeta(c Conversacion, carpeta, vista string) []string {
	var ids []string
	for _, m := range c.Mensajes {
		donde := m.CarpetaID
		if donde == "" {
			donde = vista
		}
		if donde == carpeta {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// cargarVuelven pide cuándo vuelve cada hilo de «Pospuestos»; fuera de ella no hay
// nada que pedir, y la lista pinta la fecha de llegada.
func (b *Buzon) cargarVuelven() {
	b.vuelven = nil
	p := b.posponedor()
	if p == nil || b.consulta != "" || b.rolVista() != "snoozed" {
		return
	}
	vista := b.vista
	pedir(b, func(ctx context.Context) ([]api.MailSnooze, error) {
		return p.Snoozes(ctx)
	}, func(l []api.MailSnooze, err error) {
		if err != nil || vista != b.vista {
			return
		}
		b.vuelven = make(map[string]time.Time, len(l))
		for _, sn := range l {
			b.vuelven[sn.ThreadID] = sn.Until
		}
		b.pintarLista()
	})
}

// cargarEstadoHilo pide si el hilo abierto está silenciado o pospuesto: el lector lo
// dice en «Más acciones» y en el rótulo de «Posponer».
func (b *Buzon) cargarEstadoHilo(hilo string) {
	b.P.Lector.Silenciada, b.P.Lector.PospuestaHasta = false, time.Time{}
	p := b.posponedor()
	if p == nil {
		return
	}
	pedir(b, func(ctx context.Context) (api.MailEstadoHilo, error) {
		return p.ThreadState(ctx, hilo)
	}, func(st api.MailEstadoHilo, err error) {
		if err != nil || hilo != b.hiloID {
			return
		}
		b.P.Lector.Silenciada = st.Muted
		if st.SnoozedUntil != nil {
			b.P.Lector.PospuestaHasta = *st.SnoozedUntil
		}
		b.P.Lector.Refresh()
	})
}

// botonPosponer es el `PosponerMenu` del lector: el reloj, que abre las fechas fijas
// (calculadas al ABRIR: un lector abierto desde la mañana no ofrece por la noche «Más
// tarde hoy») y «Elegir fecha y hora…». Con el hilo ya pospuesto, «Cambiar la fecha».
func (x *Lector) botonPosponer() *ui.Boton {
	t, a := x.T, x.Acciones
	clave := "snooze.action"
	if !x.PospuestaHasta.IsZero() {
		clave = "snooze.change"
	}
	var b *ui.Boton
	b = botonIconoCabecera(t.T(clave, nil), RelojD, "", false, func() {
		ahora := time.Now()
		if x.Ahora != nil {
			ahora = x.Ahora()
		}
		opciones := OpcionesDePosponer(ahora.In(time.Local))
		els := make([]fyne.CanvasObject, 0, len(opciones)+1)
		for _, o := range opciones {
			e := ui.NuevoElementoMenu(t.T(o.Clave, nil)+" · "+FechaDeOpcion(x.Locale, o.Cuando), iconos.Ninguno,
				func() { a.Posponer(x.Conversacion, o.Cuando) })
			els = append(els, e)
		}
		els = append(els, ui.NuevoElementoMenu(t.T("snooze.custom", nil), iconos.Ninguno, x.elegirFechaDePosponer))
		m := ui.NuevoMenu(els...)
		m.Rotulo = t.T(clave, nil)
		m.Mostrar(b)
	})
	return b
}

// elementosPosponer son, en «Más acciones», lo que en la web son las franjas del lector
// («Volver a Recibidos», «Dejar de silenciar») y «Silenciar conversación».
func (x *Lector) elementosPosponer() []fyne.CanvasObject {
	t, a := x.T, x.Acciones
	var els []fyne.CanvasObject
	if a.QuitarPosposicion != nil && !x.PospuestaHasta.IsZero() {
		els = append(els, elementoGlifo(t.T("snooze.unsnooze", nil), SinLeerD, false,
			func() { a.QuitarPosposicion(x.Conversacion) }))
	}
	if a.Silenciar != nil {
		clave := "mute.action"
		if x.Silenciada {
			clave = "mute.unmute"
		}
		silenciar := !x.Silenciada
		els = append(els, elementoGlifo(t.T(clave, nil), SilenciarD, false,
			func() { a.Silenciar(x.Conversacion, silenciar) }))
	}
	return els
}

// elegirFechaDePosponer es el diálogo «Posponer hasta»: fecha y hora, validadas junto
// al campo con lo mismo que exige el servicio.
func (x *Lector) elegirFechaDePosponer() {
	c := fyne.CurrentApp().Driver().CanvasForObject(x)
	if c == nil || x.Acciones == nil || x.Acciones.Posponer == nil {
		return
	}
	t := x.T
	campo := nuevoCampoCuando(aLas(time.Now().AddDate(0, 0, 1), horaManana))
	grupo := func(err string) fyne.CanvasObject {
		return ui.GrupoCampo(t.T("snooze.dateTime", nil), false, campo.objeto(), "", err)
	}
	cuerpo := container.New(ui.Columna{}, grupo(""))
	conv := x.Conversacion
	var d *ui.Dialogo
	d = ui.Confirmacion(t.T("snooze.customTitle", nil), t.T("common.cancel", nil), t.T("snooze.confirm", nil),
		false, cuerpo, nil, func() {
			cuando, ok := campo.leer()
			if !ok || !FechaDePosponerValida(cuando, time.Now()) {
				cuerpo.Objects = []fyne.CanvasObject{grupo(t.T("snooze.invalid", nil))}
				cuerpo.Refresh()
				d.Recolocar()
				return
			}
			d.Cerrar()
			x.Acciones.Posponer(conv, cuando)
		})
	d.RotuloCerrar = t.T("ui.close", nil)
	d.Mostrar(c)
}

// FechaDePosponerValida es lo que exige el servicio: entre un minuto y un año.
func FechaDePosponerValida(cuando, ahora time.Time) bool {
	d := cuando.Sub(ahora)
	return d >= MinimoPosponer && d <= MaximoPosponer
}
