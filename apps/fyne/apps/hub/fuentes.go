// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"sort"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// MaxEventosHoy es el `MAX_EVENTOS` del BFF: la tarjeta «Hoy» enseña seis como mucho.
const MaxEventosHoy = 6

// AgendaDeLaSuite es el `/api/hoy` del BFF de la web (`pages/api/hoy.ts`) hecho desde el
// cliente: aquí no hay BFF, y el token ya lo tiene la sesión transversal.
type AgendaDeLaSuite struct {
	Calendario *api.Calendars
	Correo     *api.Mail
}

var _ Agendable = AgendaDeLaSuite{}

// Hoy son las ocurrencias del día de TODOS los calendarios, por hora de inicio y
// cortadas a seis. Es el `listEvents` de la web: si la lista de calendarios falla, falla
// la agenda; si falla UN calendario, se sigue con los demás (`Promise.allSettled`).
func (a AgendaDeLaSuite) Hoy(ctx context.Context, desde, hasta time.Time) ([]EventoDeHoy, error) {
	calendarios, err := a.Calendario.List(ctx)
	if err != nil {
		return nil, err
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []EventoDeHoy
	)
	for _, c := range calendarios {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			ocurrencias, err := a.Calendario.Instances(ctx, id, desde, hasta)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, o := range ocurrencias {
				out = append(out, EventoDeHoy{
					ID: o.Event.ID, Resumen: o.Event.Summary,
					Inicio: o.Start, Fin: o.End, TodoElDia: o.Event.AllDay,
				})
			}
		}(c.ID)
	}
	wg.Wait()
	return RecortarHoy(out), nil
}

// RecortarHoy ordena por inicio y se queda con los seis primeros, que es lo que hace el
// BFF ANTES de que el lanzador ponga delante los de todo el día (`OrdenarEventos`).
func RecortarHoy(eventos []EventoDeHoy) []EventoDeHoy {
	sort.SliceStable(eventos, func(i, j int) bool { return eventos[i].Inicio.Before(eventos[j].Inicio) })
	return eventos[:min(len(eventos), MaxEventosHoy)]
}

// SinLeer son los no leídos de la bandeja de entrada o, si no hay buzón con ese rol, los
// de todos: el `sinLeer` del BFF.
func (a AgendaDeLaSuite) SinLeer(ctx context.Context) (int, error) {
	buzones, err := a.Correo.Mailboxes(ctx)
	if err != nil {
		return 0, err
	}
	return SinLeerDe(buzones), nil
}

// SinLeerDe es la cuenta de SinLeer sobre una lista ya pedida.
func SinLeerDe(buzones []api.Mailbox) int {
	return api.MailInboxUnread(buzones)
}
