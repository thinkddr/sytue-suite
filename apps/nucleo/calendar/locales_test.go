// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendar_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
)

const invitacionDePrueba = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:reunion@prueba\r\nDTSTAMP:20260101T000000Z\r\nDTSTART:20260310T090000Z\r\nDTEND:20260310T100000Z\r\nSUMMARY:Revisión trimestral\r\n" +
	"ORGANIZER:mailto:jefa@ejemplo.org\r\nATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:yo@ejemplo.org\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// Sin Correo ni Descargar todo sigue fallando cerrado; con ellos, contestar por UID,
// buscar con filtro y suscribirse a una URL funcionan en el dispositivo (A-02, A-03).
func TestRutasLocalesDeCalendar(t *testing.T) {
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	ctx := t.Context()
	cal, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ics := range []string{icsDePrueba, invitacionDePrueba} {
		if _, err := repo.ImportICS(ctx, cal.ID, []byte(ics)); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ResponderPorUID(ctx, "reunion@prueba", api.AsistenciaSi); !errors.Is(err, calendar.ErrPrivateIngress) {
		t.Fatal("sin correo no se contesta", err)
	}
	if _, err := repo.Subscribe(ctx, api.SubscriptionInput{URL: "https://ejemplo.org/f.ics"}); !errors.Is(err, calendar.ErrPrivateSubscriptions) {
		t.Fatal("sin Descargar no hay suscripciones", err)
	}

	repo.Correo = "Yo@Ejemplo.org"
	svc, err := api.NewCalendars("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ResponderPorUID(ctx, "reunion@prueba", api.AsistenciaSi); err != nil {
		t.Fatal("contestar", err)
	}
	hits, err := svc.SearchEventsFiltrada(ctx, "revision", api.FiltroBusqueda{Calendario: cal.ID, Desde: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Hasta: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}, 10, "")
	if err != nil || len(hits.Events) != 1 {
		t.Fatal("búsqueda filtrada", hits, err)
	}
	ev, err := repo.GetEvent(ctx, hits.Events[0].ID)
	if err != nil || len(ev.Attendees) != 1 || ev.Attendees[0].PartStat != api.AsistenciaSi {
		t.Fatal("la respuesta queda en el evento", ev, err)
	}
	if fuera, _ := svc.SearchEventsFiltrada(ctx, "revision", api.FiltroBusqueda{Desde: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}, 10, ""); len(fuera.Events) != 0 {
		t.Fatal("fuera del tramo no sale", fuera)
	}
	// La serie de enero (COUNT=4) alcanza el tramo de enero.
	if serie, _ := svc.SearchEventsFiltrada(ctx, "semanal", api.FiltroBusqueda{Desde: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), Hasta: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)}, 10, ""); len(serie.Events) != 1 {
		t.Fatal("la serie que alcanza el tramo sale", serie)
	}

	feed := icsDePrueba
	pedidas := 0
	repo.Descargar = func(_ context.Context, u string) ([]byte, error) {
		pedidas++
		if u != "https://ejemplo.org/festivos.ics" {
			t.Errorf("URL inesperada %q", u)
		}
		return []byte(feed), nil
	}
	sub, err := svc.Subscribe(ctx, api.SubscriptionInput{URL: "webcal://ejemplo.org/festivos.ics", DisplayName: "Festivos"})
	if err != nil || !sub.ReadOnly || sub.DisplayName != "Festivos" {
		t.Fatal("suscribirse", sub, err)
	}
	desde, hasta := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if inst, _ := repo.Instances(ctx, sub.ID, desde, hasta); len(inst) != 5 {
		t.Fatalf("instancias de la suscripción = %d, se esperaban 5", len(inst))
	}
	if _, err := repo.CreateEvent(ctx, sub.ID, api.EventInput{Summary: "x", Start: desde, End: desde.Add(time.Hour)}); err == nil {
		t.Fatal("una suscripción es de solo lectura")
	}
	// El feed cambia: se va la serie, el suelto cambia de título.
	cabecera, _, _ := strings.Cut(icsDePrueba, "BEGIN:VEVENT")
	_, suelto, ok := strings.Cut(icsDePrueba, "BEGIN:VEVENT\r\nUID:suelto")
	if !ok {
		t.Fatal("el feed de prueba no trae el evento suelto")
	}
	feed = cabecera + "BEGIN:VEVENT\r\nUID:suelto" + suelto
	feed = strings.Replace(feed, "SUMMARY:Comida", "SUMMARY:Comida de equipo", 1)
	if err := svc.RefreshSubscription(ctx, sub.ID); err != nil {
		t.Fatal("refrescar", err)
	}
	inst, _ := repo.Instances(ctx, sub.ID, desde, hasta)
	if len(inst) != 1 || inst[0].Event.Summary != "Comida de equipo" {
		t.Fatalf("tras refrescar: %+v", inst)
	}
	if pedidas != 2 {
		t.Fatal("una lectura al suscribirse y otra al refrescar", pedidas)
	}
}
