// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo que acompaña a un envío y lo que pasa después con él: las @menciones que viajan,
// programar y reprogramar, y el acuse de recibo. Son `sendBody`, el `sendAt` de
// `handleSend`, `rescheduleMsg` y `handleAck` de la web.

// prioridadAPI es el `priority` de cada una; la normal no viaja.
var prioridadAPI = [...]string{Normal: "", Importante: "important", Urgente: "urgent"}

// MencionesEn es el `mentions` de `sendBody`: los `sub` de las personas cuyo nombre
// sale LITERAL como «@Nombre» en el texto, sin repetir. @channel, @here y @all no
// viajan aquí: el servicio los saca del propio cuerpo.
func MencionesEn(texto string, roster []Miembro) []string {
	var out []string
	for _, m := range roster {
		if m.Nombre != "" && strings.Contains(texto, "@"+m.Nombre) && !slices.Contains(out, m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

// programar es la rama `sendAt` de `handleSend`: no entra en el historial —lo publica
// el servicio al vencer y llega por el tiempo real—, se confirma con un aviso que dice
// cuándo y, si el panel «Programados» está abierto, se ve ya. Si falla, el texto vuelve
// a la caja con su hora, para no perder ni una cosa ni la otra.
func (sa *Sala) programar(canal, texto string, adjuntos []string, cuando time.Time) {
	t := sa.P.T
	pedir(sa, func(ctx context.Context) (api.RoomsScheduled, error) {
		return sa.S.Schedule(ctx, canal, texto, adjuntos, cuando)
	}, func(p api.RoomsScheduled, err error) {
		if err != nil {
			sa.P.Avisar(ui.AvisoPeligro, t.T("schedule.failed", nil))
			if c := sa.P.Canal.Compositor; canal == sa.abierto && c.Texto() == "" {
				c.PonerTexto(texto)
				c.PonerProgramado(cuando)
			}
			return
		}
		sa.P.Avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, t.T("schedule.done", nil),
			t.T("schedule.doneDetail", i18n.Vars{"when": i18n.FormatDateTimeLong(sa.P.Locale, p.SendAt.Local())})))
		if sa.panel == PanelProgramado {
			sa.cargarProgramados()
		}
	})
}

// Reprogramar es `rescheduleMsg`: la fila se mueve ya a su nueva hora y, si el servicio
// dice que no, la lista se vuelve a pedir. Una hora que ya ha pasado no se manda: el
// `min` del control de la web no la deja elegir.
func (sa *Sala) Reprogramar(m Programado, cuando time.Time) {
	if !cuando.After(sa.ahora()) {
		sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("schedule.failed", nil))
		return
	}
	if l := sa.lista; l != nil && sa.panel == PanelProgramado {
		for i := range l.Programados {
			if l.Programados[i].ID == m.ID {
				l.Programados[i].Cuando = cuando
			}
		}
		slices.SortStableFunc(l.Programados, func(a, b Programado) int { return a.Cuando.Compare(b.Cuando) })
		l.Refresh()
	}
	pedir(sa, func(ctx context.Context) (api.RoomsScheduled, error) { return sa.S.Reschedule(ctx, m.ID, cuando) },
		func(_ api.RoomsScheduled, err error) {
			if err != nil && sa.panel == PanelProgramado {
				sa.cargarProgramados()
			}
		})
}

// Acusar es `handleAck`: se marca ya —confirmado y un acuse más— y se ajusta al total
// que dé el servicio; si falla, se deshace. Uno ya confirmado no se vuelve a mandar.
func (sa *Sala) Acusar(m Mensaje) {
	if m.Acusado {
		return
	}
	aplicar := func(f func(*api.RoomsMessage)) {
		if i := slices.IndexFunc(sa.mensajes, func(x api.RoomsMessage) bool { return x.ID == m.ID }); i >= 0 {
			f(&sa.mensajes[i])
			sa.pintarMensajes()
			sa.P.Canal.Historial.Refresh()
		}
	}
	aplicar(func(x *api.RoomsMessage) { x.Acked, x.AckCount = true, x.AckCount+1 })
	pedir(sa, func(ctx context.Context) (int, error) { return sa.S.Ack(ctx, m.ID) }, func(n int, err error) {
		if err != nil {
			aplicar(func(x *api.RoomsMessage) { x.Acked, x.AckCount = false, max(0, x.AckCount-1) })
			return
		}
		aplicar(func(x *api.RoomsMessage) { x.Acked, x.AckCount = true, n })
	})
}
