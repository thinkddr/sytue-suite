// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"log/slog"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
)

// Los recordatorios de los eventos los entrega el SISTEMA a su hora (PLAN §5.12): en
// la edición conectada los manda el servidor, y aquí no lo hay. Al abrir, y cada cuarto
// de hora con la app abierta, se programan los de los próximos 30 días —también los de
// cada repetición de una serie— con `local.App.Recordar`.
//
// Las horas salen de los recordatorios del propio evento (sus VALARM, en minutos antes
// del inicio, como los lee `GetEvent`): las mismas que la conectada pasa por
// `calendar/alarm.Triggers`.

const cadaRecordar = 15 * time.Minute

func recordar(ctx context.Context, l *local.App, repo *privateCalendar.Repository) {
	programar := func() {
		lista, err := avisosDeEventos(ctx, repo, time.Now())
		if err == nil {
			err = l.Recordar("calendar", lista)
		}
		if err != nil {
			slog.Warn("calendar: no se pudieron programar los recordatorios", "error", err)
		}
	}
	go func() {
		programar()
		t := time.NewTicker(cadaRecordar)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				programar()
			}
		}
	}()
}

// avisosDeEventos son los recordatorios de todas las ocurrencias de los próximos 30 días.
func avisosDeEventos(ctx context.Context, repo *privateCalendar.Repository, ahora time.Time) ([]avisos.Aviso, error) {
	cals, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	hasta := ahora.Add(avisos.Ventana)
	recordatorios := map[string][]int{} // por evento: GetEvent cuesta, una vez por evento
	var out []avisos.Aviso
	for _, c := range cals {
		inst, err := repo.Instances(ctx, c.ID, ahora, hasta.Add(7*24*time.Hour))
		if err != nil {
			return nil, err
		}
		for _, i := range inst {
			minutos, ok := recordatorios[i.Event.ID]
			if !ok {
				if ev, err := repo.GetEvent(ctx, i.Event.ID); err == nil && ev.Reminders != nil {
					minutos = *ev.Reminders
				}
				recordatorios[i.Event.ID] = minutos
			}
			for _, m := range minutos {
				out = append(out, avisoDe(i, m))
			}
		}
	}
	return out, nil
}

// avisoDe es el mismo que avisa el Hub con la app cerrada (avisos.DeEvento).
func avisoDe(i api.Instance, minutos int) avisos.Aviso {
	return avisos.DeEvento(i.Event.ID, i.Event.Summary, i.Event.Location, i.Start, i.Event.AllDay, minutos)
}

// irAlEvento enseña el día del evento que llega por enlace (un aviso pulsado, el Hub).
func irAlEvento(ctx context.Context, repo *privateCalendar.Repository, ir func(time.Time)) func(clase, id string) {
	return func(_, id string) {
		go func() {
			ev, err := repo.GetEvent(ctx, id)
			if err != nil {
				return
			}
			fyne.Do(func() { ir(ev.DTStart) })
		}()
	}
}
