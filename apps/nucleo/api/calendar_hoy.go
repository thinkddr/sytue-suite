// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// # La agenda de hoy de la suite
//
// Es el `/api/hoy` del BFF de la web (`pages/api/hoy.ts`) hecho desde el cliente. Lo
// leen DOS sitios que tienen que decir lo mismo: la tarjeta «Hoy» del Hub
// (apps/fyne/apps/hub) y el widget nativo del Hub (apps/nucleo/widgets/fuentes). Por
// eso vive aquí y no en ninguno de los dos (CONVENTIONS §13.2).

// MaxEventosHoy es el `MAX_EVENTOS` del BFF: la tarjeta «Hoy» enseña seis como mucho.
const MaxEventosHoy = 6

// EventoDeHoy es lo que la tarjeta «Hoy» enseña de un evento: lo mismo que `resumir`
// deja en el BFF de la web.
type EventoDeHoy struct {
	ID        string
	Resumen   string
	Inicio    time.Time
	Fin       time.Time
	TodoElDia bool
	// Lugar es el `location` del evento: la tarjeta lo pinta, apagado, tras el resumen.
	Lugar string
}

// HaTerminado es el `haTerminado` de la web: un evento ya pasado se apaga en la
// lista, pero no desaparece.
func (e EventoDeHoy) HaTerminado(ahora time.Time) bool {
	return !e.TodoElDia && !e.Fin.IsZero() && e.Fin.Before(ahora)
}

// LimitesDelDia es el `limitesDelDia` de `lib/hoy.ts`: de la medianoche local a la
// siguiente. Los pone el CLIENTE y no el servidor, porque es quien sabe en qué zona
// horaria vive la persona.
func LimitesDelDia(ahora time.Time) (desde, hasta time.Time) {
	a, m, d := ahora.Date()
	desde = time.Date(a, m, d, 0, 0, 0, 0, ahora.Location())
	return desde, desde.AddDate(0, 0, 1)
}

// EstadoDeEvento es el `estadoDelEvento` de `lib/hoy.ts`. El valor cero es «ninguno»:
// un evento de todo el día, o uno empezado sin fin, que no se sabe si sigue.
type EstadoDeEvento uint8

// Los estados de un evento respecto a la hora actual.
const (
	EventoSinEstado EstadoDeEvento = iota
	EventoPasado
	EventoAhora
	EventoSiguiente
)

// Estado dice si el evento ya terminó, está en curso o falta por empezar. El instante
// exacto del inicio ya cuenta como en curso.
func (e EventoDeHoy) Estado(ahora time.Time) EstadoDeEvento {
	switch {
	case e.TodoElDia || e.Inicio.IsZero():
		return EventoSinEstado
	case e.Inicio.After(ahora):
		return EventoSiguiente
	case e.HaTerminado(ahora):
		return EventoPasado
	case e.Fin.IsZero():
		return EventoSinEstado
	default:
		return EventoAhora
	}
}

// MarcasDeLaAgenda es el `marcasDeLaAgenda` de la web: «Ahora» en los eventos en curso
// y «Siguiente» SOLO en el primero que falta por empezar, en el orden de la lista. El
// resto, EventoSinEstado.
func MarcasDeLaAgenda(eventos []EventoDeHoy, ahora time.Time) []EstadoDeEvento {
	out := make([]EstadoDeEvento, len(eventos))
	haySiguiente := false
	for i, e := range eventos {
		switch e.Estado(ahora) {
		case EventoAhora:
			out[i] = EventoAhora
		case EventoSiguiente:
			if !haySiguiente {
				out[i], haySiguiente = EventoSiguiente, true
			}
		case EventoSinEstado, EventoPasado:
			// Sin marca: ni lo de todo el día ni lo ya terminado se señala.
		}
	}
	return out
}

// AgendaDeHoy es lo que el BFF de la web devuelve de la agenda: los primeros
// MaxEventosHoy, cuántos tiene el día (`total`, el «+N más») y si falta algún
// calendario que no contestó (`parcial`).
type AgendaDeHoy struct {
	Eventos []EventoDeHoy
	Total   int
	Parcial bool
}

// ErrAgendaSinCalendarios: no contestó NINGUNO de los calendarios. No es un día vacío,
// es una agenda que no se puede enseñar (la web devuelve `eventos: null`).
var ErrAgendaSinCalendarios = errors.New("ningún calendario ha contestado")

// Hoy son los eventos de AgendaCompleta: lo que lee el widget del Hub, que no pinta ni
// el «+N» ni el aviso de agenda incompleta.
func (c *Calendars) Hoy(ctx context.Context, desde, hasta time.Time) ([]EventoDeHoy, error) {
	a, err := c.AgendaCompleta(ctx, desde, hasta)
	return a.Eventos, err
}

// AgendaCompleta son las ocurrencias entre `desde` y `hasta` de TODOS los calendarios,
// por hora de inicio y cortadas a MaxEventosHoy. Es el `listEventsDetallado` de la web:
// si la lista de calendarios falla, falla la agenda; si falla UN calendario, se sigue con
// los demás y la agenda sale `Parcial`; si fallan todos, ErrAgendaSinCalendarios.
func (c *Calendars) AgendaCompleta(ctx context.Context, desde, hasta time.Time) (AgendaDeHoy, error) {
	calendarios, err := c.List(ctx)
	if err != nil {
		return AgendaDeHoy{}, err
	}
	// Cada calendario en su hueco: el orden de llegada no cambia el resultado.
	porCalendario := make([][]EventoDeHoy, len(calendarios))
	fallidos := make([]bool, len(calendarios))
	var wg sync.WaitGroup
	for i, cal := range calendarios {
		wg.Go(func() {
			ocurrencias, err := c.Instances(ctx, cal.ID, desde, hasta)
			if err != nil {
				// Antes se tiraba aquí sin decirlo: la agenda parecía un día tranquilo.
				fallidos[i] = true
				return
			}
			eventos := make([]EventoDeHoy, 0, len(ocurrencias))
			for _, o := range ocurrencias {
				eventos = append(eventos, EventoDeHoy{
					ID: o.Event.ID, Resumen: o.Event.Summary,
					Inicio: o.Start, Fin: o.End, TodoElDia: o.Event.AllDay,
					Lugar: o.Event.Location,
				})
			}
			porCalendario[i] = eventos
		})
	}
	wg.Wait()
	var out []EventoDeHoy
	caidos := 0
	for i, eventos := range porCalendario {
		if fallidos[i] {
			caidos++
		}
		out = append(out, eventos...)
	}
	if len(calendarios) > 0 && caidos == len(calendarios) {
		return AgendaDeHoy{}, ErrAgendaSinCalendarios
	}
	eventos, total := RecortarHoy(out)
	return AgendaDeHoy{Eventos: eventos, Total: total, Parcial: caidos > 0}, nil
}

// RecortarHoy ordena por inicio y se queda con los seis primeros, que es lo que hace el
// BFF ANTES de que el lanzador ponga delante los de todo el día. Devuelve también
// cuántos había, que es el «+N más en el calendario» de la tarjeta.
func RecortarHoy(eventos []EventoDeHoy) ([]EventoDeHoy, int) {
	sort.SliceStable(eventos, func(i, j int) bool { return eventos[i].Inicio.Before(eventos[j].Inicio) })
	return eventos[:min(len(eventos), MaxEventosHoy)], len(eventos)
}
