// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// maxRecientes es lo que el Hub pide de la actividad reciente.
const maxRecientes = 8

// ProductosDeHub son los productos cuya señal de cambios mueve el widget de One: Mail y
// Calendar por sus contadores y la agenda, y los que llenan la actividad reciente del Hub
// (`/api/v1/recent`: Bases, ERP, Notes y Drive, donde viven los ficheros de Docus,
// Calcus, Diapos, certiDocs y Photos). Contacts, Rooms y Cast no dan actividad reciente.
// Con la app abierta, One republica con ellos; con la app cerrada, el Hub la despierta
// con los mismos (services/hub/internal/despertar, `deOne`, que una prueba ata a esta
// lista).
var ProductosDeHub = []string{"mail", "calendar", "bases", "drive", "erp", "notes", "docs", "sheets", "slides", "pdf", "photos"}

// Hub (One) junta lo de hoy de la suite: el correo sin leer, los eventos de hoy de
// todos los calendarios y la actividad reciente. La agenda es api.Calendars.Hoy, la
// MISMA que lee la tarjeta «Hoy» (seis como mucho, un calendario caído no la tumba), y
// el correo, api.MailInboxUnread. Cada parte falla por su lado, como en la tarjeta;
// solo si fallan las tres se conserva lo publicado.
func Hub(c Comunes, hub *api.Hub, cal *api.Calendars, correo *api.Mail) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		var (
			lecturas                sync.WaitGroup
			buzones                 []api.Mailbox
			errCorreo, errRecientes error
			recientes               api.HubBusqueda
		)
		lecturas.Go(func() { buzones, errCorreo = correo.Mailboxes(ctx) })
		lecturas.Go(func() { recientes, errRecientes = hub.RecentDe(ctx, nil, maxRecientes) })
		ahora := c.ahora()
		desde, hasta := api.LimitesDelDia(ahora)
		eventos, errEventos := cal.Hoy(ctx, desde, hasta)
		lecturas.Wait()
		if err := todoFalla(errCorreo, errEventos, errRecientes); err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		var agenda, reciente []widgets.Item
		if errCorreo == nil {
			d.Counters = append(d.Counters, widgets.Counter{Kind: "unread", Value: int64(max(api.MailInboxUnread(buzones), 0))})
		}
		if errEventos == nil {
			d.Counters = append(d.Counters, widgets.Counter{Kind: "events", Value: int64(len(eventos))})
			agenda = agendaDeHoy(eventos, ahora)
		}
		if errRecientes == nil {
			reciente = actividadReciente(recientes.Hits)
		}
		d.Items = widgets.SanitizeItems(append(agenda, reciente...))
		return d, nil
	}
}

// agendaDeHoy: la agenda ya viene ordenada y cortada (api.RecortarHoy); los ya
// terminados siguen, sin marcar, como en la tarjeta (api.EventoDeHoy.HaTerminado).
func agendaDeHoy(eventos []api.EventoDeHoy, ahora time.Time) []widgets.Item {
	out := make([]widgets.Item, 0, len(eventos))
	for _, e := range eventos {
		titulo := strings.TrimSpace(e.Resumen)
		if titulo == "" {
			continue
		}
		badge := e.Inicio.In(ahora.Location()).Format("15:04")
		if e.TodoElDia {
			badge = "HOY"
		}
		out = append(out, widgets.Item{
			Title: titulo, Badge: badge,
			StartAt: unix(e.Inicio), EndAt: unix(e.Fin),
			Unread: !e.HaTerminado(ahora),
		})
	}
	return out
}

func actividadReciente(hits []api.HubHit) []widgets.Item {
	out := make([]widgets.Item, 0, min(len(hits), widgets.MaxItems))
	for _, h := range hits {
		titulo := strings.TrimSpace(h.Title)
		if titulo == "" {
			continue
		}
		var cuando int64
		if h.Timestamp != nil {
			cuando = unix(*h.Timestamp)
		}
		out = append(out, widgets.Item{
			Title: titulo, Subtitle: h.Kind, Badge: strings.ToUpper(h.Source), StartAt: cuando,
		})
		if llenos(out) {
			break
		}
	}
	return out
}
