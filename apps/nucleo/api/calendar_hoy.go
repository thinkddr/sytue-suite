// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
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

// Hoy son las ocurrencias entre `desde` y `hasta` de TODOS los calendarios, por hora de
// inicio y cortadas a MaxEventosHoy. Es el `listEvents` de la web: si la lista de
// calendarios falla, falla la agenda; si falla UN calendario, se sigue con los demás
// (`Promise.allSettled`).
func (c *Calendars) Hoy(ctx context.Context, desde, hasta time.Time) ([]EventoDeHoy, error) {
	calendarios, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	// Cada calendario en su hueco: el orden de llegada no cambia el resultado.
	porCalendario := make([][]EventoDeHoy, len(calendarios))
	var wg sync.WaitGroup
	for i, cal := range calendarios {
		wg.Go(func() {
			ocurrencias, err := c.Instances(ctx, cal.ID, desde, hasta)
			if err != nil {
				return
			}
			eventos := make([]EventoDeHoy, 0, len(ocurrencias))
			for _, o := range ocurrencias {
				eventos = append(eventos, EventoDeHoy{
					ID: o.Event.ID, Resumen: o.Event.Summary,
					Inicio: o.Start, Fin: o.End, TodoElDia: o.Event.AllDay,
				})
			}
			porCalendario[i] = eventos
		})
	}
	wg.Wait()
	var out []EventoDeHoy
	for _, eventos := range porCalendario {
		out = append(out, eventos...)
	}
	return RecortarHoy(out), nil
}

// RecortarHoy ordena por inicio y se queda con los seis primeros, que es lo que hace el
// BFF ANTES de que el lanzador ponga delante los de todo el día.
func RecortarHoy(eventos []EventoDeHoy) []EventoDeHoy {
	sort.SliceStable(eventos, func(i, j int) bool { return eventos[i].Inicio.Before(eventos[j].Inicio) })
	return eventos[:min(len(eventos), MaxEventosHoy)]
}
