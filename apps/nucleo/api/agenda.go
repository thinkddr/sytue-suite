// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// La agenda del día y el evento que nace de una nota en Sytue Notepads: el
// `/api/agenda` del BFF de la web (apps/frontend/apps/notes/src/pages/api/agenda.ts),
// con la misma sesión de la persona.

// EventoAgenda es un evento del día, ya con su enlace a Sytue Calendar.
type EventoAgenda struct {
	ID     string
	Nombre string
	URL    string
	// Ruta es la del evento dentro de Calendar (`/events/<id>`), para abrirlo en la app.
	Ruta        string
	Inicio      time.Time
	DiaCompleto bool
}

// ErrSinCalendario: la persona no tiene ningún calendario en el que escribir.
var ErrSinCalendario = errors.New("no hay calendario en el que crear el evento")

// Agenda son los eventos del día civil de `dia` (en su zona), de todos los
// calendarios, por hora. Un calendario que falle no se lleva a los demás. Los de día
// completo son FLOTANTES (00:00 UTC) y se eligen por su fecha, no por la ventana local.
func (e Elegir) Agenda(ctx context.Context, dia time.Time) ([]EventoAgenda, error) {
	if e.Calendar == nil {
		return nil, nil
	}
	desde := time.Date(dia.Year(), dia.Month(), dia.Day(), 0, 0, 0, 0, dia.Location())
	fecha := desde.Format(time.DateOnly)
	cals, err := e.Calendar.List(ctx)
	if err != nil {
		return nil, err
	}
	tandas := make([][]Instance, len(cals))
	var wg sync.WaitGroup
	for i, c := range cals {
		wg.Go(func() { tandas[i], _ = e.Calendar.Instances(ctx, c.ID, desde, desde.AddDate(0, 0, 1)) })
	}
	wg.Wait()
	var out []EventoAgenda
	for _, tanda := range tandas {
		for _, i := range tanda {
			if i.Event.AllDay && i.Start.UTC().Format(time.DateOnly) != fecha {
				continue
			}
			ruta := "/events/" + url.PathEscape(i.Event.ID)
			out = append(out, EventoAgenda{
				ID: i.Event.UID, Nombre: i.Event.Summary, Inicio: i.Start, DiaCompleto: i.Event.AllDay,
				Ruta: ruta, URL: AppURL(e.Apex, "calendar", e.Organizacion(), ruta),
			})
		}
	}
	slices.SortStableFunc(out, func(a, b EventoAgenda) int { return a.Inicio.Compare(b.Inicio) })
	return out, nil
}

// CrearEventoDelDia crea un evento de día completo el día `fecha` (AAAA-MM-DD) en el
// calendario por defecto y devuelve el enlace `calendar_event` que la nota guarda.
func (e Elegir) CrearEventoDelDia(ctx context.Context, fecha, resumen string) (NoteLink, error) {
	d, err := time.Parse(time.DateOnly, fecha)
	resumen = strings.TrimSpace(resumen)
	if err != nil || resumen == "" || e.Calendar == nil {
		return NoteLink{}, ErrSinCalendario
	}
	cals, err := e.Calendar.List(ctx)
	if err != nil {
		return NoteLink{}, err
	}
	i := slices.IndexFunc(cals, func(c Calendar) bool { return c.IsDefault })
	if i < 0 {
		i = slices.IndexFunc(cals, func(c Calendar) bool { return !c.ReadOnly })
	}
	if i < 0 {
		return NoteLink{}, ErrSinCalendario
	}
	ev, err := e.Calendar.CreateEvent(ctx, cals[i].ID, EventInput{
		Summary: resumen, Start: d, End: d.AddDate(0, 0, 1), AllDay: true,
	})
	if err != nil {
		return NoteLink{}, err
	}
	return NoteLink{
		TargetKind: "calendar_event", TargetID: ev.UID, Label: ev.Summary,
		URL: AppURL(e.Apex, "calendar", e.Organizacion(), "/events/"+url.PathEscape(ev.ID)),
	}, nil
}
